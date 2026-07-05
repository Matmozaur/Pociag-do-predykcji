package service

import (
	"context"
	"time"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
)

// ── Param structs ─────────────────────────────────────────────────────────────

type QueryRoutesParams struct {
	DateFrom           *time.Time
	DateTo             *time.Time
	StationExternalIds []int
	FromCity           string
	ToCity             string
	CarrierCodes       []string
	CommercialCategory string
	Name               string
	Limit              int
	Offset             int
}

type QueryOperationsParams struct {
	Date               *time.Time
	StationExternalIds []int
	Status             string
	CarrierCodes       []string
	MinDelay           *int
	Limit              int
	Offset             int
}

type QueryDisruptionsParams struct {
	DateFrom           *time.Time
	DateTo             *time.Time
	StationExternalIds []int
	TypeCode           string
	Limit              int
	Offset             int
}

type QueryStationsParams struct {
	Search      string
	City        string
	ExternalIds []int
	Limit       int
	Offset      int
}

// ── Repository interface ──────────────────────────────────────────────────────

type Repository interface {
	Ping(ctx context.Context) error

	QueryRoutes(ctx context.Context, p QueryRoutesParams) ([]model.RouteSummary, int64, error)
	GetRouteById(ctx context.Context, id int64) (*model.RouteDetail, error)
	GetRouteByKey(ctx context.Context, scheduleID, orderID int) (*model.RouteDetail, error)
	GetRouteStations(ctx context.Context, routeID int64) ([]model.RouteStation, error)
	GetRouteOperatingDates(ctx context.Context, routeID int64, from, to *time.Time) ([]time.Time, error)

	QueryOperations(ctx context.Context, p QueryOperationsParams) ([]model.OperationSummary, int64, error)
	GetOperationById(ctx context.Context, id int64) (*model.OperationDetail, error)
	GetOperationStatistics(ctx context.Context, date time.Time) (*model.OperationStatistics, error)

	QueryDisruptions(ctx context.Context, p QueryDisruptionsParams) ([]model.DisruptionSummary, int64, error)
	GetDisruptionById(ctx context.Context, id int64) (*model.DisruptionDetail, error)

	QueryStations(ctx context.Context, p QueryStationsParams) ([]model.Station, int64, error)
	GetStationByExternalId(ctx context.Context, extID int) (*model.Station, error)
	ListCarriers(ctx context.Context) ([]model.Carrier, error)
	ListCommercialCategories(ctx context.Context) ([]model.CommercialCategory, error)
	ListStopTypes(ctx context.Context) ([]model.StopType, error)
}

// ── Service ───────────────────────────────────────────────────────────────────

type Service struct {
	Repository
}

func New(repo Repository) *Service {
	return &Service{Repository: repo}
}

func (s *Service) Ready(ctx context.Context) error {
	return s.Ping(ctx)
}
