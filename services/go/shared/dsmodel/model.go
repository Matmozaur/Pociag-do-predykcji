package dsmodel

import "time"

// ── Common ────────────────────────────────────────────────────────────────────

type Pagination struct {
	Total  int64 `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	TraceID string `json:"trace_id,omitempty"`
}

// ── Dictionaries ──────────────────────────────────────────────────────────────

type Station struct {
	ID         int64   `json:"id"`
	ExternalID int     `json:"external_id"`
	Name       string  `json:"name"`
	City       *string `json:"city,omitempty"`
}

type StationListResponse struct {
	Data       []Station  `json:"data"`
	Pagination Pagination `json:"pagination"`
}

type Carrier struct {
	ID        int64      `json:"id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	ValidFrom *time.Time `json:"valid_from,omitempty"`
	ValidTo   *time.Time `json:"valid_to,omitempty"`
}

type CarrierListResponse struct {
	Data []Carrier `json:"data"`
}

type CommercialCategory struct {
	ID                int64   `json:"id"`
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	CarrierCode       *string `json:"carrier_code,omitempty"`
	SpeedCategoryCode *string `json:"speed_category_code,omitempty"`
}

type CommercialCategoryListResponse struct {
	Data []CommercialCategory `json:"data"`
}

type StopType struct {
	ID          int64  `json:"id"`
	ExternalID  int    `json:"external_id"`
	Description string `json:"description"`
}

type StopTypeListResponse struct {
	Data []StopType `json:"data"`
}

// ── Schedules ─────────────────────────────────────────────────────────────────

type RouteSummary struct {
	ID                       int64   `json:"id"`
	ScheduleID               int     `json:"schedule_id"`
	OrderID                  int     `json:"order_id"`
	TrainOrderID             *int    `json:"train_order_id,omitempty"`
	Name                     *string `json:"name,omitempty"`
	CarrierCode              *string `json:"carrier_code,omitempty"`
	NationalNumber           *string `json:"national_number,omitempty"`
	CommercialCategorySymbol *string `json:"commercial_category_symbol,omitempty"`
	FirstStationName         *string `json:"first_station_name,omitempty"`
	LastStationName          *string `json:"last_station_name,omitempty"`
	FirstDepartureTime       *string `json:"first_departure_time,omitempty"`
	LastArrivalTime          *string `json:"last_arrival_time,omitempty"`
	StationCount             int     `json:"station_count"`
	OperatingDatesCount      int     `json:"operating_dates_count"`
}

type RouteListResponse struct {
	Data       []RouteSummary `json:"data"`
	Pagination Pagination     `json:"pagination"`
}

type RouteStation struct {
	StationExternalID  int     `json:"station_external_id"`
	StationName        *string `json:"station_name,omitempty"`
	OrderNumber        int     `json:"order_number"`
	ArrivalTime        *string `json:"arrival_time,omitempty"`
	DepartureTime      *string `json:"departure_time,omitempty"`
	ArrivalDay         *int    `json:"arrival_day,omitempty"`
	DepartureDay       *int    `json:"departure_day,omitempty"`
	Platform           *string `json:"platform,omitempty"`
	Track              *string `json:"track,omitempty"`
	StopType           *string `json:"stop_type,omitempty"`
	CommercialCategory *string `json:"commercial_category,omitempty"`
}

type RouteStationListResponse struct {
	Data []RouteStation `json:"data"`
}

type OperatingDatesResponse struct {
	RouteID int64    `json:"route_id"`
	Dates   []string `json:"dates"`
}

type RouteConnection struct {
	ExternalConnectionID *string `json:"external_connection_id,omitempty"`
	TypeCode             string  `json:"type_code"`
	TypeName             *string `json:"type_name,omitempty"`
	StationExternalID    int     `json:"station_external_id"`
	StationName          *string `json:"station_name,omitempty"`
	WagonNumbers         *string `json:"wagon_numbers,omitempty"`
	Train1OrderID        *int    `json:"train1_order_id,omitempty"`
	Train2OrderID        *int    `json:"train2_order_id,omitempty"`
}

type RouteDetail struct {
	ID                        int64             `json:"id"`
	ScheduleID                int               `json:"schedule_id"`
	OrderID                   int               `json:"order_id"`
	TrainOrderID              *int              `json:"train_order_id,omitempty"`
	Name                      *string           `json:"name,omitempty"`
	CarrierCode               *string           `json:"carrier_code,omitempty"`
	NationalNumber            *string           `json:"national_number,omitempty"`
	InternationalArrivalNum   *string           `json:"international_arrival_num,omitempty"`
	InternationalDepartureNum *string           `json:"international_departure_num,omitempty"`
	CommercialCategorySymbol  *string           `json:"commercial_category_symbol,omitempty"`
	Stations                  []RouteStation    `json:"stations"`
	OperatingDates            []string          `json:"operating_dates"`
	Connections               []RouteConnection `json:"connections,omitempty"`
}

// ── Operations ────────────────────────────────────────────────────────────────

type OperationSummary struct {
	ID                       int64   `json:"id"`
	ScheduleID               int     `json:"schedule_id"`
	OrderID                  int     `json:"order_id"`
	TrainOrderID             *int    `json:"train_order_id,omitempty"`
	OperatingDate            string  `json:"operating_date"`
	TrainStatus              string  `json:"train_status"`
	RouteName                *string `json:"route_name,omitempty"`
	CarrierCode              *string `json:"carrier_code,omitempty"`
	MaxArrivalDelayMinutes   *int    `json:"max_arrival_delay_minutes,omitempty"`
	MaxDepartureDelayMinutes *int    `json:"max_departure_delay_minutes,omitempty"`
	StationCount             int     `json:"station_count"`
}

type OperationListResponse struct {
	Data       []OperationSummary `json:"data"`
	Pagination Pagination         `json:"pagination"`
}

type OperationStation struct {
	StationExternalID     int        `json:"station_external_id"`
	StationName           *string    `json:"station_name,omitempty"`
	PlannedSequenceNumber *int       `json:"planned_sequence_number,omitempty"`
	ActualSequenceNumber  int        `json:"actual_sequence_number"`
	PlannedArrival        *time.Time `json:"planned_arrival,omitempty"`
	PlannedDeparture      *time.Time `json:"planned_departure,omitempty"`
	ActualArrival         *time.Time `json:"actual_arrival,omitempty"`
	ActualDeparture       *time.Time `json:"actual_departure,omitempty"`
	ArrivalDelayMinutes   *int       `json:"arrival_delay_minutes,omitempty"`
	DepartureDelayMinutes *int       `json:"departure_delay_minutes,omitempty"`
	IsConfirmed           bool       `json:"is_confirmed"`
	IsCancelled           bool       `json:"is_cancelled"`
}

type OperationDetail struct {
	ID            int64              `json:"id"`
	ScheduleID    int                `json:"schedule_id"`
	OrderID       int                `json:"order_id"`
	TrainOrderID  *int               `json:"train_order_id,omitempty"`
	OperatingDate string             `json:"operating_date"`
	TrainStatus   string             `json:"train_status"`
	RouteName     *string            `json:"route_name,omitempty"`
	CarrierCode   *string            `json:"carrier_code,omitempty"`
	Stations      []OperationStation `json:"stations"`
}

type DelayDistribution struct {
	OnTime        int `json:"on_time"`
	SlightDelay   int `json:"slight_delay"`
	ModerateDelay int `json:"moderate_delay"`
	SevereDelay   int `json:"severe_delay"`
}

type OperationStatistics struct {
	Date              string            `json:"date"`
	Total             int               `json:"total"`
	ByStatus          map[string]int    `json:"by_status"`
	DelayDistribution DelayDistribution `json:"delay_distribution"`
	AvgDelayMinutes   *float64          `json:"avg_delay_minutes,omitempty"`
}

// ── Disruptions ───────────────────────────────────────────────────────────────

type DisruptionSummary struct {
	ID                   int64   `json:"id"`
	ExternalDisruptionID int64   `json:"external_disruption_id"`
	DisruptionTypeCode   *string `json:"disruption_type_code,omitempty"`
	DisruptionTypeName   *string `json:"disruption_type_name,omitempty"`
	StartStationExtID    *int    `json:"start_station_ext_id,omitempty"`
	StartStationName     *string `json:"start_station_name,omitempty"`
	EndStationExtID      *int    `json:"end_station_ext_id,omitempty"`
	EndStationName       *string `json:"end_station_name,omitempty"`
	Message              *string `json:"message,omitempty"`
	DateFrom             *string `json:"date_from,omitempty"`
	DateTo               *string `json:"date_to,omitempty"`
	AffectedRoutesCount  int     `json:"affected_routes_count"`
}

type DisruptionListResponse struct {
	Data       []DisruptionSummary `json:"data"`
	Pagination Pagination          `json:"pagination"`
}

type DisruptionAffectedRoute struct {
	ScheduleID     int     `json:"schedule_id"`
	OrderID        int     `json:"order_id"`
	TrainOrderID   *int    `json:"train_order_id,omitempty"`
	OperatingDate  *string `json:"operating_date,omitempty"`
	StationExtID   *int    `json:"station_ext_id,omitempty"`
	StationName    *string `json:"station_name,omitempty"`
	SequenceNumber *int    `json:"sequence_number,omitempty"`
}

type DisruptionDetail struct {
	ID                   int64                     `json:"id"`
	ExternalDisruptionID int64                     `json:"external_disruption_id"`
	DisruptionTypeCode   *string                   `json:"disruption_type_code,omitempty"`
	DisruptionTypeName   *string                   `json:"disruption_type_name,omitempty"`
	StartStationExtID    *int                      `json:"start_station_ext_id,omitempty"`
	StartStationName     *string                   `json:"start_station_name,omitempty"`
	EndStationExtID      *int                      `json:"end_station_ext_id,omitempty"`
	EndStationName       *string                   `json:"end_station_name,omitempty"`
	Message              *string                   `json:"message,omitempty"`
	DateFrom             *string                   `json:"date_from,omitempty"`
	DateTo               *string                   `json:"date_to,omitempty"`
	AffectedRoutes       []DisruptionAffectedRoute `json:"affected_routes"`
}
