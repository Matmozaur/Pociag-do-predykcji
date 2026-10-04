package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/pociag-do-predykcji/services/go/api/internal/model"
	"github.com/pociag-do-predykcji/services/go/api/internal/position"
	"github.com/pociag-do-predykcji/services/go/api/internal/service"
	"github.com/pociag-do-predykcji/services/go/api/internal/service/servicetest"
)

// now is 14:00 in Warsaw.
var now = ts("2026-10-04T12:00:00Z")

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func tp(s string) *time.Time { t := ts(s); return &t }
func sp(s string) *string    { return &s }
func ip(v int) *int          { return &v }
func fp(v float64) *float64  { return &v }
func dp(d time.Duration) *time.Duration {
	return &d
}

func equal[T any](t *testing.T, name string, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", name, got, want)
	}
}

func deref[T any](v *T) any {
	if v == nil {
		return nil
	}
	return *v
}

func TestTrainName(t *testing.T) {
	cases := []struct {
		train service.Train
		want  string
	}{
		{service.Train{Name: sp("Pendolino"), Category: sp("EIP"), Number: sp("1300")}, "Pendolino"},
		{service.Train{Name: sp("  "), Category: sp("IC"), Number: sp("5300")}, "IC 5300"},
		{service.Train{Number: sp("80860")}, "Pociąg 80860"},
		{service.Train{Category: sp("R"), ScheduleID: 2026, OrderID: 42}, "Pociąg 2026/42"},
	}
	for _, c := range cases {
		equal(t, "trainName", service.TrainName(c.train), c.want)
	}
}

func TestOffsetClock(t *testing.T) {
	equal(t, "08:05", deref(service.OffsetClock(dp(8*time.Hour+5*time.Minute))), any("08:05"))
	equal(t, "next day", deref(service.OffsetClock(dp(25*time.Hour+2*time.Minute))), any("01:02"))
	equal(t, "previous day", deref(service.OffsetClock(dp(-10*time.Minute))), any("23:50"))
	equal(t, "nil", service.OffsetClock(nil), (*string)(nil))
}

// ── Station board ─────────────────────────────────────────────────────────────

func boardStop(opID int64, stop position.Stop) service.BoardStop {
	return service.BoardStop{
		Run: service.Run{
			OperationID: opID, Status: "P", UpdatedAt: ts("2026-10-04T11:55:00Z").Add(time.Duration(opID) * time.Second),
			Train: service.Train{ScheduleID: 2026, OrderID: int(opID), Category: sp("IC"), Number: sp("100"),
				CarrierCode: sp("IC"), CarrierName: sp("PKP Intercity")},
		},
		Stop:     stop,
		Platform: sp("2"), Track: sp("4"),
		Origin: sp("Kraków Główny"), Destination: sp("Gdynia Główna"),
	}
}

func operationIDs(rows []model.StationBoardRow) []int64 {
	ids := []int64{}
	for _, r := range rows {
		ids = append(ids, r.OperationID)
	}
	return ids
}

func TestStationBoardBuckets(t *testing.T) {
	repo := &servicetest.Repository{
		Station: &service.Station{ID: 5100, Name: "Warszawa Centralna", Latitude: fp(52.23), Longitude: fp(21.0)},
		BoardStops: []service.BoardStop{
			// 1: arrived, departs in 3 min → at station.
			boardStop(1, position.Stop{PlannedArrival: tp("2026-10-04T11:55:00Z"), ActualArrival: tp("2026-10-04T11:55:00Z"),
				PlannedDeparture: tp("2026-10-04T12:03:00Z"), ActualDeparture: tp("2026-10-04T12:03:00Z"), IsConfirmed: true}),
			// 2: through train, 3 min late, no actual times → arrival and departure.
			boardStop(2, position.Stop{PlannedArrival: tp("2026-10-04T12:10:00Z"), PlannedDeparture: tp("2026-10-04T12:12:00Z"),
				ArrivalDelayMinutes: ip(3), DepartureDelayMinutes: ip(3)}),
			// 3: terminus, arrived 2 min ago → at station.
			boardStop(3, position.Stop{PlannedArrival: tp("2026-10-04T11:58:00Z")}),
			// 4: terminus, arrived 10 min ago → gone.
			boardStop(4, position.Stop{PlannedArrival: tp("2026-10-04T11:50:00Z")}),
			// 5: starts here in 20 min → departure.
			boardStop(5, position.Stop{PlannedDeparture: tp("2026-10-04T12:20:00Z")}),
			// 6: cancelled stop → listed, never at station.
			boardStop(6, position.Stop{PlannedArrival: tp("2026-10-04T12:05:00Z"), PlannedDeparture: tp("2026-10-04T12:06:00Z"),
				IsCancelled: true}),
		},
	}

	board, err := service.NewAt(repo, now).StationBoard(context.Background(), 5100, 2)
	if err != nil {
		t.Fatal(err)
	}

	equal(t, "window", repo.BoardWindow, [2]time.Time{ts("2026-10-04T06:00:00Z"), ts("2026-10-05T00:00:00Z")})
	equal(t, "at_station", operationIDs(board.AtStation), []int64{3, 1})
	equal(t, "arrivals", operationIDs(board.Arrivals), []int64{6, 2})
	equal(t, "departures (truncated)", operationIDs(board.Departures), []int64{6, 2})

	arrival, departure := board.Arrivals[1], board.Departures[1]
	equal(t, "arrival planned", deref(arrival.PlannedTime), any("14:10"))
	equal(t, "arrival expected", deref(arrival.ExpectedTime), any("14:13"))
	equal(t, "departure planned", deref(departure.PlannedTime), any("14:12"))
	equal(t, "departure expected at", deref(departure.ExpectedAt), any(ts("2026-10-04T12:15:00Z")))
	equal(t, "delay", deref(departure.DelayMinutes), any(3))
	equal(t, "train name", departure.TrainName, "IC 100")
	equal(t, "carrier", *departure.Carrier, model.Carrier{Code: "IC", Name: sp("PKP Intercity")})
	equal(t, "platform", deref(departure.Platform), any("2"))
	equal(t, "cancelled", board.Arrivals[0].IsCancelled, true)
	equal(t, "data as of", deref(board.DataAsOf), any(ts("2026-10-04T11:55:06Z")))
	equal(t, "station", board.Station.Name, "Warszawa Centralna")
}

func TestStationBoardUnknownStation(t *testing.T) {
	_, err := service.NewAt(&servicetest.Repository{}, now).StationBoard(context.Background(), 1, 10)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// ── Active trains ─────────────────────────────────────────────────────────────

func run(opID int64, carrier string, updated string, stops ...position.Stop) service.Run {
	return service.Run{
		OperationID: opID, OperatingDate: ts("2026-10-04T00:00:00Z"), Status: "P", UpdatedAt: ts(updated),
		Train: service.Train{ScheduleID: 2026, OrderID: int(opID), Number: sp("1"), CarrierCode: sp(carrier)},
		Stops: stops,
	}
}

func activeRepo() *servicetest.Repository {
	a := position.Stop{StationID: 1, StationName: sp("A"), SequenceNumber: 1,
		PlannedDeparture: tp("2026-10-04T11:28:00Z"), ActualDeparture: tp("2026-10-04T11:30:00Z"),
		DepartureDelayMinutes: ip(2), IsConfirmed: true, Latitude: fp(52), Longitude: fp(21)}
	b := position.Stop{StationID: 2, StationName: sp("B"), SequenceNumber: 2,
		PlannedArrival: tp("2026-10-04T12:30:00Z"), Latitude: fp(52), Longitude: fp(22)}
	noCoords := func(s position.Stop) position.Stop { s.Latitude, s.Longitude = nil, nil; return s }

	return &servicetest.Repository{
		Snapshot: ts("2026-10-04T11:58:00Z"),
		ActiveRuns: []service.Run{
			run(1, "KM", "2026-10-04T11:58:00Z", a, b),
			run(2, "KM", "2026-10-04T11:58:00Z", noCoords(a), noCoords(b)),
			run(3, "IC", "2026-10-04T11:58:00Z", a, b),
			run(4, "KM", "2026-10-04T11:30:00Z", a, b), // missing from recent snapshots
		},
	}
}

func TestLiveTrains(t *testing.T) {
	repo := activeRepo()
	resp, err := service.NewAt(repo, now).LiveTrains(context.Background(), nil, 2, 0)
	if err != nil {
		t.Fatal(err)
	}

	equal(t, "seen since", repo.ActiveSeenSince, ts("2026-10-04T11:43:00Z"))
	equal(t, "pagination", resp.Pagination, model.Pagination{Total: 3, Limit: 2, Offset: 0, HasMore: true})
	first := resp.Data[0]
	equal(t, "operation", first.OperationID, int64(1))
	equal(t, "name", first.TrainName, "Pociąg 1")
	equal(t, "status", first.Status, "in_progress")
	equal(t, "current", deref(first.CurrentStation), any("A"))
	equal(t, "next", deref(first.NextStation), any("B"))
	equal(t, "delay", deref(first.DelayMinutes), any(2))
	equal(t, "origin", deref(first.Origin), any("A"))
	equal(t, "destination", deref(first.Destination), any("B"))
}

func TestMapTrains(t *testing.T) {
	resp, err := service.NewAt(activeRepo(), now).MapTrains(context.Background(), []string{"km"})
	if err != nil {
		t.Fatal(err)
	}

	equal(t, "trains", len(resp.Trains), 1)
	equal(t, "unpositioned", resp.UnpositionedCount, 1)
	equal(t, "data as of", deref(resp.DataAsOf), any(ts("2026-10-04T11:58:00Z")))
	train := resp.Trains[0]
	equal(t, "phase", train.Phase, position.PhaseEnRoute)
	equal(t, "longitude halfway", train.Longitude, 21.5)
	equal(t, "method", train.Method, position.MethodInterpolated)
	equal(t, "previous stop time", train.PreviousStop.Time, ts("2026-10-04T11:30:00Z"))
	equal(t, "next stop", deref(train.NextStop.StationName), any("B"))
}

func TestActiveTrainsWithoutOperations(t *testing.T) {
	resp, err := service.NewAt(&servicetest.Repository{}, now).MapTrains(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, "trains", len(resp.Trains), 0)
	equal(t, "data as of", resp.DataAsOf, (*time.Time)(nil))
}

func TestTrainDetail(t *testing.T) {
	r := run(7, "KM", "2026-10-04T11:58:00Z",
		position.Stop{StationID: 1, StationName: sp("A"), SequenceNumber: 1, PlannedDeparture: tp("2026-10-04T11:28:00Z"),
			ActualDeparture: tp("2026-10-04T11:30:00Z"), DepartureDelayMinutes: ip(2), IsConfirmed: true},
		position.Stop{StationID: 2, StationName: sp("B"), SequenceNumber: 3, PlannedArrival: tp("2026-10-04T12:30:00Z"),
			IsCancelled: true},
	)
	r.Train.CarrierName = sp("Koleje Mazowieckie")
	detail, err := service.NewAt(&servicetest.Repository{Run: &r}, now).TrainDetail(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}

	equal(t, "operating date", detail.OperatingDate, "2026-10-04")
	equal(t, "carrier", *detail.Carrier, model.Carrier{Code: "KM", Name: sp("Koleje Mazowieckie")})
	equal(t, "stops", len(detail.Stops), 2)
	equal(t, "planned departure", deref(detail.Stops[0].PlannedDeparture), any("13:28"))
	equal(t, "actual departure", deref(detail.Stops[0].ActualDeparture), any("13:30"))
	equal(t, "sequence", detail.Stops[1].Sequence, 3)
	equal(t, "cancelled", detail.Stops[1].IsCancelled, true)
}

// ── Schedules ─────────────────────────────────────────────────────────────────

func connection(trainID int64, from, to time.Duration, fromSeq, toSeq int) service.Connection {
	return service.Connection{
		Train: service.Train{ID: trainID, ScheduleID: 2026, OrderID: int(trainID), Number: sp("1"), CarrierCode: sp("IC")},
		From:  service.ScheduleStop{Seq: fromSeq, StationID: 5100, StationName: "Warszawa Centralna", Departure: &from},
		To:    service.ScheduleStop{Seq: toSeq, StationID: 6000, StationName: "Kraków Główny", Arrival: &to},
	}
}

func TestSearchSchedules(t *testing.T) {
	repo := &servicetest.Repository{Connections: []service.Connection{
		connection(10, 8*time.Hour, 10*time.Hour+30*time.Minute, 2, 5),
		connection(11, 23*time.Hour+50*time.Minute, 25*time.Hour+10*time.Minute, 1, 2),
		connection(12, 7*time.Hour, 12*time.Hour, 1, 9),
	}}
	svc := service.NewAt(repo, now)
	q := service.ConnectionQuery{
		Date: ts("2026-10-05T00:00:00Z"), From: service.ParsePlace("Warszawa"), To: service.ParsePlace(" 6000 "),
	}

	byDuration, err := svc.SearchSchedules(context.Background(), q, service.SortDuration, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, "query", repo.Query.To, service.Place{StationID: 6000})
	equal(t, "echo", byDuration.Query, model.ScheduleSearchQuery{From: "Warszawa", To: "6000", Date: "2026-10-05"})
	equal(t, "pagination", byDuration.Pagination, model.Pagination{Total: 3, Limit: 2, Offset: 1})
	equal(t, "order", []int64{byDuration.Data[0].RouteID, byDuration.Data[1].RouteID}, []int64{10, 12})
	equal(t, "duration", byDuration.Data[0].DurationMinutes, 150)
	equal(t, "stops between", byDuration.Data[0].StopsCount, 2)

	byDeparture, err := svc.SearchSchedules(context.Background(), q, service.SortDeparture, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	last := byDeparture.Data[2]
	equal(t, "latest departure", last.RouteID, int64(11))
	equal(t, "after midnight", last.Arrival.Time, "01:10")
	equal(t, "overnight duration", last.DurationMinutes, 80)
}

func TestScheduleDetail(t *testing.T) {
	repo := &servicetest.Repository{Schedule: &service.TrainSchedule{
		Train: service.Train{ID: 3, Category: sp("Os"), Number: sp("69967"), CarrierCode: sp("KD")},
		OperatingDates: []time.Time{
			ts("2026-10-03T00:00:00Z"), ts("2026-10-04T00:00:00Z"), ts("2026-10-05T00:00:00Z"),
		},
		Stops: []service.ScheduleStop{
			{Seq: 1, StationID: 60103, StationName: "Wrocław Główny", Departure: dp(23*time.Hour + 15*time.Minute), Platform: sp("1")},
			{Seq: 2, StationID: 59394, StationName: "Oleśnica", Arrival: dp(24*time.Hour + 2*time.Minute)},
		},
	}}

	detail, err := service.NewAt(repo, now).ScheduleDetail(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, "name", detail.TrainName, "Os 69967")
	equal(t, "upcoming dates", detail.OperatingDates, []string{"2026-10-04", "2026-10-05"})
	equal(t, "departure", deref(detail.Stops[0].DepartureTime), any("23:15"))
	equal(t, "arrival", deref(detail.Stops[1].ArrivalTime), any("00:02"))
	equal(t, "duration", deref(detail.TotalDurationMinutes), any(47))
}

// ── Disruptions & dashboard ───────────────────────────────────────────────────

func TestDisruptions(t *testing.T) {
	repo := &servicetest.Repository{Disruptions: []service.Disruption{
		{ID: 1, Message: "a", AffectedTrains: 12, DateFrom: tp("2026-10-04T00:00:00Z")},
		{ID: 2, Message: "b", AffectedTrains: 3},
		{ID: 3, Message: "c", AffectedTrains: 1},
	}}
	resp, err := service.NewAt(repo, now).Disruptions(context.Background(), 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	severities := []string{}
	for _, d := range resp.Data {
		severities = append(severities, d.Severity)
	}
	equal(t, "severity", severities, []string{"high", "medium", "low"})
	equal(t, "date", deref(resp.Data[0].DateFrom), any("2026-10-04"))
	equal(t, "total", resp.Pagination.Total, 3)
}

func TestDashboardOverview(t *testing.T) {
	repo := &servicetest.Repository{
		Statistics: service.DayStatistics{Total: 10, InProgress: 2, Completed: 2, Cancelled: 1, Started: 4, OnTime: 3},
		Freshness:  service.Freshness{OperationsUpdatedAt: tp("2026-10-04T11:58:00Z"), Disruptions: 7},
	}
	// 00:30 in Warsaw on 5 October: the operating date is already the 5th.
	overview, err := service.NewAt(repo, ts("2026-10-04T22:30:00Z")).DashboardOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	equal(t, "stats date", repo.StatsDate, ts("2026-10-05T00:00:00Z"))
	equal(t, "seen since", repo.StatsSeenSince, ts("2026-10-04T11:43:00Z"))
	equal(t, "date", overview.Statistics.Date, "2026-10-05")
	equal(t, "on time", deref(overview.Statistics.OnTimePercentage), any(75.0))
	equal(t, "in progress", overview.Statistics.InProgress, 2)
	equal(t, "disruptions", overview.DisruptionsActive, 7)
}
