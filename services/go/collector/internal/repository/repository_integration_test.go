//go:build integration

package repository_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/pociag-do-predykcji/services/go/collector/internal/repository"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := postgres.Run(
		ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("collector_test"),
		postgres.WithUsername("collector"),
		postgres.WithPassword("collector"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		log.Fatalf("start postgres container: %v", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("postgres connection string: %v", err)
	}

	testPool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("create pgx pool: %v", err)
	}

	if err := setupSchema(ctx, testPool); err != nil {
		log.Fatalf("setup schema: %v", err)
	}

	exitCode := m.Run()

	testPool.Close()
	if err := container.Terminate(ctx); err != nil {
		log.Printf("terminate postgres container: %v", err)
	}

	os.Exit(exitCode)
}

func TestRepository_CreateAndListIngestionRuns_Success(t *testing.T) {
	t.Parallel()

	repo := repository.New(testPool)
	ctx := context.Background()
	runDate := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)

	runID, err := repo.CreateIngestionRun(ctx, "schedules", runDate)
	require.NoError(t, err)
	require.NotZero(t, runID)

	err = repo.MarkIngestionRunSuccess(ctx, runID, 12)
	require.NoError(t, err)

	runs, err := repo.ListIngestionRuns(ctx, nil, 10)
	require.NoError(t, err)
	require.NotEmpty(t, runs)
	require.Equal(t, "schedules", runs[0].Pipeline)
}

func setupSchema(ctx context.Context, pool *pgxpool.Pool) error {
	statements := []string{
		`CREATE TABLE ingestion_runs (
			id BIGSERIAL PRIMARY KEY,
			pipeline TEXT NOT NULL,
			run_date DATE NOT NULL,
			status TEXT NOT NULL DEFAULT 'running',
			records_fetched INT DEFAULT 0,
			records_upserted INT DEFAULT 0,
			started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			completed_at TIMESTAMPTZ,
			error_message TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE raw_dictionaries (
			id BIGSERIAL PRIMARY KEY,
			fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			dictionary_type TEXT NOT NULL,
			payload JSONB NOT NULL,
			record_count INT,
			ingestion_run_id BIGINT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE raw_schedules (
			id BIGSERIAL PRIMARY KEY,
			fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			date_from DATE NOT NULL,
			date_to DATE NOT NULL,
			page INT NOT NULL DEFAULT 1,
			payload JSONB NOT NULL,
			record_count INT,
			ingestion_run_id BIGINT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE raw_operations (
			id BIGSERIAL PRIMARY KEY,
			fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			operating_date DATE NOT NULL,
			page INT NOT NULL DEFAULT 1,
			payload JSONB NOT NULL,
			record_count INT,
			ingestion_run_id BIGINT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE raw_disruptions (
			id BIGSERIAL PRIMARY KEY,
			fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			date_from DATE NOT NULL,
			date_to DATE NOT NULL,
			payload JSONB NOT NULL,
			record_count INT,
			ingestion_run_id BIGINT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	}

	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			return fmt.Errorf("exec statement: %w", err)
		}
	}

	return nil
}
