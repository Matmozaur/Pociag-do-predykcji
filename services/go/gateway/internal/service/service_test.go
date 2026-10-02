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
	getStationBoardFn        func(ctx context.Context, externalID int, limit int) (*dataservice.StationBoardResponse, error)
	getRouteByKeyFn          func(ctx context.Context, scheduleID, orderID int) (*dataservice.RouteDetail, error)
	listCarriersFn           func(ctx context.Context) (*dataservice.CarrierListResponse, error)
	getOperationStatsFn      func(ctx context.Context, date string) (*dataservice.OperationStatistics, error)
	queryDisruptionsFn       func(ctx context.Context, p dataservice.QueryDisruptionsParams) (*dataservice.DisruptionListResponse, error)
	getDisruptionByIDFn      func(ctx context.Context, disruptionID int64) (*dataservice.DisruptionDetail, error)
	queryOperationsFn        func(ctx context.Context, p dataservice.QueryOperationsParams) (*dataservice.OperationListResponse, error)
	getOperationByIDFn       func(ctx context.Context, operationID int64) (*dataservice.OperationDetail, error)
	listActiveOperationsFn   func(ctx context.Context, p dataservice.ListActiveOperationsParams) (*dataservice.ActiveTrainListResponse, error)
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

func (m *mockDataServiceClient) GetStationBoard(ctx context.Context, externalID int, limit int) (*dataservice.StationBoardResponse, error) {
	return m.getStationBoardFn(ctx, externalID, limit)
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

func (m *mockDataServiceClient) ListActiveOperations(ctx context.Context, p dataservice.ListActiveOperationsParams) (*dataservice.ActiveTrainListResponse, error) {
	return m.listActiveOperationsFn(ctx, p)
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

func TestGetStationBoard_ShapesSections(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	at := func(hh, mm int) *time.Time {
		v := time.Date(2026, 9, 29, hh, mm, 0, 0, loc)
		return &v
	}
	intPtr := func(v int) *int { return &v }
	dataAsOf := time.Date(2026, 9, 29, 18, 0, 0, 0, time.UTC)

	var gotLimit int
	mockClient := &mockDataServiceClient{
		getStationByExternalIDFn: func(ctx context.Context, externalID int) (*dataservice.Station, error) {
			return &dataservice.Station{ExternalID: externalID, Name: "Kraków Główny", City: ptr("Kraków")}, nil
		},
		getStationBoardFn: func(ctx context.Context, externalID int, limit int) (*dataservice.StationBoardResponse, error) {
			gotLimit = limit
			return &dataservice.StationBoardResponse{
				StationExternalID: externalID,
				DataAsOf:          &dataAsOf,
				Entries: []dataservice.StationBoardEntry{
					// Terminus standing at the platform: no departure, falls back to arrival fields.
					{Bucket: "at_station", OperationID: 1, ScheduleID: 2026, OrderID: 1, TrainStatus: "P",
						RouteName: ptr("Pieniny"), CarrierCode: ptr("IC"),
						PlannedArrival: at(20, 0), ExpectedArrival: at(20, 5), ArrivalDelayMinutes: intPtr(5),
						ArrivalPlatform: ptr("II"), ArrivalTrack: ptr("3"), IsConfirmed: true},
					// Through train: arrival row uses arrival fields, departure row departure fields.
					{Bucket: "arrival", OperationID: 2, ScheduleID: 2026, OrderID: 2, TrainStatus: "P",
						CommercialCategory: ptr("TLK"), NationalNumber: ptr("31100"), CarrierCode: ptr("XX"),
						PlannedArrival: at(20, 10), ExpectedArrival: at(20, 12), ArrivalDelayMinutes: intPtr(2),
						ArrivalPlatform: ptr("I"), ArrivalTrack: ptr("1"),
						PlannedDeparture: at(20, 15), ExpectedDeparture: at(20, 17), DepartureDelayMinutes: intPtr(2),
						DeparturePlatform: ptr("IV"), DepartureTrack: ptr("7")},
					{Bucket: "departure", OperationID: 2, ScheduleID: 2026, OrderID: 2, TrainStatus: "P",
						CommercialCategory: ptr("TLK"), NationalNumber: ptr("31100"), CarrierCode: ptr("XX"),
						PlannedArrival: at(20, 10), ExpectedArrival: at(20, 12), ArrivalDelayMinutes: intPtr(2),
						ArrivalPlatform: ptr("I"), ArrivalTrack: ptr("1"),
						PlannedDeparture: at(20, 15), ExpectedDeparture: at(20, 17), DepartureDelayMinutes: intPtr(2),
						DeparturePlatform: ptr("IV"), DepartureTrack: ptr("7")},
					// Missing route: only train number known; cancelled stop; no expected time.
					{Bucket: "departure", OperationID: 3, ScheduleID: 2026, OrderID: 3, TrainStatus: "S",
						TrainNumber: ptr("91234"), PlannedDeparture: at(21, 0), IsCancelled: true},
					// Missing route and train number: schedule/order fallback.
					{Bucket: "departure", OperationID: 4, ScheduleID: 2026, OrderID: 4, TrainStatus: "S",
						PlannedDeparture: at(21, 30)},
				},
			}, nil
		},
		listCarriersFn: func(ctx context.Context) (*dataservice.CarrierListResponse, error) {
			return &dataservice.CarrierListResponse{Data: []dataservice.Carrier{{Code: "IC", Name: "PKP Intercity"}}}, nil
		},
	}

	svc := New(mockClient)
	resp, err := svc.GetStationBoard(context.Background(), 33506, 7)
	if err != nil {
		t.Fatalf("GetStationBoard returned error: %v", err)
	}
	if gotLimit != 7 {
		t.Fatalf("expected limit 7 passed to data-service, got %d", gotLimit)
	}
	if resp.Station.ExternalID != 33506 || resp.Station.Name != "Kraków Główny" {
		t.Fatalf("unexpected station %+v", resp.Station)
	}
	if resp.DataAsOf == nil || !resp.DataAsOf.Equal(dataAsOf) {
		t.Fatalf("expected data_as_of %v, got %v", dataAsOf, resp.DataAsOf)
	}
	if len(resp.AtStation) != 1 || len(resp.Arrivals) != 1 || len(resp.Departures) != 3 {
		t.Fatalf("unexpected section sizes: at_station=%d arrivals=%d departures=%d", len(resp.AtStation), len(resp.Arrivals), len(resp.Departures))
	}

	term := resp.AtStation[0]
	if term.TrainName != "Pieniny" || *term.PlannedTime != "20:00" || *term.ExpectedTime != "20:05" ||
		*term.DelayMinutes != 5 || *term.Platform != "II" || *term.Track != "3" {
		t.Fatalf("terminus row should use arrival fields, got %+v", term)
	}
	if term.Carrier == nil || *term.Carrier.Code != "IC" || term.Carrier.Name == nil || *term.Carrier.Name != "PKP Intercity" {
		t.Fatalf("expected resolved carrier IC / PKP Intercity, got %+v", term.Carrier)
	}
	if term.Status != "in_progress" || !term.IsConfirmed {
		t.Fatalf("unexpected status fields %+v", term)
	}
	if term.ExpectedAt == nil || term.ExpectedAt.Location() != time.UTC || !term.ExpectedAt.Equal(*at(20, 5)) {
		t.Fatalf("expected expected_at %v in UTC, got %v", at(20, 5), term.ExpectedAt)
	}

	arr := resp.Arrivals[0]
	if arr.TrainName != "TLK 31100" || *arr.PlannedTime != "20:10" || *arr.ExpectedTime != "20:12" || *arr.Platform != "I" || *arr.Track != "1" {
		t.Fatalf("arrival row should use arrival fields, got %+v", arr)
	}
	if arr.Carrier == nil || *arr.Carrier.Code != "XX" || arr.Carrier.Name != nil {
		t.Fatalf("unknown carrier should keep code without name, got %+v", arr.Carrier)
	}

	dep := resp.Departures[0]
	if *dep.PlannedTime != "20:15" || *dep.ExpectedTime != "20:17" || *dep.Platform != "IV" || *dep.Track != "7" {
		t.Fatalf("departure row should use departure fields, got %+v", dep)
	}

	noRoute := resp.Departures[1]
	if noRoute.TrainName != "Pociąg 91234" || noRoute.Carrier != nil || !noRoute.IsCancelled || noRoute.Status != "not_started" {
		t.Fatalf("unexpected missing-route row %+v", noRoute)
	}
	if noRoute.ExpectedTime == nil || *noRoute.ExpectedTime != "21:00" {
		t.Fatalf("expected time should fall back to planned, got %v", noRoute.ExpectedTime)
	}
	if resp.Departures[2].TrainName != "Pociąg 2026/4" {
		t.Fatalf("expected schedule/order fallback name, got %q", resp.Departures[2].TrainName)
	}
}

func TestGetStationBoard_EmptyBoardReturnsEmptySections(t *testing.T) {
	mockClient := &mockDataServiceClient{
		getStationBoardFn: func(ctx context.Context, externalID int, limit int) (*dataservice.StationBoardResponse, error) {
			return &dataservice.StationBoardResponse{}, nil
		},
	}

	resp, err := New(mockClient).GetStationBoard(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetStationBoard returned error: %v", err)
	}
	if resp.AtStation == nil || resp.Arrivals == nil || resp.Departures == nil {
		t.Fatalf("expected non-nil empty sections, got %+v", resp)
	}
}

func TestGetStationBoard_StationNotFound_ReturnsErrNotFound(t *testing.T) {
	mockClient := &mockDataServiceClient{
		getStationByExternalIDFn: func(ctx context.Context, externalID int) (*dataservice.Station, error) {
			return nil, dataservice.ErrNotFound
		},
		getStationBoardFn: func(ctx context.Context, externalID int, limit int) (*dataservice.StationBoardResponse, error) {
			t.Fatal("board should not be requested for a missing station")
			return nil, nil
		},
	}

	_, err := New(mockClient).GetStationBoard(context.Background(), 1, 10)
	if !errors.Is(err, dataservice.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetMapTrains_MapsPositionedTrainsAndCountsUnpositioned(t *testing.T) {
	generatedAt := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)
	dataAsOf := generatedAt.Add(-3 * time.Minute)
	prevTime := generatedAt.Add(-5 * time.Minute)
	nextTime := generatedAt.Add(7 * time.Minute)
	lat, lon := 50.067, 19.945
	delay := 4
	var gotParams dataservice.ListActiveOperationsParams
	mockClient := &mockDataServiceClient{
		listActiveOperationsFn: func(ctx context.Context, p dataservice.ListActiveOperationsParams) (*dataservice.ActiveTrainListResponse, error) {
			gotParams = p
			return &dataservice.ActiveTrainListResponse{
				Data: []dataservice.ActiveTrain{
					{
						OperationID:  1,
						ScheduleID:   2026,
						OrderID:      11,
						TrainStatus:  "P",
						RouteName:    ptr("Kraków Główny - Warszawa Centralna"),
						CarrierCode:  ptr("IC"),
						Origin:       &dataservice.StopRef{StationExternalID: 1, StationName: ptr("Kraków Główny")},
						Destination:  &dataservice.StopRef{StationExternalID: 2, StationName: ptr("Warszawa Centralna")},
						Phase:        "en_route",
						PreviousStop: &dataservice.StopTiming{StationExternalID: 1, StationName: ptr("Kraków Główny"), Time: prevTime, Latitude: &lat, Longitude: &lon},
						NextStop:     &dataservice.StopTiming{StationExternalID: 3, Time: nextTime},
						DelayMinutes: &delay,
						Position:     &dataservice.TrainPosition{Latitude: 50.1, Longitude: 19.9, Progress: 0.4, Method: "interpolated"},
						Confidence:   "high",
					},
					{OperationID: 2, ScheduleID: 2026, OrderID: 12, TrainStatus: "S", Phase: "not_departed", Confidence: "low"},
					{
						OperationID: 3, ScheduleID: 2026, OrderID: 13, TrainStatus: "S", RouteName: ptr("  "),
						Phase: "at_station", Confidence: "medium",
						Position: &dataservice.TrainPosition{Latitude: 52.2, Longitude: 21.0, Method: "station"},
					},
					{
						OperationID: 4, ScheduleID: 2026, OrderID: 14, TrainStatus: "P", CarrierCode: ptr("KM"),
						Phase: "en_route", Confidence: "medium",
						Position: &dataservice.TrainPosition{Latitude: 52.0, Longitude: 20.0, Progress: 0.5, Method: "interpolated_sparse"},
					},
				},
				Total:       4,
				GeneratedAt: generatedAt,
				DataAsOf:    dataAsOf,
			}, nil
		},
	}

	resp, err := New(mockClient).GetMapTrains(context.Background(), []string{"IC", "KM"})
	if err != nil {
		t.Fatalf("GetMapTrains returned error: %v", err)
	}
	if len(gotParams.CarrierCodes) != 2 || gotParams.CarrierCodes[0] != "IC" || gotParams.Limit != mapTrainsLimit {
		t.Fatalf("unexpected params: %+v", gotParams)
	}
	if resp.UnpositionedCount != 1 {
		t.Fatalf("expected 1 unpositioned train, got %d", resp.UnpositionedCount)
	}
	if !resp.GeneratedAt.Equal(generatedAt) || !resp.DataAsOf.Equal(dataAsOf) {
		t.Fatalf("unexpected timestamps: %v %v", resp.GeneratedAt, resp.DataAsOf)
	}
	if len(resp.Trains) != 3 {
		t.Fatalf("expected 3 positioned trains, got %d", len(resp.Trains))
	}

	first := resp.Trains[0]
	if first.OperationID != 1 || first.TrainName != "Kraków Główny - Warszawa Centralna" || first.Status != "in_progress" ||
		first.Phase != "en_route" || first.Latitude != 50.1 || first.Longitude != 19.9 || first.Progress != 0.4 ||
		first.Method != "interpolated" || first.Confidence != "high" || *first.DelayMinutes != 4 || *first.CarrierCode != "IC" {
		t.Fatalf("unexpected first train: %+v", first)
	}
	if first.Origin == nil || *first.Origin != "Kraków Główny" || first.Destination == nil || *first.Destination != "Warszawa Centralna" {
		t.Fatalf("unexpected origin/destination: %v %v", first.Origin, first.Destination)
	}
	if first.PreviousStop == nil || !first.PreviousStop.Time.Equal(prevTime) || *first.PreviousStop.Latitude != lat {
		t.Fatalf("unexpected previous stop: %+v", first.PreviousStop)
	}
	if first.NextStop == nil || !first.NextStop.Time.Equal(nextTime) || first.NextStop.StationName != nil || first.NextStop.Latitude != nil {
		t.Fatalf("unexpected next stop: %+v", first.NextStop)
	}

	if got := resp.Trains[1]; got.TrainName != "Pociąg 2026/13" || got.Status != "not_started" || got.PreviousStop != nil || got.Origin != nil {
		t.Fatalf("expected fallback name without carrier, got %+v", got)
	}
	if got := resp.Trains[2].TrainName; got != "KM 2026/14" {
		t.Fatalf("expected carrier fallback name, got %q", got)
	}
}

func TestGetMapTrains_ClientError_ReturnsError(t *testing.T) {
	mockClient := &mockDataServiceClient{
		listActiveOperationsFn: func(ctx context.Context, p dataservice.ListActiveOperationsParams) (*dataservice.ActiveTrainListResponse, error) {
			return nil, errors.New("boom")
		},
	}

	if _, err := New(mockClient).GetMapTrains(context.Background(), nil); err == nil {
		t.Fatal("expected error")
	}
}

func ptr(v string) *string {
	return &v
}
