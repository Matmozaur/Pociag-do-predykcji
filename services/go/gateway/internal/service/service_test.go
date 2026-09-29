package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pociag-do-predykcji/services/go/gateway/internal/client/dataservice"
)

type mockDataServiceClient struct {
	queryRoutesFn            func(ctx context.Context, p dataservice.QueryRoutesParams) (*dataservice.RouteListResponse, error)
	getRouteStationsFn       func(ctx context.Context, routeID int64) (*dataservice.RouteStationListResponse, error)
	getStationByExternalIDFn func(ctx context.Context, externalID int) (*dataservice.Station, error)
	getRouteByKeyFn          func(ctx context.Context, scheduleID, orderID int) (*dataservice.RouteDetail, error)
	listCarriersFn           func(ctx context.Context) (*dataservice.CarrierListResponse, error)
	getOperationStatsFn      func(ctx context.Context, date string) (*dataservice.OperationStatistics, error)
	queryDisruptionsFn       func(ctx context.Context, p dataservice.QueryDisruptionsParams) (*dataservice.DisruptionListResponse, error)
	getDisruptionByIDFn      func(ctx context.Context, disruptionID int64) (*dataservice.DisruptionDetail, error)
	queryOperationsFn        func(ctx context.Context, p dataservice.QueryOperationsParams) (*dataservice.OperationListResponse, error)
	getOperationByIDFn       func(ctx context.Context, operationID int64) (*dataservice.OperationDetail, error)
}

func (m *mockDataServiceClient) Ready(ctx context.Context) error { return nil }

func (m *mockDataServiceClient) QueryStations(ctx context.Context, search string, limit, offset int) (*dataservice.StationListResponse, error) {
	return &dataservice.StationListResponse{}, nil
}

func (m *mockDataServiceClient) ListStationsWithCoordinates(ctx context.Context, limit int) (*dataservice.StationListResponse, error) {
	return &dataservice.StationListResponse{}, nil
}

func (m *mockDataServiceClient) GetStationByExternalID(ctx context.Context, externalID int) (*dataservice.Station, error) {
	if m.getStationByExternalIDFn != nil {
		return m.getStationByExternalIDFn(ctx, externalID)
	}
	return &dataservice.Station{}, nil
}

func (m *mockDataServiceClient) ListCarriers(ctx context.Context) (*dataservice.CarrierListResponse, error) {
	if m.listCarriersFn != nil {
		return m.listCarriersFn(ctx)
	}
	return &dataservice.CarrierListResponse{}, nil
}

func (m *mockDataServiceClient) QueryRoutes(ctx context.Context, p dataservice.QueryRoutesParams) (*dataservice.RouteListResponse, error) {
	return m.queryRoutesFn(ctx, p)
}

func (m *mockDataServiceClient) GetRouteByID(ctx context.Context, routeID int64) (*dataservice.RouteDetail, error) {
	return &dataservice.RouteDetail{}, nil
}

func (m *mockDataServiceClient) GetRouteByKey(ctx context.Context, scheduleID, orderID int) (*dataservice.RouteDetail, error) {
	if m.getRouteByKeyFn != nil {
		return m.getRouteByKeyFn(ctx, scheduleID, orderID)
	}
	return &dataservice.RouteDetail{}, nil
}

func (m *mockDataServiceClient) GetRouteStations(ctx context.Context, routeID int64) (*dataservice.RouteStationListResponse, error) {
	return m.getRouteStationsFn(ctx, routeID)
}

func (m *mockDataServiceClient) GetRouteOperatingDates(ctx context.Context, routeID int64) (*dataservice.OperatingDatesResponse, error) {
	return &dataservice.OperatingDatesResponse{}, nil
}

func (m *mockDataServiceClient) QueryOperations(ctx context.Context, p dataservice.QueryOperationsParams) (*dataservice.OperationListResponse, error) {
	if m.queryOperationsFn != nil {
		return m.queryOperationsFn(ctx, p)
	}
	return &dataservice.OperationListResponse{}, nil
}

func (m *mockDataServiceClient) GetOperationByID(ctx context.Context, operationID int64) (*dataservice.OperationDetail, error) {
	if m.getOperationByIDFn != nil {
		return m.getOperationByIDFn(ctx, operationID)
	}
	return &dataservice.OperationDetail{}, nil
}

func (m *mockDataServiceClient) GetOperationStatistics(ctx context.Context, date string) (*dataservice.OperationStatistics, error) {
	return m.getOperationStatsFn(ctx, date)
}

func (m *mockDataServiceClient) QueryDisruptions(ctx context.Context, p dataservice.QueryDisruptionsParams) (*dataservice.DisruptionListResponse, error) {
	return m.queryDisruptionsFn(ctx, p)
}

func (m *mockDataServiceClient) GetDisruptionByID(ctx context.Context, disruptionID int64) (*dataservice.DisruptionDetail, error) {
	if m.getDisruptionByIDFn != nil {
		return m.getDisruptionByIDFn(ctx, disruptionID)
	}
	return &dataservice.DisruptionDetail{}, nil
}

func TestSearchSchedules_MultiCategoryDedupAndStationOrder(t *testing.T) {
	carrierCode := "IC"
	trainName := "IC 8301"
	mockClient := &mockDataServiceClient{
		listCarriersFn: func(ctx context.Context) (*dataservice.CarrierListResponse, error) {
			return &dataservice.CarrierListResponse{Data: []dataservice.Carrier{{Code: "IC", Name: "Intercity"}}}, nil
		},
		queryRoutesFn: func(ctx context.Context, p dataservice.QueryRoutesParams) (*dataservice.RouteListResponse, error) {
			return &dataservice.RouteListResponse{Data: []dataservice.RouteSummary{
				{ID: 1, Name: &trainName, CarrierCode: &carrierCode, FirstDepartureTime: ptr("07:00"), LastArrivalTime: ptr("09:00"), StationCount: 4},
				{ID: 2, Name: &trainName, CarrierCode: &carrierCode, FirstDepartureTime: ptr("07:30"), LastArrivalTime: ptr("09:10"), StationCount: 4},
			}}, nil
		},
		getRouteStationsFn: func(ctx context.Context, routeID int64) (*dataservice.RouteStationListResponse, error) {
			if routeID == 1 {
				return &dataservice.RouteStationListResponse{Data: []dataservice.RouteStation{
					{StationExternalID: 100, OrderNumber: 1},
					{StationExternalID: 200, OrderNumber: 3},
				}}, nil
			}
			return &dataservice.RouteStationListResponse{Data: []dataservice.RouteStation{
				{StationExternalID: 100, OrderNumber: 4},
				{StationExternalID: 200, OrderNumber: 2},
			}}, nil
		},
	}

	svc := New(mockClient)
	resp, err := svc.SearchSchedules(context.Background(), "100", "200", "2026-07-05", nil, []string{"IC", "TLK"}, "departure", 20, 0)
	if err != nil {
		t.Fatalf("SearchSchedules returned error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 item after dedupe and order filter, got %d", len(resp.Data))
	}
	if resp.Data[0].RouteID != 1 {
		t.Fatalf("expected route 1, got %d", resp.Data[0].RouteID)
	}
}

func TestSearchSchedules_MixedFromCityAndToStationID_AppliesBothFilters(t *testing.T) {
	carrierCode := "IC"
	trainName := "IC 8301"

	queryCalls := 0
	mockClient := &mockDataServiceClient{
		listCarriersFn: func(ctx context.Context) (*dataservice.CarrierListResponse, error) {
			return &dataservice.CarrierListResponse{Data: []dataservice.Carrier{{Code: "IC", Name: "Intercity"}}}, nil
		},
		queryRoutesFn: func(ctx context.Context, p dataservice.QueryRoutesParams) (*dataservice.RouteListResponse, error) {
			queryCalls++
			if p.FromCity != "Warszawa" {
				t.Fatalf("expected from city filter to be preserved, got %q", p.FromCity)
			}
			return &dataservice.RouteListResponse{Data: []dataservice.RouteSummary{
				{ID: 1, Name: &trainName, CarrierCode: &carrierCode, FirstDepartureTime: ptr("07:00"), LastArrivalTime: ptr("09:00"), StationCount: 4},
				{ID: 2, Name: &trainName, CarrierCode: &carrierCode, FirstDepartureTime: ptr("07:15"), LastArrivalTime: ptr("10:00"), StationCount: 5},
			}}, nil
		},
		getRouteStationsFn: func(ctx context.Context, routeID int64) (*dataservice.RouteStationListResponse, error) {
			switch routeID {
			case 1:
				return &dataservice.RouteStationListResponse{Data: []dataservice.RouteStation{
					{StationExternalID: 100, OrderNumber: 1},
					{StationExternalID: 300, OrderNumber: 4},
				}}, nil
			case 2:
				return &dataservice.RouteStationListResponse{Data: []dataservice.RouteStation{
					{StationExternalID: 101, OrderNumber: 1},
					{StationExternalID: 300, OrderNumber: 3},
				}}, nil
			default:
				return nil, errors.New("unexpected route id")
			}
		},
		getStationByExternalIDFn: func(ctx context.Context, externalID int) (*dataservice.Station, error) {
			switch externalID {
			case 100:
				city := "Warszawa"
				return &dataservice.Station{ExternalID: externalID, City: &city}, nil
			case 101:
				city := "Krakow"
				return &dataservice.Station{ExternalID: externalID, City: &city}, nil
			case 300:
				city := "Gdansk"
				return &dataservice.Station{ExternalID: externalID, City: &city}, nil
			default:
				return nil, errors.New("unexpected station id")
			}
		},
	}

	svc := New(mockClient)
	resp, err := svc.SearchSchedules(context.Background(), "Warszawa", "300", "2026-07-05", nil, nil, "departure", 20, 0)
	if err != nil {
		t.Fatalf("SearchSchedules returned error: %v", err)
	}
	if queryCalls == 0 {
		t.Fatal("expected query routes to be called")
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 item after mixed filtering, got %d", len(resp.Data))
	}
	if resp.Data[0].RouteID != 1 {
		t.Fatalf("expected route 1, got %d", resp.Data[0].RouteID)
	}
}

func TestGetDisruptionDetail_MissingOperatingDate_ReturnsError(t *testing.T) {
	mockClient := &mockDataServiceClient{
		getDisruptionByIDFn: func(ctx context.Context, disruptionID int64) (*dataservice.DisruptionDetail, error) {
			return &dataservice.DisruptionDetail{
				ID: 1,
				AffectedRoutes: []dataservice.DisruptionAffectedRoute{
					{ScheduleID: 10, OrderID: 20, OperatingDate: nil},
				},
			}, nil
		},
	}

	svc := New(mockClient)
	_, err := svc.GetDisruptionDetail(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error when operating_date is missing")
	}
}

func TestGetDashboardOverview_ComputesStatistics(t *testing.T) {
	lastUpdated := time.Date(2026, 7, 5, 6, 13, 20, 0, time.UTC)
	mockClient := &mockDataServiceClient{
		getOperationStatsFn: func(ctx context.Context, date string) (*dataservice.OperationStatistics, error) {
			avg := 6.5
			return &dataservice.OperationStatistics{
				Date:  "2026-07-05",
				Total: 10,
				ByStatus: map[string]int{
					"P": 3,
					"C": 6,
					"X": 1,
				},
				DelayDistribution: dataservice.DelayDistribution{OnTime: 7},
				AvgDelayMinutes:   &avg,
				LastUpdatedAt:     &lastUpdated,
			}, nil
		},
		queryDisruptionsFn: func(ctx context.Context, p dataservice.QueryDisruptionsParams) (*dataservice.DisruptionListResponse, error) {
			return &dataservice.DisruptionListResponse{Pagination: dataservice.Pagination{Total: 4}}, nil
		},
	}

	svc := New(mockClient)
	resp, err := svc.GetDashboardOverview(context.Background())
	if err != nil {
		t.Fatalf("GetDashboardOverview returned error: %v", err)
	}
	if resp.Statistics.TotalTrains != 10 {
		t.Fatalf("expected total trains 10, got %d", resp.Statistics.TotalTrains)
	}
	if resp.DisruptionsActive != 4 {
		t.Fatalf("expected disruptions active 4, got %d", resp.DisruptionsActive)
	}
	if resp.DataFreshness.SchedulesLastUpdated != nil {
		t.Fatal("expected data_freshness.schedules_last_updated to be omitted")
	}
	if resp.DataFreshness.OperationsLastUpdated == nil || !resp.DataFreshness.OperationsLastUpdated.Equal(lastUpdated) {
		t.Fatalf("expected data_freshness.operations_last_updated %v, got %v", lastUpdated, resp.DataFreshness.OperationsLastUpdated)
	}
}

func TestWarsawToday(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		{name: "CEST after local midnight", now: time.Date(2026, 9, 27, 22, 30, 0, 0, time.UTC), want: "2026-09-28"},
		{name: "CET after local midnight", now: time.Date(2026, 1, 15, 23, 30, 0, 0, time.UTC), want: "2026-01-16"},
		{name: "CEST before local midnight", now: time.Date(2026, 9, 27, 21, 30, 0, 0, time.UTC), want: "2026-09-27"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := warsawToday(tt.now); got != tt.want {
				t.Fatalf("warsawToday(%s) = %s, want %s", tt.now, got, tt.want)
			}
		})
	}
}

func TestGetLiveTrains_RequestsActiveOperationsForWarsawToday(t *testing.T) {
	var got dataservice.QueryOperationsParams
	mockClient := &mockDataServiceClient{
		queryOperationsFn: func(ctx context.Context, p dataservice.QueryOperationsParams) (*dataservice.OperationListResponse, error) {
			got = p
			return &dataservice.OperationListResponse{}, nil
		},
	}

	svc := New(mockClient)
	if _, err := svc.GetLiveTrains(context.Background(), []string{"IC"}, nil, 20, 0); err != nil {
		t.Fatalf("GetLiveTrains returned error: %v", err)
	}
	if !got.ActiveOnly {
		t.Fatal("expected ActiveOnly=true")
	}
	if got.Status != "" {
		t.Fatalf("expected no explicit status filter, got %q", got.Status)
	}
	want := warsawToday(time.Now())
	if got.Date == nil || *got.Date != want {
		t.Fatalf("expected date %s, got %v", want, got.Date)
	}
	if len(got.CarrierCodes) != 1 || got.CarrierCodes[0] != "IC" {
		t.Fatalf("expected carrier codes [IC], got %v", got.CarrierCodes)
	}
}

func TestGetLiveTrains_ResolvesCurrentAndNextStation(t *testing.T) {
	trainName := "IC 8301"
	carrierCode := "IC"
	station1 := "Warszawa Centralna"
	station2 := "Krakow Glowny"

	mockClient := &mockDataServiceClient{
		queryOperationsFn: func(ctx context.Context, p dataservice.QueryOperationsParams) (*dataservice.OperationListResponse, error) {
			return &dataservice.OperationListResponse{
				Data:       []dataservice.OperationSummary{{ID: 1}},
				Pagination: dataservice.Pagination{Total: 1},
			}, nil
		},
		getOperationByIDFn: func(ctx context.Context, operationID int64) (*dataservice.OperationDetail, error) {
			return &dataservice.OperationDetail{
				ID:          1,
				RouteName:   &trainName,
				CarrierCode: &carrierCode,
				TrainStatus: "P",
				Stations: []dataservice.OperationStation{
					{StationExternalID: 100, StationName: &station1, ActualSequenceNumber: 1, IsConfirmed: true},
					{StationExternalID: 200, StationName: &station2, ActualSequenceNumber: 2},
				},
			}, nil
		},
	}

	svc := New(mockClient)
	resp, err := svc.GetLiveTrains(context.Background(), nil, nil, 20, 0)
	if err != nil {
		t.Fatalf("GetLiveTrains returned error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Data))
	}
	if resp.Data[0].CurrentStation == nil || *resp.Data[0].CurrentStation != station1 {
		t.Fatalf("expected current_station %q, got %v", station1, resp.Data[0].CurrentStation)
	}
	if resp.Data[0].NextStation == nil || *resp.Data[0].NextStation != station2 {
		t.Fatalf("expected next_station %q, got %v", station2, resp.Data[0].NextStation)
	}
}

func ptr(v string) *string {
	return &v
}
