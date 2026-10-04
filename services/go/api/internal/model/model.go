// Package model holds the API response shapes. They are the contract in specs/openapi/api.yml,
// which the frontend (services/frontend/src/lib/api.ts) mirrors.
package model

import "time"

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type Pagination struct {
	Total   int  `json:"total"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

func NewPagination(total, limit, offset int) Pagination {
	return Pagination{Total: total, Limit: limit, Offset: offset, HasMore: offset+limit < total}
}

// ── Stations ──────────────────────────────────────────────────────────────────

type StationSuggestion struct {
	ExternalID int     `json:"external_id"`
	Name       string  `json:"name"`
	City       *string `json:"city,omitempty"`
}

type StationSuggestionsResponse struct {
	Suggestions []StationSuggestion `json:"suggestions"`
}

type StationMapPoint struct {
	ExternalID int     `json:"external_id"`
	Name       string  `json:"name"`
	City       *string `json:"city,omitempty"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
}

type StationMapResponse struct {
	Stations []StationMapPoint `json:"stations"`
}

type BoardStation struct {
	ExternalID int      `json:"external_id"`
	Name       string   `json:"name"`
	City       *string  `json:"city,omitempty"`
	Latitude   *float64 `json:"latitude,omitempty"`
	Longitude  *float64 `json:"longitude,omitempty"`
}

type Carrier struct {
	Code string  `json:"code"`
	Name *string `json:"name,omitempty"`
}

type StationBoardRow struct {
	OperationID        int64      `json:"operation_id"`
	TrainName          string     `json:"train_name"`
	TrainNumber        *string    `json:"train_number,omitempty"`
	CommercialCategory *string    `json:"commercial_category,omitempty"`
	Carrier            *Carrier   `json:"carrier,omitempty"`
	Origin             *string    `json:"origin,omitempty"`
	Destination        *string    `json:"destination,omitempty"`
	PlannedTime        *string    `json:"planned_time,omitempty"`
	ExpectedTime       *string    `json:"expected_time,omitempty"`
	ExpectedAt         *time.Time `json:"expected_at,omitempty"`
	DelayMinutes       *int       `json:"delay_minutes,omitempty"`
	Platform           *string    `json:"platform,omitempty"`
	Track              *string    `json:"track,omitempty"`
	Status             string     `json:"status"`
	IsCancelled        bool       `json:"is_cancelled"`
	IsConfirmed        bool       `json:"is_confirmed"`
}

type StationBoardView struct {
	Station     BoardStation      `json:"station"`
	GeneratedAt time.Time         `json:"generated_at"`
	DataAsOf    *time.Time        `json:"data_as_of,omitempty"`
	AtStation   []StationBoardRow `json:"at_station"`
	Arrivals    []StationBoardRow `json:"arrivals"`
	Departures  []StationBoardRow `json:"departures"`
}

// ── Schedules ─────────────────────────────────────────────────────────────────

type ScheduleEndpoint struct {
	StationName       string `json:"station_name"`
	StationExternalID int    `json:"station_external_id"`
	Time              string `json:"time"`
}

type ScheduleSearchResult struct {
	RouteID            int64            `json:"route_id"`
	TrainName          string           `json:"train_name"`
	Carrier            Carrier          `json:"carrier"`
	CommercialCategory *string          `json:"commercial_category,omitempty"`
	Departure          ScheduleEndpoint `json:"departure"`
	Arrival            ScheduleEndpoint `json:"arrival"`
	DurationMinutes    int              `json:"duration_minutes"`
	StopsCount         int              `json:"stops_count"`
}

type ScheduleSearchQuery struct {
	From string `json:"from"`
	To   string `json:"to"`
	Date string `json:"date"`
}

type ScheduleSearchResponse struct {
	Data       []ScheduleSearchResult `json:"data"`
	Pagination Pagination             `json:"pagination"`
	Query      ScheduleSearchQuery    `json:"query"`
}

type ScheduleStopView struct {
	StationName       string  `json:"station_name"`
	StationExternalID int     `json:"station_external_id"`
	Order             int     `json:"order"`
	ArrivalTime       *string `json:"arrival_time,omitempty"`
	DepartureTime     *string `json:"departure_time,omitempty"`
	Platform          *string `json:"platform,omitempty"`
}

type ScheduleDetailView struct {
	RouteID              int64              `json:"route_id"`
	TrainName            string             `json:"train_name"`
	Carrier              Carrier            `json:"carrier"`
	CommercialCategory   *string            `json:"commercial_category,omitempty"`
	NationalNumber       *string            `json:"national_number,omitempty"`
	Stops                []ScheduleStopView `json:"stops"`
	OperatingDates       []string           `json:"operating_dates"`
	TotalDurationMinutes *int               `json:"total_duration_minutes,omitempty"`
}

// ── Trains ────────────────────────────────────────────────────────────────────

type LiveTrainSummary struct {
	OperationID    int64   `json:"operation_id"`
	TrainName      string  `json:"train_name"`
	CarrierCode    *string `json:"carrier_code,omitempty"`
	Status         string  `json:"status"`
	StatusCode     string  `json:"status_code"`
	CurrentStation *string `json:"current_station,omitempty"`
	NextStation    *string `json:"next_station,omitempty"`
	DelayMinutes   *int    `json:"delay_minutes,omitempty"`
	Origin         *string `json:"origin,omitempty"`
	Destination    *string `json:"destination,omitempty"`
}

type LiveTrainsResponse struct {
	Data        []LiveTrainSummary `json:"data"`
	Pagination  Pagination         `json:"pagination"`
	GeneratedAt time.Time          `json:"generated_at"`
}

type TrainMapStop struct {
	StationName *string   `json:"station_name,omitempty"`
	Time        time.Time `json:"time"`
	Latitude    *float64  `json:"latitude,omitempty"`
	Longitude   *float64  `json:"longitude,omitempty"`
}

type TrainMapPoint struct {
	OperationID  int64         `json:"operation_id"`
	TrainName    string        `json:"train_name"`
	CarrierCode  *string       `json:"carrier_code,omitempty"`
	Status       string        `json:"status"`
	Phase        string        `json:"phase"`
	DelayMinutes *int          `json:"delay_minutes,omitempty"`
	Latitude     float64       `json:"latitude"`
	Longitude    float64       `json:"longitude"`
	Progress     float64       `json:"progress"`
	Method       string        `json:"method"`
	Confidence   string        `json:"confidence"`
	PreviousStop *TrainMapStop `json:"previous_stop,omitempty"`
	NextStop     *TrainMapStop `json:"next_stop,omitempty"`
	Origin       *string       `json:"origin,omitempty"`
	Destination  *string       `json:"destination,omitempty"`
}

type TrainMapResponse struct {
	Trains            []TrainMapPoint `json:"trains"`
	UnpositionedCount int             `json:"unpositioned_count"`
	GeneratedAt       time.Time       `json:"generated_at"`
	DataAsOf          *time.Time      `json:"data_as_of,omitempty"`
}

type TrainStopView struct {
	StationName           string  `json:"station_name"`
	StationExternalID     int     `json:"station_external_id"`
	Sequence              int     `json:"sequence"`
	PlannedArrival        *string `json:"planned_arrival,omitempty"`
	PlannedDeparture      *string `json:"planned_departure,omitempty"`
	ActualArrival         *string `json:"actual_arrival,omitempty"`
	ActualDeparture       *string `json:"actual_departure,omitempty"`
	ArrivalDelayMinutes   *int    `json:"arrival_delay_minutes,omitempty"`
	DepartureDelayMinutes *int    `json:"departure_delay_minutes,omitempty"`
	IsConfirmed           bool    `json:"is_confirmed"`
	IsCancelled           bool    `json:"is_cancelled"`
}

type TrainDetailView struct {
	OperationID   int64           `json:"operation_id"`
	TrainName     string          `json:"train_name"`
	Carrier       *Carrier        `json:"carrier,omitempty"`
	OperatingDate string          `json:"operating_date"`
	Status        string          `json:"status"`
	StatusCode    string          `json:"status_code"`
	Stops         []TrainStopView `json:"stops"`
}

// ── Disruptions & dashboard ───────────────────────────────────────────────────

type DisruptionView struct {
	ID                  int     `json:"id"`
	StartStation        *string `json:"start_station,omitempty"`
	EndStation          *string `json:"end_station,omitempty"`
	Message             string  `json:"message"`
	DateFrom            *string `json:"date_from,omitempty"`
	DateTo              *string `json:"date_to,omitempty"`
	AffectedRoutesCount int     `json:"affected_routes_count"`
	Severity            string  `json:"severity"`
}

type DisruptionListView struct {
	Data       []DisruptionView `json:"data"`
	Pagination Pagination       `json:"pagination"`
}

type DashboardStatistics struct {
	Date             string   `json:"date"`
	TotalTrains      int      `json:"total_trains"`
	InProgress       int      `json:"in_progress"`
	Completed        int      `json:"completed"`
	Cancelled        int      `json:"cancelled"`
	AvgDelayMinutes  *float64 `json:"avg_delay_minutes,omitempty"`
	OnTimePercentage *float64 `json:"on_time_percentage,omitempty"`
}

type DataFreshness struct {
	SchedulesLastUpdated  *time.Time `json:"schedules_last_updated,omitempty"`
	OperationsLastUpdated *time.Time `json:"operations_last_updated,omitempty"`
}

type DashboardOverview struct {
	Statistics        DashboardStatistics `json:"statistics"`
	DisruptionsActive int                 `json:"disruptions_active"`
	DataFreshness     DataFreshness       `json:"data_freshness"`
}
