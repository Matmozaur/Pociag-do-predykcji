package repository_test

// SQL against a real PostgreSQL with db/migrations applied. Skipped unless
// POCIAG_TEST_DATABASE_URL points at a disposable database; its public schema is recreated.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pociag-do-predykcji/services/go/api/internal/position"
	"github.com/pociag-do-predykcji/services/go/api/internal/repository"
	"github.com/pociag-do-predykcji/services/go/api/internal/service"
)

// at is 14:00 in Warsaw on 4 October 2026.
var at = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

const seed = `
INSERT INTO stations (id, name, city, latitude, longitude) VALUES
    (1, 'Warszawa Centralna', 'Warszawa', 52.23, 21.00),
    (2, 'Warszawa Wschodnia', 'Warszawa', 52.25, 21.05),
    (3, 'Łódź Fabryczna', 'Łódź', 51.77, 19.47),
    (4, 'Kraków Główny', NULL, 50.07, 19.95),
    (5, 'Koluszki', NULL, NULL, NULL);

INSERT INTO trains (id, schedule_id, order_id, name, number, category, carrier_code, carrier_name, operating_dates)
VALUES
    (10, 2026, 100, NULL, '1300', 'IC', 'IC', 'PKP Intercity', '{2026-10-04,2026-10-05}'),
    (11, 2026, 101, 'Łodzianka', '5000', 'R', 'PR', 'POLREGIO', '{2026-10-04}'),
    (12, 2026, 102, NULL, NULL, NULL, NULL, NULL, '{}');

-- Train 10: Warszawa Wschodnia 13:00 → Warszawa Centralna 13:10 → Koluszki → Kraków Główny 16:00.
-- Train 11: Łódź Fabryczna 13:30 → Koluszki → Warszawa Centralna 14:30 (next day offsets for one stop).
INSERT INTO schedule_stops (train_id, seq, station_id, arrival, departure, platform, track) VALUES
    (10, 1, 2, NULL, '13:00', '1', '2'),
    (10, 2, 1, '13:08', '13:10', '3', '5'),
    (10, 3, 5, '13:50', '13:51', NULL, NULL),
    (10, 4, 4, '16:00', NULL, '4', '1'),
    (11, 1, 3, NULL, '13:30', '2', NULL),
    (11, 2, 5, '14:00', '14:01', NULL, NULL),
    (11, 3, 1, '1 day 00:30', NULL, '6', '12');

INSERT INTO operations (id, train_id, operating_date, status, updated_at) VALUES
    (100, 10, '2026-10-04', 'P', '2026-10-04T11:58:00Z'),
    (101, 11, '2026-10-04', 'S', '2026-10-04T11:58:00Z'),
    (102, 12, '2026-10-04', 'P', '2026-10-04T11:00:00Z'),
    (103, 10, '2026-10-03', 'X', '2026-10-04T11:58:00Z'),
    (104, 11, '2026-10-03', 'C', '2026-10-04T11:58:00Z');

INSERT INTO operation_stops (operation_id, seq, station_id, planned_arrival, planned_departure,
    actual_arrival, actual_departure, arrival_delay, departure_delay, is_confirmed, is_cancelled) VALUES
    (100, 1, 2, NULL, '2026-10-04T11:00Z', NULL, '2026-10-04T11:04Z', NULL, 4, true, false),
    (100, 2, 1, '2026-10-04T11:08Z', '2026-10-04T11:10Z', '2026-10-04T11:12Z', '2026-10-04T11:14Z', 4, 4, true, false),
    (100, 3, 5, '2026-10-04T11:50Z', '2026-10-04T11:51Z', NULL, NULL, 6, 6, false, true),
    (100, 4, 4, '2026-10-04T14:00Z', NULL, '2026-10-04T14:06Z', NULL, 6, NULL, false, false),
    (101, 1, 3, NULL, '2026-10-04T13:30Z', NULL, '2026-10-04T13:30Z', NULL, NULL, false, false),
    (101, 2, 1, '2026-10-04T14:30Z', NULL, '2026-10-04T14:30Z', NULL, NULL, NULL, false, false),
    (102, 1, 3, NULL, '2026-10-04T11:30Z', NULL, NULL, NULL, NULL, false, false),
    (102, 2, 1, '2026-10-04T12:30Z', NULL, NULL, NULL, NULL, NULL, false, false),
    (103, 1, 1, '2026-10-04T11:30Z', '2026-10-04T11:31Z', NULL, NULL, NULL, NULL, false, true),
    (104, 1, 1, '2026-10-04T10:00Z', NULL, '2026-10-04T10:20Z', NULL, 20, NULL, true, false);

INSERT INTO disruptions (id, type_code, start_station_id, end_station_id, message, date_from, date_to, affected_trains)
VALUES
    (1, 'utr_32', 3, 5, 'Komunikacja zastępcza', '2026-10-04', '2026-10-05', 12),
    (2, NULL, NULL, NULL, 'Awaria', '2026-10-04', '2026-10-04', 1);
`

func setup(t *testing.T) *repository.Repository {
	t.Helper()
	url := os.Getenv("POCIAG_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("POCIAG_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	migration, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "db", "migrations", "001_init.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{"DROP SCHEMA public CASCADE; CREATE SCHEMA public;", string(migration), seed} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	return repository.New(pool)
}

func equal[T any](t *testing.T, name string, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", name, got, want)
	}
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestRepository(t *testing.T) {
	repo := setup(t)
	ctx := context.Background()

	t.Run("search stations ranks prefix matches first", func(t *testing.T) {
		stations, err := repo.SearchStations(ctx, "warszawa", 10)
		check(t, err)
		equal(t, "names", []string{stations[0].Name, stations[1].Name}, []string{"Warszawa Centralna", "Warszawa Wschodnia"})
		stations, err = repo.SearchStations(ctx, "łódź", 10)
		check(t, err)
		equal(t, "by city", len(stations), 1)
	})

	t.Run("mapped stations have coordinates", func(t *testing.T) {
		stations, err := repo.ListMappedStations(ctx)
		check(t, err)
		equal(t, "count", len(stations), 4)
		_, err = repo.GetStation(ctx, 99)
		equal(t, "missing", errors.Is(err, service.ErrNotFound), true)
	})

	t.Run("board stops", func(t *testing.T) {
		stops, err := repo.ListBoardStops(ctx, 1, at.Add(-6*time.Hour), at.Add(12*time.Hour))
		check(t, err)
		ops := map[int64]service.BoardStop{}
		for _, s := range stops {
			ops[s.Run.OperationID] = s
		}
		// 103 is fully cancelled (X); 100, 101, 102 and 104 stop at station 1 in the window.
		equal(t, "operations", len(ops), 4)
		b := ops[100]
		equal(t, "platform from schedule", *b.Platform, "3")
		equal(t, "origin", *b.Origin, "Warszawa Wschodnia")
		equal(t, "destination", *b.Destination, "Kraków Główny")
		equal(t, "train", *b.Run.Train.CarrierName, "PKP Intercity")
		equal(t, "delay", *b.Stop.DepartureDelayMinutes, 4)
		equal(t, "bare train has no platform", ops[102].Platform, (*string)(nil))
	})

	t.Run("active runs", func(t *testing.T) {
		snapshot, err := repo.LatestSnapshot(ctx)
		check(t, err)
		equal(t, "snapshot", snapshot.UTC(), time.Date(2026, 10, 4, 11, 58, 0, 0, time.UTC))

		runs, err := repo.ListActiveRuns(ctx, at, snapshot.Add(-position.SnapshotWindow), position.OperatingDates(at))
		check(t, err)
		// 101 departs 13:30 (too early), 102 was not in recent snapshots, 103/104 are X/C.
		equal(t, "runs", len(runs), 1)
		run := runs[0]
		equal(t, "operation", run.OperationID, int64(100))
		equal(t, "cancelled stop dropped", len(run.Stops), 3)
		equal(t, "station name", *run.Stops[0].StationName, "Warszawa Wschodnia")
		equal(t, "coordinates", *run.Stops[2].Latitude, 50.07)
	})

	t.Run("run detail", func(t *testing.T) {
		run, err := repo.GetRun(ctx, 100)
		check(t, err)
		equal(t, "stops", len(run.Stops), 4)
		equal(t, "cancelled", run.Stops[2].IsCancelled, true)
		equal(t, "operating date", run.OperatingDate.Format(time.DateOnly), "2026-10-04")
		_, err = repo.GetRun(ctx, 999)
		equal(t, "missing", errors.Is(err, service.ErrNotFound), true)
	})

	t.Run("connections by station id and by city", func(t *testing.T) {
		date := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
		byID, err := repo.SearchConnections(ctx, service.ConnectionQuery{
			Date: date, From: service.Place{StationID: 2}, To: service.Place{StationID: 4},
		})
		check(t, err)
		equal(t, "by id", len(byID), 1)
		equal(t, "stops", [2]int{byID[0].From.Seq, byID[0].To.Seq}, [2]int{1, 4})
		equal(t, "duration", *byID[0].To.Arrival-*byID[0].From.Departure, 3*time.Hour)

		byCity, err := repo.SearchConnections(ctx, service.ConnectionQuery{
			Date: date, From: service.Place{Name: "łódź"}, To: service.Place{Name: "Warszawa"},
		})
		check(t, err)
		equal(t, "by city", len(byCity), 1)
		equal(t, "overnight arrival", *byCity[0].To.Arrival, 24*time.Hour+30*time.Minute)

		reverse, err := repo.SearchConnections(ctx, service.ConnectionQuery{
			Date: date, From: service.Place{StationID: 4}, To: service.Place{StationID: 2},
		})
		check(t, err)
		equal(t, "wrong direction", len(reverse), 0)

		filtered, err := repo.SearchConnections(ctx, service.ConnectionQuery{
			Date: date.AddDate(0, 0, 1), From: service.Place{Name: "Warszawa"}, To: service.Place{Name: "Kraków"},
			Carriers: []string{"IC"}, Categories: []string{"IC"},
		})
		check(t, err)
		equal(t, "next day, filtered", len(filtered), 1)
		equal(t, "earliest departure", filtered[0].From.StationName, "Warszawa Wschodnia")
	})

	t.Run("train schedule", func(t *testing.T) {
		schedule, err := repo.GetTrainSchedule(ctx, 11)
		check(t, err)
		equal(t, "name", *schedule.Train.Name, "Łodzianka")
		equal(t, "dates", len(schedule.OperatingDates), 1)
		equal(t, "stops", len(schedule.Stops), 3)
		equal(t, "next day", *schedule.Stops[2].Arrival, 24*time.Hour+30*time.Minute)
		equal(t, "platform", *schedule.Stops[2].Platform, "6")
		_, err = repo.GetTrainSchedule(ctx, 999)
		equal(t, "missing", errors.Is(err, service.ErrNotFound), true)
	})

	t.Run("disruptions", func(t *testing.T) {
		disruptions, total, err := repo.ListDisruptions(ctx, 1, 0)
		check(t, err)
		equal(t, "total", total, 2)
		equal(t, "page", len(disruptions), 1)
		equal(t, "stations", [2]string{*disruptions[0].StartStation, *disruptions[0].EndStation},
			[2]string{"Łódź Fabryczna", "Koluszki"})
	})

	t.Run("statistics and freshness", func(t *testing.T) {
		stats, err := repo.GetDayStatistics(ctx, time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 10, 4, 11, 43, 0, 0, time.UTC))
		check(t, err)
		// 100 (P, confirmed delay 4), 101 (S), 102 (P but stale).
		equal(t, "stats", *stats, service.DayStatistics{
			Total: 3, InProgress: 1, Started: 2, OnTime: 2, AvgDelay: ptr(4.0),
		})

		f, err := repo.GetFreshness(ctx)
		check(t, err)
		equal(t, "operations", f.OperationsUpdatedAt.UTC(), time.Date(2026, 10, 4, 11, 58, 0, 0, time.UTC))
		equal(t, "disruptions", f.Disruptions, 2)
		equal(t, "schedules", f.SchedulesUpdatedAt != nil, true)
	})
}

func ptr[T any](v T) *T { return &v }
