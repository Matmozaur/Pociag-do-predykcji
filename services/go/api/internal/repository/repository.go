// Package repository reads the curated tables (db/migrations/001_init.up.sql) with pgx.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/pociag-do-predykcji/services/go/api/internal/position"
	"github.com/pociag-do-predykcji/services/go/api/internal/service"
)

type Repository struct {
	pool   *pgxpool.Pool
	tracer trace.Tracer
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, tracer: otel.Tracer("pociag.api")}
}

func (r *Repository) Ping(ctx context.Context) error {
	ctx, span := r.tracer.Start(ctx, "db.ping")
	defer span.End()
	return r.pool.Ping(ctx)
}

// ── Column lists shared by several queries ────────────────────────────────────

const runColumns = `o.id, o.operating_date, o.status, o.updated_at`

func runTargets(r *service.Run) []any {
	return []any{&r.OperationID, &r.OperatingDate, &r.Status, &r.UpdatedAt}
}

const trainColumns = `t.id, t.schedule_id, t.order_id, t.name, t.number, t.category, t.carrier_code, t.carrier_name`

func trainTargets(t *service.Train) []any {
	return []any{&t.ID, &t.ScheduleID, &t.OrderID, &t.Name, &t.Number, &t.Category, &t.CarrierCode, &t.CarrierName}
}

const stopColumns = `s.station_id, s.seq, s.planned_arrival, s.planned_departure, s.actual_arrival,
	s.actual_departure, s.arrival_delay, s.departure_delay, s.is_confirmed, s.is_cancelled`

func stopTargets(s *position.Stop) []any {
	return []any{&s.StationID, &s.SequenceNumber, &s.PlannedArrival, &s.PlannedDeparture, &s.ActualArrival,
		&s.ActualDeparture, &s.ArrivalDelayMinutes, &s.DepartureDelayMinutes, &s.IsConfirmed, &s.IsCancelled}
}

// stationTargets scans "st.name, st.latitude, st.longitude" into a stop.
func stationTargets(s *position.Stop) []any {
	return []any{&s.StationName, &s.Latitude, &s.Longitude}
}

func targets(groups ...[]any) []any {
	var out []any
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return service.ErrNotFound
	}
	return err
}

// seconds converts EXTRACT(EPOCH FROM interval) to a duration.
func seconds(v *int) *time.Duration {
	if v == nil {
		return nil
	}
	d := time.Duration(*v) * time.Second
	return &d
}

// ── Stations ──────────────────────────────────────────────────────────────────

func (r *Repository) SearchStations(ctx context.Context, query string, limit int) ([]service.Station, error) {
	ctx, span := r.tracer.Start(ctx, "db.stations.search")
	defer span.End()

	// Prefix matches first, then other substring matches; shorter names first within each.
	const sql = `
		SELECT id, name, city, latitude, longitude
		FROM stations
		WHERE name ILIKE '%' || $1 || '%' OR city ILIKE $1 || '%'
		ORDER BY name NOT ILIKE $1 || '%', length(name), name
		LIMIT $2`
	return r.stations(ctx, sql, query, limit)
}

func (r *Repository) ListMappedStations(ctx context.Context) ([]service.Station, error) {
	ctx, span := r.tracer.Start(ctx, "db.stations.mapped")
	defer span.End()

	const sql = `
		SELECT id, name, city, latitude, longitude
		FROM stations
		WHERE latitude IS NOT NULL AND longitude IS NOT NULL
		ORDER BY id`
	return r.stations(ctx, sql)
}

func (r *Repository) GetStation(ctx context.Context, id int) (*service.Station, error) {
	ctx, span := r.tracer.Start(ctx, "db.stations.get")
	defer span.End()

	const sql = `SELECT id, name, city, latitude, longitude FROM stations WHERE id = $1`
	var st service.Station
	if err := r.pool.QueryRow(ctx, sql, id).Scan(&st.ID, &st.Name, &st.City, &st.Latitude, &st.Longitude); err != nil {
		return nil, fmt.Errorf("get station %d: %w", id, notFound(err))
	}
	return &st, nil
}

func (r *Repository) stations(ctx context.Context, sql string, args ...any) ([]service.Station, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query stations: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (service.Station, error) {
		var st service.Station
		err := row.Scan(&st.ID, &st.Name, &st.City, &st.Latitude, &st.Longitude)
		return st, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan stations: %w", err)
	}
	return out, nil
}

// ListBoardStops returns the stops at the station planned (departure, else arrival) within
// [from, to], of runs that are not fully cancelled. The time predicate matches the expression of
// operation_stops_station_time_idx.
func (r *Repository) ListBoardStops(ctx context.Context, stationID int, from, to time.Time) ([]service.BoardStop, error) {
	ctx, span := r.tracer.Start(ctx, "db.station_board.query")
	defer span.End()

	sql := `
		SELECT ` + runColumns + `, ` + trainColumns + `, ` + stopColumns + `,
		       ss.platform, ss.track, origin.name, destination.name
		FROM operation_stops s
		JOIN operations o ON o.id = s.operation_id
		JOIN trains t ON t.id = o.train_id
		LEFT JOIN LATERAL (
		    SELECT platform, track FROM schedule_stops
		    WHERE train_id = t.id AND station_id = s.station_id ORDER BY seq LIMIT 1
		) ss ON TRUE
		LEFT JOIN LATERAL (
		    SELECT st.name FROM operation_stops f JOIN stations st ON st.id = f.station_id
		    WHERE f.operation_id = o.id ORDER BY f.seq LIMIT 1
		) origin ON TRUE
		LEFT JOIN LATERAL (
		    SELECT st.name FROM operation_stops l JOIN stations st ON st.id = l.station_id
		    WHERE l.operation_id = o.id ORDER BY l.seq DESC LIMIT 1
		) destination ON TRUE
		WHERE s.station_id = $1
		  AND COALESCE(s.planned_departure, s.planned_arrival) BETWEEN $2 AND $3
		  AND o.status <> 'X'`

	rows, err := r.pool.Query(ctx, sql, stationID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query station board: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (service.BoardStop, error) {
		var b service.BoardStop
		err := row.Scan(targets(
			runTargets(&b.Run),
			trainTargets(&b.Run.Train),
			stopTargets(&b.Stop),
			[]any{&b.Platform, &b.Track, &b.Origin, &b.Destination},
		)...)
		return b, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan station board: %w", err)
	}
	return out, nil
}

// ── Operations ────────────────────────────────────────────────────────────────

func (r *Repository) LatestSnapshot(ctx context.Context) (time.Time, error) {
	ctx, span := r.tracer.Start(ctx, "db.operations.latest_snapshot")
	defer span.End()

	var latest *time.Time
	if err := r.pool.QueryRow(ctx, `SELECT MAX(updated_at) FROM operations`).Scan(&latest); err != nil {
		return time.Time{}, fmt.Errorf("latest snapshot: %w", err)
	}
	if latest == nil {
		return time.Time{}, nil
	}
	return *latest, nil
}

// ListActiveRuns pre-selects runs for the active predicate (position.IsActive): status S or P on
// one of dates, seen since seenSince, with effective first departure (minus
// position.PreDepartureWindow) <= at <= effective last arrival. Effective times follow
// position.EffectiveTimes. Cancelled stops are left out.
func (r *Repository) ListActiveRuns(ctx context.Context, at, seenSince time.Time, dates []time.Time) ([]service.Run, error) {
	ctx, span := r.tracer.Start(ctx, "db.operations.active")
	defer span.End()

	sql := `
		WITH runs AS (
		    SELECT o.id FROM operations o
		    JOIN operation_stops s ON s.operation_id = o.id AND NOT s.is_cancelled
		    WHERE o.operating_date = ANY($1::date[])
		      AND o.status IN ('S', 'P')
		      AND o.updated_at >= $2
		    GROUP BY o.id
		    HAVING MIN(COALESCE(
		               COALESCE(s.actual_departure, s.planned_departure + make_interval(mins => COALESCE(s.departure_delay, 0))),
		               COALESCE(s.actual_arrival, s.planned_arrival + make_interval(mins => COALESCE(s.arrival_delay, 0)))
		           )) - $3::interval <= $4
		       AND MAX(COALESCE(
		               COALESCE(s.actual_arrival, s.planned_arrival + make_interval(mins => COALESCE(s.arrival_delay, 0))),
		               COALESCE(s.actual_departure, s.planned_departure + make_interval(mins => COALESCE(s.departure_delay, 0)))
		           )) >= $4
		)
		SELECT ` + runColumns + `, ` + trainColumns + `, ` + stopColumns + `,
		       st.name, st.latitude, st.longitude
		FROM runs
		JOIN operations o ON o.id = runs.id
		JOIN trains t ON t.id = o.train_id
		JOIN operation_stops s ON s.operation_id = o.id AND NOT s.is_cancelled
		JOIN stations st ON st.id = s.station_id
		ORDER BY o.id, s.seq`

	rows, err := r.pool.Query(ctx, sql, dates, seenSince, position.PreDepartureWindow, at)
	if err != nil {
		return nil, fmt.Errorf("query active runs: %w", err)
	}
	return collectRuns(rows)
}

func (r *Repository) GetRun(ctx context.Context, operationID int64) (*service.Run, error) {
	ctx, span := r.tracer.Start(ctx, "db.operations.get")
	defer span.End()

	var run service.Run
	runSQL := `SELECT ` + runColumns + `, ` + trainColumns + `
		FROM operations o JOIN trains t ON t.id = o.train_id WHERE o.id = $1`
	err := r.pool.QueryRow(ctx, runSQL, operationID).Scan(targets(runTargets(&run), trainTargets(&run.Train))...)
	if err != nil {
		return nil, fmt.Errorf("get run %d: %w", operationID, notFound(err))
	}

	stopsSQL := `SELECT ` + stopColumns + `, st.name, st.latitude, st.longitude
		FROM operation_stops s JOIN stations st ON st.id = s.station_id
		WHERE s.operation_id = $1 ORDER BY s.seq`
	rows, err := r.pool.Query(ctx, stopsSQL, operationID)
	if err != nil {
		return nil, fmt.Errorf("query run %d stops: %w", operationID, err)
	}
	run.Stops, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (position.Stop, error) {
		var stop position.Stop
		err := row.Scan(targets(stopTargets(&stop), stationTargets(&stop))...)
		return stop, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan run %d stops: %w", operationID, err)
	}
	return &run, nil
}

// collectRuns groups rows of (run, train, stop, station), ordered by run, into runs.
func collectRuns(rows pgx.Rows) ([]service.Run, error) {
	defer rows.Close()
	runs := []service.Run{}
	for rows.Next() {
		var run service.Run
		var stop position.Stop
		dest := targets(runTargets(&run), trainTargets(&run.Train), stopTargets(&stop), stationTargets(&stop))
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		if n := len(runs); n == 0 || runs[n-1].OperationID != run.OperationID {
			runs = append(runs, run)
		}
		last := &runs[len(runs)-1]
		last.Stops = append(last.Stops, stop)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runs: %w", err)
	}
	return runs, nil
}

// ── Schedules ─────────────────────────────────────────────────────────────────

// SearchConnections finds trains running on q.Date that depart from a q.From station and later
// arrive at a q.To station. A Place name matches a station's city or the start of its name. For
// several matching stations, the earliest departure and then the earliest arrival win.
func (r *Repository) SearchConnections(ctx context.Context, q service.ConnectionQuery) ([]service.Connection, error) {
	ctx, span := r.tracer.Start(ctx, "db.schedules.search")
	defer span.End()

	sql := `
		SELECT DISTINCT ON (t.id) ` + trainColumns + `,
		       a.seq, a.station_id, sa.name, EXTRACT(EPOCH FROM a.departure)::int,
		       b.seq, b.station_id, sb.name, EXTRACT(EPOCH FROM b.arrival)::int
		FROM trains t
		JOIN schedule_stops a ON a.train_id = t.id AND a.departure IS NOT NULL
		JOIN stations sa ON sa.id = a.station_id
		JOIN schedule_stops b ON b.train_id = t.id AND b.seq > a.seq AND b.arrival IS NOT NULL
		JOIN stations sb ON sb.id = b.station_id
		WHERE t.operating_dates @> ARRAY[$1::date]
		  AND (sa.id = $2 OR ($2 = 0 AND (sa.city ILIKE $3 OR sa.name ILIKE $3 || '%')))
		  AND (sb.id = $4 OR ($4 = 0 AND (sb.city ILIKE $5 OR sb.name ILIKE $5 || '%')))
		  AND (cardinality($6::text[]) = 0 OR t.carrier_code = ANY($6))
		  AND (cardinality($7::text[]) = 0 OR t.category = ANY($7))
		ORDER BY t.id, a.departure, b.arrival`

	rows, err := r.pool.Query(ctx, sql, q.Date, q.From.StationID, q.From.Name, q.To.StationID, q.To.Name,
		nonNil(q.Carriers), nonNil(q.Categories))
	if err != nil {
		return nil, fmt.Errorf("query connections: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (service.Connection, error) {
		var c service.Connection
		var departure, arrival *int
		err := row.Scan(targets(
			trainTargets(&c.Train),
			[]any{&c.From.Seq, &c.From.StationID, &c.From.StationName, &departure},
			[]any{&c.To.Seq, &c.To.StationID, &c.To.StationName, &arrival},
		)...)
		c.From.Departure, c.To.Arrival = seconds(departure), seconds(arrival)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan connections: %w", err)
	}
	return out, nil
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (r *Repository) GetTrainSchedule(ctx context.Context, trainID int64) (*service.TrainSchedule, error) {
	ctx, span := r.tracer.Start(ctx, "db.schedules.get")
	defer span.End()

	var schedule service.TrainSchedule
	trainSQL := `SELECT ` + trainColumns + `, t.operating_dates FROM trains t WHERE t.id = $1`
	err := r.pool.QueryRow(ctx, trainSQL, trainID).Scan(append(trainTargets(&schedule.Train), &schedule.OperatingDates)...)
	if err != nil {
		return nil, fmt.Errorf("get train %d: %w", trainID, notFound(err))
	}

	const stopsSQL = `
		SELECT s.seq, s.station_id, st.name, EXTRACT(EPOCH FROM s.arrival)::int,
		       EXTRACT(EPOCH FROM s.departure)::int, s.platform
		FROM schedule_stops s JOIN stations st ON st.id = s.station_id
		WHERE s.train_id = $1
		ORDER BY s.seq`
	rows, err := r.pool.Query(ctx, stopsSQL, trainID)
	if err != nil {
		return nil, fmt.Errorf("query schedule stops: %w", err)
	}
	schedule.Stops, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (service.ScheduleStop, error) {
		var st service.ScheduleStop
		var arrival, departure *int
		err := row.Scan(&st.Seq, &st.StationID, &st.StationName, &arrival, &departure, &st.Platform)
		st.Arrival, st.Departure = seconds(arrival), seconds(departure)
		return st, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan schedule stops: %w", err)
	}
	return &schedule, nil
}

// ── Disruptions & statistics ──────────────────────────────────────────────────

func (r *Repository) ListDisruptions(ctx context.Context, limit, offset int) ([]service.Disruption, int, error) {
	ctx, span := r.tracer.Start(ctx, "db.disruptions.list")
	defer span.End()

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM disruptions`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count disruptions: %w", err)
	}
	const sql = `
		SELECT d.id, ss.name, es.name, d.message, d.date_from, d.date_to, d.affected_trains
		FROM disruptions d
		LEFT JOIN stations ss ON ss.id = d.start_station_id
		LEFT JOIN stations es ON es.id = d.end_station_id
		ORDER BY d.affected_trains DESC, d.id
		LIMIT $1 OFFSET $2`
	rows, err := r.pool.Query(ctx, sql, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query disruptions: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (service.Disruption, error) {
		var d service.Disruption
		err := row.Scan(&d.ID, &d.StartStation, &d.EndStation, &d.Message, &d.DateFrom, &d.DateTo, &d.AffectedTrains)
		return d, err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("scan disruptions: %w", err)
	}
	return out, total, nil
}

func (r *Repository) GetDayStatistics(ctx context.Context, date, seenSince time.Time) (*service.DayStatistics, error) {
	ctx, span := r.tracer.Start(ctx, "db.operations.statistics")
	defer span.End()

	const sql = `
		WITH runs AS (
		    SELECT o.status, o.updated_at,
		           (SELECT GREATEST(COALESCE(MAX(s.arrival_delay), 0), COALESCE(MAX(s.departure_delay), 0))
		            FROM operation_stops s WHERE s.operation_id = o.id AND s.is_confirmed) AS max_delay
		    FROM operations o
		    WHERE o.operating_date = $1
		)
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'P' AND updated_at >= $2),
		       count(*) FILTER (WHERE status = 'C'),
		       count(*) FILTER (WHERE status IN ('X', 'Q')),
		       count(*) FILTER (WHERE status IN ('P', 'C')),
		       count(*) FILTER (WHERE status IN ('P', 'C') AND max_delay <= 5),
		       AVG(max_delay) FILTER (WHERE status IN ('P', 'C') AND max_delay > 0)::float8
		FROM runs`

	var s service.DayStatistics
	err := r.pool.QueryRow(ctx, sql, date, seenSince).Scan(
		&s.Total, &s.InProgress, &s.Completed, &s.Cancelled, &s.Started, &s.OnTime, &s.AvgDelay)
	if err != nil {
		return nil, fmt.Errorf("day statistics: %w", err)
	}
	return &s, nil
}

func (r *Repository) GetFreshness(ctx context.Context) (*service.Freshness, error) {
	ctx, span := r.tracer.Start(ctx, "db.freshness")
	defer span.End()

	const sql = `
		SELECT (SELECT MAX(updated_at) FROM trains WHERE cardinality(operating_dates) > 0),
		       (SELECT MAX(updated_at) FROM operations),
		       (SELECT count(*) FROM disruptions)`
	var f service.Freshness
	if err := r.pool.QueryRow(ctx, sql).Scan(&f.SchedulesUpdatedAt, &f.OperationsUpdatedAt, &f.Disruptions); err != nil {
		return nil, fmt.Errorf("freshness: %w", err)
	}
	return &f, nil
}
