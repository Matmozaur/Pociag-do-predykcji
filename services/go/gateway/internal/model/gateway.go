package model

import "time"

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type PaginationMeta struct {
	Total   int64 `json:"total"`
	Limit   int   `json:"limit"`
	Offset  int   `json:"offset"`
	HasMore bool  `json:"has_more"`
}

type StationSuggestion struct {
	ExternalID int     `json:"external_id"`
	Name       string  `json:"name"`
	City       *string `json:"city,omitempty"`
}

type StationSuggestionsResponse struct {
	Suggestions []StationSuggestion `json:"suggestions"`
}

type CarrierInfo struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type ScheduleEndpoint struct {
	StationName       string `json:"station_name"`
	StationExternalID *int   `json:"station_external_id,omitempty"`
	Time              string `json:"time"`
}

type ScheduleSearchResult struct {
	RouteID            int64            `json:"route_id"`
	TrainName          string           `json:"train_name"`
	Carrier            CarrierInfo      `json:"carrier"`
	CommercialCategory *string          `json:"commercial_category,omitempty"`
	Departure          ScheduleEndpoint `json:"departure"`
	Arrival            ScheduleEndpoint `json:"arrival"`
	DurationMinutes    *int             `json:"duration_minutes,omitempty"`
	StopsCount         *int             `json:"stops_count,omitempty"`
}

type ScheduleSearchQuery struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Date string `json:"date,omitempty"`
}

type ScheduleSearchResponse struct {
	Data       []ScheduleSearchResult `json:"data"`
	Pagination PaginationMeta         `json:"pagination"`
	Query      ScheduleSearchQuery    `json:"query"`
}

type ScheduleStopView struct {
	StationName       string  `json:"station_name"`
	StationExternalID *int    `json:"station_external_id,omitempty"`
	Order             int     `json:"order"`
	ArrivalTime       *string `json:"arrival_time,omitempty"`
	DepartureTime     *string `json:"departure_time,omitempty"`
	Platform          *string `json:"platform,omitempty"`
	StopType          *string `json:"stop_type,omitempty"`
}

type ScheduleDetailView struct {
	RouteID              int64              `json:"route_id"`
	TrainName            string             `json:"train_name"`
	Carrier              CarrierInfo        `json:"carrier"`
	CommercialCategory   *string            `json:"commercial_category,omitempty"`
	NationalNumber       *string            `json:"national_number,omitempty"`
	Stops                []ScheduleStopView `json:"stops"`
	OperatingDates       []string           `json:"operating_dates"`
	TotalDurationMinutes *int               `json:"total_duration_minutes,omitempty"`
}

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
	Pagination  PaginationMeta     `json:"pagination"`
	GeneratedAt time.Time          `json:"generated_at"`
}

type TrainStopView struct {
	StationName           string  `json:"station_name"`
	StationExternalID     *int    `json:"station_external_id,omitempty"`
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

type TrainCarrier struct {
	Code *string `json:"code,omitempty"`
	Name *string `json:"name,omitempty"`
}

type TrainDetailView struct {
	OperationID   int64           `json:"operation_id"`
	TrainName     string          `json:"train_name"`
	Carrier       *TrainCarrier   `json:"carrier,omitempty"`
	OperatingDate string          `json:"operating_date"`
	Status        string          `json:"status"`
	StatusCode    string          `json:"status_code"`
	Stops         []TrainStopView `json:"stops"`
}

type DisruptionSummaryView struct {
	ID                  int64   `json:"id"`
	TypeName            *string `json:"type_name,omitempty"`
	StartStation        *string `json:"start_station,omitempty"`
	EndStation          *string `json:"end_station,omitempty"`
	Message             string  `json:"message"`
	DateFrom            *string `json:"date_from,omitempty"`
	DateTo              *string `json:"date_to,omitempty"`
	AffectedRoutesCount *int    `json:"affected_routes_count,omitempty"`
	Severity            *string `json:"severity,omitempty"`
}

type DisruptionListView struct {
	Data       []DisruptionSummaryView `json:"data"`
	Pagination PaginationMeta          `json:"pagination"`
}

type DisruptionAffectedRouteView struct {
	TrainName     string  `json:"train_name"`
	CarrierCode   *string `json:"carrier_code,omitempty"`
	OperatingDate string  `json:"operating_date"`
	StationName   *string `json:"station_name,omitempty"`
}

type DisruptionDetailView struct {
	ID             int64                         `json:"id"`
	TypeName       *string                       `json:"type_name,omitempty"`
	StartStation   *string                       `json:"start_station,omitempty"`
	EndStation     *string                       `json:"end_station,omitempty"`
	Message        string                        `json:"message"`
	DateFrom       *string                       `json:"date_from,omitempty"`
	DateTo         *string                       `json:"date_to,omitempty"`
	AffectedRoutes []DisruptionAffectedRouteView `json:"affected_routes"`
}

type DashboardStatistics struct {
	Date             string   `json:"date"`
	TotalTrains      int      `json:"total_trains"`
	InProgress       *int     `json:"in_progress,omitempty"`
	Completed        *int     `json:"completed,omitempty"`
	Cancelled        *int     `json:"cancelled,omitempty"`
	AvgDelayMinutes  *float64 `json:"avg_delay_minutes,omitempty"`
	OnTimePercentage *float64 `json:"on_time_percentage,omitempty"`
}

type DashboardDataFreshness struct {
	SchedulesLastUpdated  *time.Time `json:"schedules_last_updated,omitempty"`
	OperationsLastUpdated *time.Time `json:"operations_last_updated,omitempty"`
}

type DashboardOverview struct {
	Statistics        DashboardStatistics    `json:"statistics"`
	DisruptionsActive int                    `json:"disruptions_active"`
	DataFreshness     DashboardDataFreshness `json:"data_freshness"`
}
