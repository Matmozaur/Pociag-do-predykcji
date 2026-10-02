// Package position implements the active train model of docs/proposals/train-positions.md §4,
// as specified for listActiveOperations in specs/openapi/data-service.yml: effective stop times,
// the active predicate, and the phase and estimated position of a train at an instant. It is
// pure (no I/O) so other consumers of the ActiveTrain model can reuse it.
package position

import (
	"sort"
	"sync"
	"time"
	_ "time/tzdata" // the runtime image has no zoneinfo; operating dates are Europe/Warsaw dates
)

// Thresholds of the active predicate and of the confidence levels.
const (
	// PreDepartureWindow is how long before its effective departure from the first stop a
	// train already counts as active.
	PreDepartureWindow = 5 * time.Minute
	// SnapshotWindow is how far behind MAX(train_operations.updated_at) an operation may have
	// been last seen and still count as active.
	SnapshotWindow = 15 * time.Minute
	// HighConfidenceSnapshotAge and HighConfidenceConfirmationAge bound confidence high.
	HighConfidenceSnapshotAge     = 15 * time.Minute
	HighConfidenceConfirmationAge = 20 * time.Minute
	// MediumConfidenceSnapshotAge bounds confidence medium.
	MediumConfidenceSnapshotAge = 30 * time.Minute
)

// Train statuses that can pass the active predicate.
const (
	StatusNotStarted = "S"
	StatusInProgress = "P"
)

// Phases (ActiveTrain.phase).
const (
	PhaseNotDeparted = "not_departed"
	PhaseAtStation   = "at_station"
	PhaseEnRoute     = "en_route"
	PhaseArrived     = "arrived"
)

// Position methods (TrainPosition.method).
const (
	MethodStation            = "station"
	MethodInterpolated       = "interpolated"
	MethodInterpolatedSparse = "interpolated_sparse"
)

// Confidence levels (ActiveTrain.confidence).
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// Stop is one operation_stations row with its station's name and coordinates.
type Stop struct {
	StationExternalID     int
	StationName           *string
	SequenceNumber        int // actual_sequence_number
	PlannedArrival        *time.Time
	PlannedDeparture      *time.Time
	ActualArrival         *time.Time
	ActualDeparture       *time.Time
	ArrivalDelayMinutes   *int
	DepartureDelayMinutes *int
	IsConfirmed           bool
	IsCancelled           bool
	Latitude              *float64
	Longitude             *float64
}

// HasCoordinates reports whether the stop's station has coordinates.
func (s Stop) HasCoordinates() bool { return s.Latitude != nil && s.Longitude != nil }

// TimedStop is a non-cancelled stop with its effective times. A stop with only one effective
// time uses it for both, so the first stop (no arrival) and the last stop (no departure) work.
type TimedStop struct {
	Stop
	Arrival   time.Time
	Departure time.Time
}

// EffectiveTimes returns the non-cancelled stops that have an effective time, ordered by
// sequence number. The effective arrival is the actual arrival, else the planned arrival plus
// the arrival delay (0 when unknown); the effective departure follows the same rule. This is the
// expected-time rule of the station board query.
func EffectiveTimes(stops []Stop) []TimedStop {
	out := make([]TimedStop, 0, len(stops))
	for _, s := range stops {
		if s.IsCancelled {
			continue
		}
		arr := effective(s.ActualArrival, s.PlannedArrival, s.ArrivalDelayMinutes)
		dep := effective(s.ActualDeparture, s.PlannedDeparture, s.DepartureDelayMinutes)
		switch {
		case arr == nil && dep == nil:
			continue
		case arr == nil:
			arr = dep
		case dep == nil:
			dep = arr
		}
		out = append(out, TimedStop{Stop: s, Arrival: *arr, Departure: *dep})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SequenceNumber < out[j].SequenceNumber })
	return out
}

func effective(actual, planned *time.Time, delayMinutes *int) *time.Time {
	if actual != nil {
		return actual
	}
	if planned == nil {
		return nil
	}
	delay := 0
	if delayMinutes != nil {
		delay = *delayMinutes
	}
	t := planned.Add(time.Duration(delay) * time.Minute)
	return &t
}

var warsaw = sync.OnceValue(func() *time.Location {
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		// Unreachable: time/tzdata is embedded.
		return time.UTC
	}
	return loc
})

// OperatingDates returns the operating dates a train active at `at` can have: the
// Europe/Warsaw date of `at` and the day before, as UTC midnights.
func OperatingDates(at time.Time) []time.Time {
	y, m, d := at.In(warsaw()).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return []time.Time{today, today.AddDate(0, 0, -1)}
}

// Operation is the train_operations data the active predicate needs.
type Operation struct {
	TrainStatus   string
	OperatingDate time.Time // calendar date; only year, month and day are compared
	LastSeenAt    time.Time // train_operations.updated_at
}

// IsActive evaluates the active predicate at `at`. stops come from EffectiveTimes;
// latestSnapshot is MAX(train_operations.updated_at).
func IsActive(op Operation, stops []TimedStop, at, latestSnapshot time.Time) bool {
	if len(stops) == 0 {
		return false
	}
	firstDep := stops[0].Departure
	lastArr := stops[len(stops)-1].Arrival

	switch op.TrainStatus {
	case StatusInProgress:
	case StatusNotStarted:
		if at.Before(firstDep) {
			return false
		}
	default:
		return false
	}

	y, m, d := op.OperatingDate.Date()
	opDate := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	dateOK := false
	for _, date := range OperatingDates(at) {
		if opDate.Equal(date) {
			dateOK = true
		}
	}
	if !dateOK {
		return false
	}

	if op.LastSeenAt.Before(latestSnapshot.Add(-SnapshotWindow)) {
		return false
	}
	return !at.Before(firstDep.Add(-PreDepartureWindow)) && !at.After(lastArr)
}

// Point is an estimated position.
type Point struct {
	Latitude  float64
	Longitude float64
	Progress  float64 // 0..1 between anchors; 0 at a station
	Method    string
}

// Result is the phase and estimated position of a train at an instant.
type Result struct {
	Phase string
	// Previous is the stop the train is at or last departed from (anchor A when the position is
	// interpolated); its time is the effective departure. Nil only when there are no stops.
	Previous *TimedStop
	// Next is the next stop ahead (anchor B when the position is interpolated); its time is the
	// effective arrival. Nil at the last stop.
	Next *TimedStop
	// Position is nil when no anchor stop has coordinates.
	Position *Point
}

// Estimate returns the phase and position at `at` for stops from EffectiveTimes. ok is false
// when there are no stops.
func Estimate(stops []TimedStop, at time.Time) (res Result, ok bool) {
	n := len(stops)
	if n == 0 {
		return Result{}, false
	}

	switch {
	case at.Before(stops[0].Departure):
		return atStop(stops, 0, PhaseNotDeparted), true
	case at.After(stops[n-1].Arrival):
		return atStop(stops, n-1, PhaseArrived), true
	}

	for i := range stops {
		if !at.Before(stops[i].Arrival) && !at.After(stops[i].Departure) {
			if stops[i].HasCoordinates() {
				return atStop(stops, i, PhaseAtStation), true
			}
			// No coordinates at this stop: interpolate between the nearest stops that have them.
			return between(stops, i, at, PhaseAtStation), true
		}
	}

	// En route: the segment (i, i+1) with dep(i) < at < arr(i+1). If effective times are not
	// monotonic, fall back to the last stop departed before `at`.
	seg := -1
	for i := 0; i < n-1; i++ {
		if at.After(stops[i].Departure) && at.Before(stops[i+1].Arrival) {
			seg = i
			break
		}
		if !at.Before(stops[i].Departure) {
			seg = i
		}
	}
	if seg < 0 {
		seg = 0
	}
	return between(stops, seg, at, PhaseEnRoute), true
}

// atStop places the train at stop i.
func atStop(stops []TimedStop, i int, phase string) Result {
	res := Result{Phase: phase, Previous: &stops[i]}
	if i+1 < len(stops) {
		res.Next = &stops[i+1]
	}
	if s := stops[i]; s.HasCoordinates() {
		res.Position = &Point{Latitude: *s.Latitude, Longitude: *s.Longitude, Progress: 0, Method: MethodStation}
	}
	return res
}

// between places the train between anchor A (the nearest stop ≤ i with coordinates) and anchor
// B (the nearest stop ≥ i+1 with coordinates). With a single anchor the train is pinned to it
// (progress 0 at A, 1 at B); with none, Position is nil.
func between(stops []TimedStop, i int, at time.Time, phase string) Result {
	a, b := -1, -1
	for k := i; k >= 0; k-- {
		if stops[k].HasCoordinates() {
			a = k
			break
		}
	}
	for k := i + 1; k < len(stops); k++ {
		if stops[k].HasCoordinates() {
			b = k
			break
		}
	}

	res := Result{Phase: phase, Previous: &stops[i]}
	if i+1 < len(stops) {
		res.Next = &stops[i+1]
	}
	if a < 0 && b < 0 {
		return res
	}

	method := MethodInterpolated
	if a != i || b != i+1 {
		method = MethodInterpolatedSparse
	}
	switch {
	case a >= 0 && b >= 0:
		res.Previous, res.Next = &stops[a], &stops[b]
		A, B := stops[a], stops[b]
		p := progress(A.Departure, B.Arrival, at)
		res.Position = &Point{
			Latitude:  *A.Latitude + p*(*B.Latitude-*A.Latitude),
			Longitude: *A.Longitude + p*(*B.Longitude-*A.Longitude),
			Progress:  p,
			Method:    method,
		}
	case a >= 0:
		res.Previous = &stops[a]
		res.Position = &Point{Latitude: *stops[a].Latitude, Longitude: *stops[a].Longitude, Progress: 0, Method: method}
	default:
		res.Next = &stops[b]
		res.Position = &Point{Latitude: *stops[b].Latitude, Longitude: *stops[b].Longitude, Progress: 1, Method: method}
	}
	return res
}

// progress is (at - from) / (to - from), clamped to [0, 1].
func progress(from, to, at time.Time) float64 {
	span := to.Sub(from)
	if span <= 0 {
		if at.Before(to) {
			return 0
		}
		return 1
	}
	p := float64(at.Sub(from)) / float64(span)
	return min(max(p, 0), 1)
}

// LastConfirmed returns the last confirmed stop and its delay (departure delay, else arrival
// delay; 0 when PLK reports neither, which it does for on-time stops). ok is false when no stop
// is confirmed.
func LastConfirmed(stops []TimedStop) (stop *TimedStop, delayMinutes int, ok bool) {
	for i := len(stops) - 1; i >= 0; i-- {
		s := &stops[i]
		if !s.IsConfirmed {
			continue
		}
		switch {
		case s.DepartureDelayMinutes != nil:
			delayMinutes = *s.DepartureDelayMinutes
		case s.ArrivalDelayMinutes != nil:
			delayMinutes = *s.ArrivalDelayMinutes
		}
		return s, delayMinutes, true
	}
	return nil, 0, false
}

// Confidence grades the estimate at `at`: high when the operation was seen under 15 min ago and
// the last confirmation (its effective departure, else arrival) is under 20 min old; medium when
// it was seen under 30 min ago; low otherwise. lastConfirmed may be nil.
func Confidence(lastSeenAt time.Time, lastConfirmed *TimedStop, at time.Time) string {
	snapshotAge := at.Sub(lastSeenAt)
	if snapshotAge < HighConfidenceSnapshotAge && lastConfirmed != nil &&
		at.Sub(lastConfirmed.Departure) < HighConfidenceConfirmationAge {
		return ConfidenceHigh
	}
	if snapshotAge < MediumConfidenceSnapshotAge {
		return ConfidenceMedium
	}
	return ConfidenceLow
}
