// Package service builds the API views from curated rows. The Repository interface is the only
// I/O; everything else is pure and unit-tested with a fake repository.
package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pociag-do-predykcji/services/go/api/internal/position"
)

// ErrNotFound is returned by Repository lookups of a missing id.
var ErrNotFound = errors.New("not found")

// Station is a stations row.
type Station struct {
	ID        int
	Name      string
	City      *string
	Latitude  *float64
	Longitude *float64
}

// Train is a trains row (without operating dates).
type Train struct {
	ID          int64
	ScheduleID  int
	OrderID     int
	Name        *string
	Number      *string
	Category    *string
	CarrierCode *string
	CarrierName *string
}

// Run is an operation: a train's run on one operating date, with its stops ordered by seq.
type Run struct {
	OperationID   int64
	OperatingDate time.Time
	Status        string
	UpdatedAt     time.Time // last PLK snapshot containing the run
	Train         Train
	Stops         []position.Stop
}

// BoardStop is one stop of a run at the board station.
type BoardStop struct {
	Run         Run // Stops is empty
	Stop        position.Stop
	Platform    *string // from the train's schedule
	Track       *string
	Origin      *string // first and last station names of the run
	Destination *string
}

// ScheduleStop is a schedule_stops row with its station name. Arrival and Departure are offsets
// from local midnight of the operating date.
type ScheduleStop struct {
	Seq         int
	StationID   int
	StationName string
	Arrival     *time.Duration
	Departure   *time.Duration
	Platform    *string
}

// TrainSchedule is a train with its planned stops.
type TrainSchedule struct {
	Train          Train
	OperatingDates []time.Time
	Stops          []ScheduleStop
}

// Place is a search endpoint: a station id, else a city or station-name prefix.
type Place struct {
	StationID int
	Name      string
}

// ConnectionQuery selects trains running on Date that stop at From and later at To.
type ConnectionQuery struct {
	Date       time.Time
	From, To   Place
	Carriers   []string
	Categories []string
}

// Connection is a train serving a ConnectionQuery: its departure stop and later arrival stop.
type Connection struct {
	Train Train
	From  ScheduleStop
	To    ScheduleStop
}

// Disruption is a disruptions row with its station names.
type Disruption struct {
	ID             int
	StartStation   *string
	EndStation     *string
	Message        string
	DateFrom       *time.Time
	DateTo         *time.Time
	AffectedTrains int
}

// DayStatistics summarises the runs of one operating date. Started counts runs in progress or
// completed; OnTime and AvgDelay only consider those, by their largest confirmed delay.
type DayStatistics struct {
	Total      int
	InProgress int // status P and in the latest snapshot
	Completed  int
	Cancelled  int // fully or partially
	Started    int
	OnTime     int // max delay <= 5 minutes
	AvgDelay   *float64
}

// Freshness is when each dataset was last written.
type Freshness struct {
	SchedulesUpdatedAt  *time.Time
	OperationsUpdatedAt *time.Time
	Disruptions         int
}

type Repository interface {
	Ping(ctx context.Context) error

	SearchStations(ctx context.Context, query string, limit int) ([]Station, error)
	ListMappedStations(ctx context.Context) ([]Station, error)
	GetStation(ctx context.Context, id int) (*Station, error)
	ListBoardStops(ctx context.Context, stationID int, from, to time.Time) ([]BoardStop, error)

	// LatestSnapshot is MAX(operations.updated_at), zero when there are no operations.
	LatestSnapshot(ctx context.Context) (time.Time, error)
	// ListActiveRuns returns the S/P runs on the given dates seen since seenSince whose
	// non-cancelled stops span `at`, with those stops (station names and coordinates included).
	ListActiveRuns(ctx context.Context, at, seenSince time.Time, dates []time.Time) ([]Run, error)
	GetRun(ctx context.Context, operationID int64) (*Run, error)

	SearchConnections(ctx context.Context, q ConnectionQuery) ([]Connection, error)
	GetTrainSchedule(ctx context.Context, trainID int64) (*TrainSchedule, error)

	ListDisruptions(ctx context.Context, limit, offset int) ([]Disruption, int, error)
	GetDayStatistics(ctx context.Context, date, seenSince time.Time) (*DayStatistics, error)
	GetFreshness(ctx context.Context) (*Freshness, error)
}

type Service struct {
	repo Repository
	now  func() time.Time
}

func New(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) Ready(ctx context.Context) error {
	if err := s.repo.Ping(ctx); err != nil {
		return fmt.Errorf("ready: %w", err)
	}
	return nil
}

// ── Shared helpers ────────────────────────────────────────────────────────────

// warsaw is loaded lazily so the binary's embedded time/tzdata is registered first.
var warsaw = sync.OnceValue(func() *time.Location {
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		return time.UTC
	}
	return loc
})

// warsawDate is the Europe/Warsaw calendar date of t (the PLK operating date) as a UTC midnight.
func warsawDate(t time.Time) time.Time {
	y, m, d := t.In(warsaw()).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// clock formats an instant as Europe/Warsaw HH:MM.
func clock(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.In(warsaw()).Format("15:04")
	return &s
}

// offsetClock formats a schedule offset from local midnight as HH:MM of its day.
func offsetClock(d *time.Duration) *string {
	if d == nil {
		return nil
	}
	minutes := int(d.Minutes()) % (24 * 60)
	if minutes < 0 {
		minutes += 24 * 60
	}
	s := fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
	return &s
}

// trainName is the human-facing train name: its name, else "<category> <number>", else
// "Pociąg <number>", else "Pociąg <schedule_id>/<order_id>".
func trainName(t Train) string {
	name, category, number := trimmed(t.Name), trimmed(t.Category), trimmed(t.Number)
	switch {
	case name != "":
		return name
	case category != "" && number != "":
		return category + " " + number
	case number != "":
		return "Pociąg " + number
	}
	return "Pociąg " + strconv.Itoa(t.ScheduleID) + "/" + strconv.Itoa(t.OrderID)
}

// statusLabel maps a PLK train status code to its API label.
func statusLabel(code string) string {
	switch code {
	case "P":
		return "in_progress"
	case "C":
		return "completed"
	case "X":
		return "cancelled"
	case "Q":
		return "partial_cancelled"
	}
	return "not_started"
}

func trimmed(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func containsFold(values []string, v *string) bool {
	if v == nil {
		return false
	}
	for _, candidate := range values {
		if strings.EqualFold(candidate, *v) {
			return true
		}
	}
	return false
}
