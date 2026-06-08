package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/pociag-do-predykcji/services/go/collector/internal/service"
)

type Repository struct {
	pool   *pgxpool.Pool
	tracer trace.Tracer
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:   pool,
		tracer: otel.Tracer("pociag.collector"),
	}
}

func (r *Repository) Ping(ctx context.Context) error {
	ctx, span := r.tracer.Start(ctx, "db.connection.ping")
	defer span.End()

	if err := r.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	return nil
}

func (r *Repository) IsPipelineRunning(ctx context.Context, pipeline string, runDate time.Time) (bool, error) {
	ctx, span := r.tracer.Start(ctx, "db.ingestion_runs.exists")
	defer span.End()

	const query = `
SELECT EXISTS(
    SELECT 1
    FROM ingestion_runs
    WHERE pipeline = $1
      AND run_date = $2
      AND status = 'running'
)`

	var exists bool
	if err := r.pool.QueryRow(ctx, query, pipeline, runDate.Format("2006-01-02")).Scan(&exists); err != nil {
		return false, fmt.Errorf("check pipeline running: %w", err)
	}

	return exists, nil
}

func (r *Repository) CreateIngestionRun(ctx context.Context, pipeline string, runDate time.Time) (int64, error) {
	ctx, span := r.tracer.Start(ctx, "db.ingestion_runs.insert")
	defer span.End()

	const query = `
INSERT INTO ingestion_runs (pipeline, run_date, status)
VALUES ($1, $2, 'running')
RETURNING id`

	var runID int64
	if err := r.pool.QueryRow(ctx, query, pipeline, runDate.Format("2006-01-02")).Scan(&runID); err != nil {
		return 0, fmt.Errorf("create ingestion run: %w", err)
	}

	return runID, nil
}

func (r *Repository) MarkIngestionRunSuccess(ctx context.Context, runID int64, recordsFetched int) error {
	ctx, span := r.tracer.Start(ctx, "db.ingestion_runs.mark_success")
	defer span.End()

	const query = `
UPDATE ingestion_runs
SET status = 'success',
    records_fetched = $2,
    completed_at = NOW(),
    updated_at = NOW()
WHERE id = $1`

	if _, err := r.pool.Exec(ctx, query, runID, recordsFetched); err != nil {
		return fmt.Errorf("mark ingestion run success: %w", err)
	}

	return nil
}

func (r *Repository) MarkIngestionRunFailed(ctx context.Context, runID int64, errorMessage string) error {
	ctx, span := r.tracer.Start(ctx, "db.ingestion_runs.mark_failed")
	defer span.End()

	const query = `
UPDATE ingestion_runs
SET status = 'failed',
    error_message = $2,
    completed_at = NOW(),
    updated_at = NOW()
WHERE id = $1`

	if _, err := r.pool.Exec(ctx, query, runID, errorMessage); err != nil {
		return fmt.Errorf("mark ingestion run failed: %w", err)
	}

	return nil
}

func (r *Repository) InsertRawDictionaries(ctx context.Context, dictionaryType string, payload []byte, recordCount int, ingestionRunID int64) error {
	ctx, span := r.tracer.Start(ctx, "db.raw_dictionaries.insert")
	defer span.End()

	const query = `
INSERT INTO raw_dictionaries (dictionary_type, payload, record_count, ingestion_run_id)
VALUES ($1, $2::jsonb, $3, $4)`

	if _, err := r.pool.Exec(ctx, query, dictionaryType, payload, recordCount, ingestionRunID); err != nil {
		return fmt.Errorf("insert raw dictionaries: %w", err)
	}

	return nil
}

func (r *Repository) InsertRawSchedules(ctx context.Context, dateFrom time.Time, dateTo time.Time, page int, payload []byte, recordCount int, ingestionRunID int64) error {
	ctx, span := r.tracer.Start(ctx, "db.raw_schedules.insert")
	defer span.End()

	const query = `
INSERT INTO raw_schedules (date_from, date_to, page, payload, record_count, ingestion_run_id)
VALUES ($1, $2, $3, $4::jsonb, $5, $6)`

	if _, err := r.pool.Exec(ctx, query, dateFrom.Format("2006-01-02"), dateTo.Format("2006-01-02"), page, payload, recordCount, ingestionRunID); err != nil {
		return fmt.Errorf("insert raw schedules: %w", err)
	}

	return nil
}

func (r *Repository) InsertRawOperations(ctx context.Context, operatingDate time.Time, page int, payload []byte, recordCount int, ingestionRunID int64) error {
	ctx, span := r.tracer.Start(ctx, "db.raw_operations.insert")
	defer span.End()

	const query = `
INSERT INTO raw_operations (operating_date, page, payload, record_count, ingestion_run_id)
VALUES ($1, $2, $3::jsonb, $4, $5)`

	if _, err := r.pool.Exec(ctx, query, operatingDate.Format("2006-01-02"), page, payload, recordCount, ingestionRunID); err != nil {
		return fmt.Errorf("insert raw operations: %w", err)
	}

	return nil
}

func (r *Repository) InsertRawDisruptions(ctx context.Context, dateFrom time.Time, dateTo time.Time, payload []byte, recordCount int, ingestionRunID int64) error {
	ctx, span := r.tracer.Start(ctx, "db.raw_disruptions.insert")
	defer span.End()

	const query = `
INSERT INTO raw_disruptions (date_from, date_to, payload, record_count, ingestion_run_id)
VALUES ($1, $2, $3::jsonb, $4, $5)`

	if _, err := r.pool.Exec(ctx, query, dateFrom.Format("2006-01-02"), dateTo.Format("2006-01-02"), payload, recordCount, ingestionRunID); err != nil {
		return fmt.Errorf("insert raw disruptions: %w", err)
	}

	return nil
}

func (r *Repository) ListIngestionRuns(ctx context.Context, pipeline *string, limit int) ([]service.IngestionRun, error) {
	ctx, span := r.tracer.Start(ctx, "db.ingestion_runs.list")
	defer span.End()

	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	baseQuery := `
SELECT id, pipeline, run_date, status, records_fetched, records_upserted, started_at, completed_at, error_message
FROM ingestion_runs`

	args := []any{limit}
	query := baseQuery
	if pipeline != nil {
		query += ` WHERE pipeline = $2`
		args = []any{limit, *pipeline}
	}
	query += ` ORDER BY started_at DESC LIMIT $1`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list ingestion runs: %w", err)
	}
	defer rows.Close()

	runs := make([]service.IngestionRun, 0)
	for rows.Next() {
		var run service.IngestionRun
		var runDate time.Time
		var recordsFetched pgtype.Int4
		var recordsUpserted pgtype.Int4
		var completedAt pgtype.Timestamptz
		var errorMessage pgtype.Text

		if err := rows.Scan(
			&run.ID,
			&run.Pipeline,
			&runDate,
			&run.Status,
			&recordsFetched,
			&recordsUpserted,
			&run.StartedAt,
			&completedAt,
			&errorMessage,
		); err != nil {
			return nil, fmt.Errorf("scan ingestion run: %w", err)
		}

		run.RunDate = runDate.Format("2006-01-02")
		if recordsFetched.Valid {
			value := int(recordsFetched.Int32)
			run.RecordsFetched = &value
		}
		if recordsUpserted.Valid {
			value := int(recordsUpserted.Int32)
			run.RecordsUpserted = &value
		}
		if completedAt.Valid {
			value := completedAt.Time
			run.CompletedAt = &value
		}
		if errorMessage.Valid {
			value := errorMessage.String
			run.ErrorMessage = &value
		}

		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ingestion runs: %w", err)
	}

	return runs, nil
}
