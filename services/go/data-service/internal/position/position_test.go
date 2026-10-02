package position

import (
	"math"
	"testing"
	"time"
)

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func tp(s string) *time.Time { t := ts(s); return &t }
func ip(v int) *int          { return &v }
func fp(v float64) *float64  { return &v }

type stopOpt func(*Stop)

func arr(s string) stopOpt {
	return func(st *Stop) { st.PlannedArrival = tp(s); st.ActualArrival = tp(s) }
}
func dep(s string) stopOpt {
	return func(st *Stop) { st.PlannedDeparture = tp(s); st.ActualDeparture = tp(s) }
}
func coords(lat, lon float64) stopOpt {
	return func(st *Stop) { st.Latitude, st.Longitude = fp(lat), fp(lon) }
}
func confirmed() stopOpt { return func(st *Stop) { st.IsConfirmed = true } }
func cancelled() stopOpt { return func(st *Stop) { st.IsCancelled = true } }
func arrDelay(planned string, d int) stopOpt {
	return func(st *Stop) { st.PlannedArrival = tp(planned); st.ArrivalDelayMinutes = ip(d) }
}

func stop(id, seq int, opts ...stopOpt) Stop {
	s := Stop{StationExternalID: id, SequenceNumber: seq}
	for _, o := range opts {
		o(&s)
	}
	return s
}

// line is the base route: A (coords) dep 10:00 → B (coords) 10:30–10:32 → C (no coords)
// 11:00–11:01 → D (coords) arr 11:30. The first stop has no arrival, the last no departure.
func line() []Stop {
	return []Stop{
		stop(1, 1, dep("2026-10-02T10:00:00Z"), coords(52, 21), confirmed()),
		stop(2, 2, arr("2026-10-02T10:30:00Z"), dep("2026-10-02T10:32:00Z"), coords(52, 22)),
		stop(3, 3, arr("2026-10-02T11:00:00Z"), dep("2026-10-02T11:01:00Z")),
		stop(4, 4, arr("2026-10-02T11:30:00Z"), coords(53, 22)),
	}
}

func noCoords(stops []Stop) []Stop {
	out := append([]Stop(nil), stops...)
	for i := range out {
		out[i].Latitude, out[i].Longitude = nil, nil
	}
	return out
}

func TestEstimate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stops      []Stop
		at         string
		wantPhase  string
		wantPrev   int // station id, 0 = none
		wantNext   int
		wantMethod string // "" = no position
		wantLat    float64
		wantLon    float64
		wantProg   float64
	}{
		{
			name:  "not departed: before first departure, at the first stop",
			stops: line(), at: "2026-10-02T09:57:00Z",
			wantPhase: PhaseNotDeparted, wantPrev: 1, wantNext: 2,
			wantMethod: MethodStation, wantLat: 52, wantLon: 21,
		},
		{
			name:  "at station with coordinates",
			stops: line(), at: "2026-10-02T10:31:00Z",
			wantPhase: PhaseAtStation, wantPrev: 2, wantNext: 3,
			wantMethod: MethodStation, wantLat: 52, wantLon: 22,
		},
		{
			name:  "en route mid-segment between adjacent stops",
			stops: line(), at: "2026-10-02T10:15:00Z",
			wantPhase: PhaseEnRoute, wantPrev: 1, wantNext: 2,
			wantMethod: MethodInterpolated, wantLat: 52, wantLon: 21.5, wantProg: 0.5,
		},
		{
			name:  "sparse anchors: segment C→D, C has no coordinates, anchors B and D",
			stops: line(), at: "2026-10-02T11:15:00Z",
			wantPhase: PhaseEnRoute, wantPrev: 2, wantNext: 4,
			wantMethod: MethodInterpolatedSparse,
			// progress = (11:15 − 10:32) / (11:30 − 10:32) = 43/58
			wantLat: 52 + 43.0/58, wantLon: 22, wantProg: 43.0 / 58,
		},
		{
			name:  "at a station without coordinates: interpolated between B and D",
			stops: line(), at: "2026-10-02T11:00:30Z",
			wantPhase: PhaseAtStation, wantPrev: 2, wantNext: 4,
			wantMethod: MethodInterpolatedSparse,
			wantLat:    52 + 28.5/58, wantLon: 22, wantProg: 28.5 / 58,
		},
		{
			name:  "no coordinates anywhere: no position, segment stops reported",
			stops: noCoords(line()), at: "2026-10-02T10:15:00Z",
			wantPhase: PhaseEnRoute, wantPrev: 1, wantNext: 2,
		},
		{
			name: "single anchor behind: pinned to it with progress 0",
			stops: []Stop{
				stop(1, 1, dep("2026-10-02T10:00:00Z"), coords(52, 21)),
				stop(2, 2, arr("2026-10-02T10:30:00Z")),
			},
			at:        "2026-10-02T10:10:00Z",
			wantPhase: PhaseEnRoute, wantPrev: 1, wantNext: 2,
			wantMethod: MethodInterpolatedSparse, wantLat: 52, wantLon: 21, wantProg: 0,
		},
		{
			name: "single anchor ahead: pinned to it with progress 1",
			stops: []Stop{
				stop(1, 1, dep("2026-10-02T10:00:00Z")),
				stop(2, 2, arr("2026-10-02T10:30:00Z"), coords(53, 22)),
			},
			at:        "2026-10-02T10:10:00Z",
			wantPhase: PhaseEnRoute, wantPrev: 1, wantNext: 2,
			wantMethod: MethodInterpolatedSparse, wantLat: 53, wantLon: 22, wantProg: 1,
		},
		{
			name:  "last stop has no departure: arrived at the last stop",
			stops: line(), at: "2026-10-02T11:31:00Z",
			wantPhase: PhaseArrived, wantPrev: 4,
			wantMethod: MethodStation, wantLat: 53, wantLon: 22,
		},
		{
			name: "negative delay: forecast arrival 3 min early puts the train at the station",
			stops: []Stop{
				stop(1, 1, dep("2026-10-02T10:00:00Z"), coords(52, 21)),
				stop(2, 2, arrDelay("2026-10-02T10:30:00Z", -3), coords(52, 22)),
				stop(3, 3, arr("2026-10-02T11:00:00Z"), coords(52, 23)),
			},
			at:        "2026-10-02T10:27:00Z",
			wantPhase: PhaseAtStation, wantPrev: 2, wantNext: 3,
			wantMethod: MethodStation, wantLat: 52, wantLon: 22,
		},
		{
			name: "cancelled intermediate stop is skipped: A and D become adjacent",
			stops: []Stop{
				stop(1, 1, dep("2026-10-02T10:00:00Z"), coords(52, 21)),
				stop(2, 2, arr("2026-10-02T10:30:00Z"), dep("2026-10-02T10:32:00Z"), coords(60, 30), cancelled()),
				stop(4, 3, arr("2026-10-02T11:00:00Z"), coords(52, 23)),
			},
			at:        "2026-10-02T10:31:00Z",
			wantPhase: PhaseEnRoute, wantPrev: 1, wantNext: 4,
			wantMethod: MethodInterpolated, wantLat: 52, wantLon: 21 + 2*31.0/60, wantProg: 31.0 / 60,
		},
		{
			name: "DST end (2026-10-25): 02:30 CEST → 02:30 CET is one real hour, half way at 01:00Z",
			stops: []Stop{
				stop(1, 1, dep("2026-10-25T00:30:00Z"), coords(52, 21)),
				stop(2, 2, arr("2026-10-25T01:30:00Z"), coords(52, 22)),
			},
			at:        "2026-10-25T01:00:00Z",
			wantPhase: PhaseEnRoute, wantPrev: 1, wantNext: 2,
			wantMethod: MethodInterpolated, wantLat: 52, wantLon: 21.5, wantProg: 0.5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res, ok := Estimate(EffectiveTimes(tt.stops), ts(tt.at))
			if !ok {
				t.Fatal("Estimate returned !ok")
			}
			if res.Phase != tt.wantPhase {
				t.Errorf("phase = %s, want %s", res.Phase, tt.wantPhase)
			}
			if got := stationID(res.Previous); got != tt.wantPrev {
				t.Errorf("previous = %d, want %d", got, tt.wantPrev)
			}
			if got := stationID(res.Next); got != tt.wantNext {
				t.Errorf("next = %d, want %d", got, tt.wantNext)
			}
			if tt.wantMethod == "" {
				if res.Position != nil {
					t.Fatalf("position = %+v, want none", *res.Position)
				}
				return
			}
			p := res.Position
			if p == nil {
				t.Fatal("position = nil")
			}
			if p.Method != tt.wantMethod {
				t.Errorf("method = %s, want %s", p.Method, tt.wantMethod)
			}
			if !near(p.Latitude, tt.wantLat) || !near(p.Longitude, tt.wantLon) || !near(p.Progress, tt.wantProg) {
				t.Errorf("position = (%v, %v, progress %v), want (%v, %v, progress %v)",
					p.Latitude, p.Longitude, p.Progress, tt.wantLat, tt.wantLon, tt.wantProg)
			}
		})
	}
}

func TestEstimate_NoStops(t *testing.T) {
	t.Parallel()
	if _, ok := Estimate(nil, ts("2026-10-02T10:00:00Z")); ok {
		t.Fatal("Estimate(nil) ok = true")
	}
}

func TestProgress_Clamped(t *testing.T) {
	t.Parallel()

	from, to := ts("2026-10-02T10:00:00Z"), ts("2026-10-02T11:00:00Z")
	tests := []struct {
		name     string
		from, to time.Time
		at       string
		want     float64
	}{
		{"before start clamps to 0", from, to, "2026-10-02T09:00:00Z", 0},
		{"after end clamps to 1", from, to, "2026-10-02T12:00:00Z", 1},
		{"midpoint", from, to, "2026-10-02T10:30:00Z", 0.5},
		{"zero span, before", from, from, "2026-10-02T09:59:00Z", 0},
		{"inverted span, after", to, from, "2026-10-02T11:30:00Z", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := progress(tt.from, tt.to, ts(tt.at)); got != tt.want {
				t.Errorf("progress = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEffectiveTimes(t *testing.T) {
	t.Parallel()

	stops := EffectiveTimes([]Stop{
		stop(3, 3, arr("2026-10-02T11:00:00Z")),              // last: arrival only
		stop(9, 2, cancelled(), arr("2026-10-02T10:30:00Z")), // cancelled: skipped
		stop(8, 4),                                       // no times: skipped
		stop(1, 1, dep("2026-10-02T10:00:00Z")),          // first: departure only
		stop(2, 5, arrDelay("2026-10-02T12:00:00Z", -2)), // planned + negative delay
		{StationExternalID: 5, SequenceNumber: 6, PlannedDeparture: tp("2026-10-02T13:00:00Z")}, // no delay: planned
	})

	want := []struct {
		id       int
		arr, dep string
	}{
		{1, "2026-10-02T10:00:00Z", "2026-10-02T10:00:00Z"},
		{3, "2026-10-02T11:00:00Z", "2026-10-02T11:00:00Z"},
		{2, "2026-10-02T11:58:00Z", "2026-10-02T11:58:00Z"},
		{5, "2026-10-02T13:00:00Z", "2026-10-02T13:00:00Z"},
	}
	if len(stops) != len(want) {
		t.Fatalf("len = %d, want %d", len(stops), len(want))
	}
	for i, w := range want {
		s := stops[i]
		if s.StationExternalID != w.id || !s.Arrival.Equal(ts(w.arr)) || !s.Departure.Equal(ts(w.dep)) {
			t.Errorf("stop %d = {%d %v %v}, want {%d %s %s}", i, s.StationExternalID, s.Arrival, s.Departure, w.id, w.arr, w.dep)
		}
	}
}

func TestIsActive(t *testing.T) {
	t.Parallel()

	today := ts("2026-10-02T00:00:00Z")
	latest := ts("2026-10-02T10:40:00Z")
	tests := []struct {
		name   string
		op     Operation
		stops  []Stop
		at     string
		latest time.Time
		want   bool
	}{
		{name: "P en route", op: Operation{TrainStatus: "P", OperatingDate: today, LastSeenAt: latest}, stops: line(), at: "2026-10-02T10:15:00Z", want: true},
		{name: "P within the pre-departure window", op: Operation{TrainStatus: "P", OperatingDate: today, LastSeenAt: latest}, stops: line(), at: "2026-10-02T09:55:00Z", want: true},
		{name: "P before the pre-departure window", op: Operation{TrainStatus: "P", OperatingDate: today, LastSeenAt: latest}, stops: line(), at: "2026-10-02T09:54:59Z", want: false},
		{name: "P at the last arrival", op: Operation{TrainStatus: "P", OperatingDate: today, LastSeenAt: latest}, stops: line(), at: "2026-10-02T11:30:00Z", want: true},
		{name: "P after the last arrival: no grace", op: Operation{TrainStatus: "P", OperatingDate: today, LastSeenAt: latest}, stops: line(), at: "2026-10-02T11:30:01Z", want: false},
		{name: "S past its departure", op: Operation{TrainStatus: "S", OperatingDate: today, LastSeenAt: latest}, stops: line(), at: "2026-10-02T10:01:00Z", want: true},
		{name: "S before its departure", op: Operation{TrainStatus: "S", OperatingDate: today, LastSeenAt: latest}, stops: line(), at: "2026-10-02T09:58:00Z", want: false},
		{name: "Q excluded", op: Operation{TrainStatus: "Q", OperatingDate: today, LastSeenAt: latest}, stops: line(), at: "2026-10-02T10:15:00Z", want: false},
		{name: "C excluded", op: Operation{TrainStatus: "C", OperatingDate: today, LastSeenAt: latest}, stops: line(), at: "2026-10-02T10:15:00Z", want: false},
		{name: "stale snapshot", op: Operation{TrainStatus: "P", OperatingDate: today, LastSeenAt: latest.Add(-SnapshotWindow - time.Second)}, stops: line(), at: "2026-10-02T10:15:00Z", want: false},
		{name: "no stops", op: Operation{TrainStatus: "P", OperatingDate: today, LastSeenAt: latest}, at: "2026-10-02T10:15:00Z", want: false},
		{
			name:  "zombie P from two days ago",
			op:    Operation{TrainStatus: "P", OperatingDate: ts("2026-09-30T00:00:00Z"), LastSeenAt: latest},
			stops: []Stop{stop(1, 1, dep("2026-09-30T10:00:00Z")), stop(2, 2, arr("2026-09-30T11:00:00Z"))},
			at:    "2026-10-02T10:15:00Z", want: false,
		},
		{
			name:  "zombie P from two days ago even if its times look current",
			op:    Operation{TrainStatus: "P", OperatingDate: ts("2026-09-30T00:00:00Z"), LastSeenAt: latest},
			stops: line(), at: "2026-10-02T10:15:00Z", want: false,
		},
		{
			name:  "overnight train from yesterday's operating date",
			op:    Operation{TrainStatus: "P", OperatingDate: ts("2026-10-01T00:00:00Z"), LastSeenAt: latest},
			stops: []Stop{stop(1, 1, dep("2026-10-01T21:00:00Z")), stop(2, 2, arr("2026-10-02T04:00:00Z"))},
			at:    "2026-10-02T01:00:00Z", want: true,
		},
		{
			// 2026-10-25T23:30Z is 2026-10-26 00:30 CET, so 10-25 is "yesterday" in Warsaw although
			// it is still 10-25 in UTC; 10-24 is no longer a valid operating date.
			name:  "DST day: Warsaw date rolls over at 23:00Z after the switch to CET",
			op:    Operation{TrainStatus: "P", OperatingDate: ts("2026-10-24T00:00:00Z"), LastSeenAt: ts("2026-10-25T23:30:00Z")},
			stops: []Stop{stop(1, 1, dep("2026-10-25T20:00:00Z")), stop(2, 2, arr("2026-10-26T01:00:00Z"))},
			at:    "2026-10-25T23:30:00Z", latest: ts("2026-10-25T23:30:00Z"), want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := latest
			if !tt.latest.IsZero() {
				l = tt.latest
			}
			if got := IsActive(tt.op, EffectiveTimes(tt.stops), ts(tt.at), l); got != tt.want {
				t.Errorf("IsActive = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOperatingDates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		at        string
		wantToday string
	}{
		{"2026-10-02T10:00:00Z", "2026-10-02"},
		{"2026-10-02T21:59:59Z", "2026-10-02"}, // 23:59:59 CEST
		{"2026-10-02T22:00:00Z", "2026-10-03"}, // 00:00 CEST, still 10-02 in UTC
		// DST end: CEST (+2) until 2026-10-25T01:00Z, CET (+1) after.
		{"2026-10-24T21:59:00Z", "2026-10-24"},
		{"2026-10-24T22:00:00Z", "2026-10-25"},
		{"2026-10-25T22:59:00Z", "2026-10-25"},
		{"2026-10-25T23:00:00Z", "2026-10-26"},
		// DST start: 2026-03-29, CET until 01:00Z.
		{"2026-03-28T22:59:00Z", "2026-03-28"},
		{"2026-03-28T23:00:00Z", "2026-03-29"},
		{"2026-03-29T21:59:00Z", "2026-03-29"},
		{"2026-03-29T22:00:00Z", "2026-03-30"},
	}
	for _, tt := range tests {
		t.Run(tt.at, func(t *testing.T) {
			t.Parallel()
			dates := OperatingDates(ts(tt.at))
			today := ts(tt.wantToday + "T00:00:00Z")
			if len(dates) != 2 || !dates[0].Equal(today) || !dates[1].Equal(today.AddDate(0, 0, -1)) {
				t.Errorf("OperatingDates = %v, want [%s, day before]", dates, tt.wantToday)
			}
		})
	}
}

func TestLastConfirmedAndConfidence(t *testing.T) {
	t.Parallel()

	at := ts("2026-10-02T10:40:00Z")
	withDelays := func(st *Stop) { st.DepartureDelayMinutes = ip(4); st.ArrivalDelayMinutes = ip(2) }
	arrOnly := func(st *Stop) { st.ArrivalDelayMinutes = ip(-1) }

	tests := []struct {
		name          string
		stops         []Stop
		lastSeen      time.Time
		wantStop      int // 0 = none
		wantDelay     int
		wantConfident string
	}{
		{
			name: "recent confirmation and fresh snapshot: high, departure delay wins",
			stops: []Stop{
				stop(1, 1, dep("2026-10-02T10:00:00Z"), confirmed()),
				stop(2, 2, arr("2026-10-02T10:30:00Z"), dep("2026-10-02T10:32:00Z"), confirmed(), withDelays),
				stop(3, 3, arr("2026-10-02T11:00:00Z")),
			},
			lastSeen: at.Add(-5 * time.Minute), wantStop: 2, wantDelay: 4, wantConfident: ConfidenceHigh,
		},
		{
			name: "arrival delay fallback; confirmation 40 min old: medium",
			stops: []Stop{
				stop(1, 1, dep("2026-10-02T10:00:00Z"), confirmed(), arrOnly),
				stop(2, 2, arr("2026-10-02T11:00:00Z")),
			},
			lastSeen: at.Add(-5 * time.Minute), wantStop: 1, wantDelay: -1, wantConfident: ConfidenceMedium,
		},
		{
			name: "confirmed without delays reports 0",
			stops: []Stop{
				stop(1, 1, dep("2026-10-02T10:30:00Z"), confirmed()),
				stop(2, 2, arr("2026-10-02T11:00:00Z")),
			},
			lastSeen: at.Add(-5 * time.Minute), wantStop: 1, wantDelay: 0, wantConfident: ConfidenceHigh,
		},
		{
			name:     "nothing confirmed, fresh snapshot: medium",
			stops:    []Stop{stop(1, 1, dep("2026-10-02T10:30:00Z")), stop(2, 2, arr("2026-10-02T11:00:00Z"))},
			lastSeen: at.Add(-5 * time.Minute), wantConfident: ConfidenceMedium,
		},
		{
			name:     "old snapshot: low",
			stops:    []Stop{stop(1, 1, dep("2026-10-02T10:30:00Z"), confirmed()), stop(2, 2, arr("2026-10-02T11:00:00Z"))},
			lastSeen: at.Add(-31 * time.Minute), wantStop: 1, wantConfident: ConfidenceLow,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stops := EffectiveTimes(tt.stops)
			s, delay, ok := LastConfirmed(stops)
			if got := stationID(s); got != tt.wantStop || ok != (tt.wantStop != 0) {
				t.Fatalf("LastConfirmed = %d (ok %v), want %d", got, ok, tt.wantStop)
			}
			if delay != tt.wantDelay {
				t.Errorf("delay = %d, want %d", delay, tt.wantDelay)
			}
			if got := Confidence(tt.lastSeen, s, at); got != tt.wantConfident {
				t.Errorf("Confidence = %s, want %s", got, tt.wantConfident)
			}
		})
	}
}

func stationID(s *TimedStop) int {
	if s == nil {
		return 0
	}
	return s.StationExternalID
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
