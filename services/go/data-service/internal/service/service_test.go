package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
)

// mockRepository implements the Repository interface, delegating to optional
// function fields so each test only wires up what it needs.
type mockRepository struct {
	getMapRoutesFn func(ctx context.Context) ([]model.MapRoute, error)
}

func (m *mockRepository) Ping(ctx context.Context) error { return nil }

func (m *mockRepository) QueryRoutes(ctx context.Context, p QueryRoutesParams) ([]model.RouteSummary, int64, error) {
	return nil, 0, nil
}

func (m *mockRepository) GetRouteById(ctx context.Context, id int64) (*model.RouteDetail, error) {
	return &model.RouteDetail{}, nil
}

func (m *mockRepository) GetRouteByKey(ctx context.Context, scheduleID, orderID int) (*model.RouteDetail, error) {
	return &model.RouteDetail{}, nil
}

func (m *mockRepository) GetRouteStations(ctx context.Context, routeID int64) ([]model.RouteStation, error) {
	return nil, nil
}

func (m *mockRepository) GetRouteOperatingDates(ctx context.Context, routeID int64, from, to *time.Time) ([]time.Time, error) {
	return nil, nil
}

func (m *mockRepository) GetMapRoutes(ctx context.Context) ([]model.MapRoute, error) {
	if m.getMapRoutesFn != nil {
		return m.getMapRoutesFn(ctx)
	}
	return nil, nil
}

func (m *mockRepository) QueryOperations(ctx context.Context, p QueryOperationsParams) ([]model.OperationSummary, int64, error) {
	return nil, 0, nil
}

func (m *mockRepository) GetOperationById(ctx context.Context, id int64) (*model.OperationDetail, error) {
	return &model.OperationDetail{}, nil
}

func (m *mockRepository) GetOperationStatistics(ctx context.Context, date time.Time) (*model.OperationStatistics, error) {
	return &model.OperationStatistics{}, nil
}

func (m *mockRepository) QueryDisruptions(ctx context.Context, p QueryDisruptionsParams) ([]model.DisruptionSummary, int64, error) {
	return nil, 0, nil
}

func (m *mockRepository) GetDisruptionById(ctx context.Context, id int64) (*model.DisruptionDetail, error) {
	return &model.DisruptionDetail{}, nil
}

func (m *mockRepository) QueryStations(ctx context.Context, p QueryStationsParams) ([]model.Station, int64, error) {
	return nil, 0, nil
}

func (m *mockRepository) GetStationByExternalId(ctx context.Context, extID int) (*model.Station, error) {
	return &model.Station{}, nil
}

func (m *mockRepository) ListCarriers(ctx context.Context) ([]model.Carrier, error) {
	return nil, nil
}

func (m *mockRepository) ListCommercialCategories(ctx context.Context) ([]model.CommercialCategory, error) {
	return nil, nil
}

func (m *mockRepository) ListStopTypes(ctx context.Context) ([]model.StopType, error) {
	return nil, nil
}

func TestGetMapRoutes_ReturnsRepositoryData(t *testing.T) {
	trainName := "IC 8301"
	carrierCode := "IC"
	category := "IC"

	repo := &mockRepository{
		getMapRoutesFn: func(ctx context.Context) ([]model.MapRoute, error) {
			return []model.MapRoute{
				{
					RouteID:                  1,
					TrainName:                &trainName,
					CarrierCode:              &carrierCode,
					CommercialCategorySymbol: &category,
					StationExternalIDs:       []int{100, 200, 300},
					RouteCount:               5,
				},
			}, nil
		},
	}

	svc := New(repo)
	routes, err := svc.GetMapRoutes(context.Background())
	if err != nil {
		t.Fatalf("GetMapRoutes returned error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].RouteID != 1 {
		t.Fatalf("expected route_id 1, got %d", routes[0].RouteID)
	}
	if len(routes[0].StationExternalIDs) != 3 || routes[0].StationExternalIDs[2] != 300 {
		t.Fatalf("unexpected station_external_ids: %v", routes[0].StationExternalIDs)
	}
	if routes[0].RouteCount != 5 {
		t.Fatalf("expected route_count 5, got %d", routes[0].RouteCount)
	}
}

func TestGetMapRoutes_PropagatesRepositoryError(t *testing.T) {
	wantErr := errors.New("db unavailable")
	repo := &mockRepository{
		getMapRoutesFn: func(ctx context.Context) ([]model.MapRoute, error) {
			return nil, wantErr
		},
	}

	svc := New(repo)
	_, err := svc.GetMapRoutes(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error to wrap %v, got %v", wantErr, err)
	}
}
