package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var ErrPipelineRunning = errors.New("pipeline already running")

type FetchSchedulesRequest struct {
	DateFrom time.Time
	DateTo   time.Time
	Force    bool
}

type FetchOperationsRequest struct {
	Date  time.Time
	Force bool
}

type FetchDisruptionsRequest struct {
	DateFrom time.Time
	DateTo   time.Time
	Force    bool
}

type FetchDictionariesResult struct {
	FetchedTypes map[string]int `json:"fetched_types"`
	DurationMS   int64          `json:"duration_ms,omitempty"`
}

type FetchResult struct {
	Pipeline       string `json:"pipeline"`
	Status         string `json:"status"`
	RecordsFetched int    `json:"records_fetched"`
	PagesLanded    int    `json:"pages_landed"`
	DurationMS     int64  `json:"duration_ms,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
}

type IngestionRun struct {
	ID              int64      `json:"id"`
	Pipeline        string     `json:"pipeline"`
	RunDate         string     `json:"run_date"`
	Status          string     `json:"status"`
	RecordsFetched  *int       `json:"records_fetched,omitempty"`
	RecordsUpserted *int       `json:"records_upserted,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	ErrorMessage    *string    `json:"error_message,omitempty"`
}

type Repository interface {
	Ping(ctx context.Context) error
	IsPipelineRunning(ctx context.Context, pipeline string, runDate time.Time) (bool, error)
	CreateIngestionRun(ctx context.Context, pipeline string, runDate time.Time) (int64, error)
	MarkIngestionRunSuccess(ctx context.Context, runID int64, recordsFetched int) error
	MarkIngestionRunFailed(ctx context.Context, runID int64, errorMessage string) error
	InsertRawDictionaries(ctx context.Context, dictionaryType string, payload []byte, recordCount int, ingestionRunID int64) error
	InsertRawSchedules(ctx context.Context, dateFrom time.Time, dateTo time.Time, page int, payload []byte, recordCount int, ingestionRunID int64) error
	InsertRawOperations(ctx context.Context, operatingDate time.Time, page int, payload []byte, recordCount int, ingestionRunID int64) error
	InsertRawDisruptions(ctx context.Context, dateFrom time.Time, dateTo time.Time, payload []byte, recordCount int, ingestionRunID int64) error
	ListIngestionRuns(ctx context.Context, pipeline *string, limit int) ([]IngestionRun, error)
}

type PLKClient interface {
	FetchDictionaries(ctx context.Context) (map[string][]byte, error)
	FetchSchedules(ctx context.Context, dateFrom time.Time, dateTo time.Time, page int, pageSize int) ([]byte, error)
	FetchOperations(ctx context.Context, operatingDate time.Time, page int, pageSize int) ([]byte, error)
	FetchDisruptions(ctx context.Context, dateFrom time.Time, dateTo time.Time) ([]byte, error)
}

type Service struct {
	repo      Repository
	plkClient PLKClient
	tracer    trace.Tracer
}

func New(repo Repository, plkClient PLKClient) *Service {
	return &Service{
		repo:      repo,
		plkClient: plkClient,
		tracer:    otel.Tracer("pociag.collector"),
	}
}

func (s *Service) Ready(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "readiness.check")
	defer span.End()

	if err := s.repo.Ping(ctx); err != nil {
		return fmt.Errorf("ready: %w", err)
	}

	return nil
}

func (s *Service) FetchDictionaries(ctx context.Context) (FetchDictionariesResult, error) {
	ctx, span := s.tracer.Start(ctx, "dictionaries.fetch")
	defer span.End()

	start := time.Now()
	pipeline := "dictionaries"
	runDate := start.UTC()

	runID, err := s.repo.CreateIngestionRun(ctx, pipeline, runDate)
	if err != nil {
		return FetchDictionariesResult{}, fmt.Errorf("fetch dictionaries: create ingestion run: %w", err)
	}

	dictionaryPayloads, err := s.plkClient.FetchDictionaries(ctx)
	if err != nil {
		return FetchDictionariesResult{}, s.failRun(ctx, runID, err, "fetch dictionaries")
	}

	totalRecords := 0
	fetchedTypes := make(map[string]int, len(dictionaryPayloads))
	for dictionaryType, payload := range dictionaryPayloads {
		recordCount := countRecords(payload)
		if err := s.repo.InsertRawDictionaries(ctx, dictionaryType, payload, recordCount, runID); err != nil {
			return FetchDictionariesResult{}, s.failRun(ctx, runID, err, "fetch dictionaries: insert raw payload")
		}

		totalRecords += recordCount
		fetchedTypes[dictionaryType] = recordCount
	}

	if err := s.repo.MarkIngestionRunSuccess(ctx, runID, totalRecords); err != nil {
		return FetchDictionariesResult{}, fmt.Errorf("fetch dictionaries: mark success: %w", err)
	}

	return FetchDictionariesResult{
		FetchedTypes: fetchedTypes,
		DurationMS:   time.Since(start).Milliseconds(),
	}, nil
}

func (s *Service) FetchSchedules(ctx context.Context, req FetchSchedulesRequest) (FetchResult, error) {
	ctx, span := s.tracer.Start(ctx, "schedules.fetch")
	defer span.End()

	return s.fetchWithRange(ctx, "schedules", req.DateFrom, req.DateTo, req.Force)
}

func (s *Service) FetchOperations(ctx context.Context, req FetchOperationsRequest) (FetchResult, error) {
	ctx, span := s.tracer.Start(ctx, "operations.fetch")
	defer span.End()

	start := time.Now()

	if !req.Force {
		running, err := s.repo.IsPipelineRunning(ctx, "operations", req.Date)
		if err != nil {
			return FetchResult{}, fmt.Errorf("fetch operations: check running: %w", err)
		}
		if running {
			return FetchResult{}, ErrPipelineRunning
		}
	}

	runID, err := s.repo.CreateIngestionRun(ctx, "operations", req.Date)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetch operations: create ingestion run: %w", err)
	}

	payload, err := s.plkClient.FetchOperations(ctx, req.Date, 1, 1000)
	if err != nil {
		return FetchResult{}, s.failRun(ctx, runID, err, "fetch operations")
	}

	recordCount := countRecords(payload)
	if err := s.repo.InsertRawOperations(ctx, req.Date, 1, payload, recordCount, runID); err != nil {
		return FetchResult{}, s.failRun(ctx, runID, err, "fetch operations: insert raw payload")
	}

	if err := s.repo.MarkIngestionRunSuccess(ctx, runID, recordCount); err != nil {
		return FetchResult{}, fmt.Errorf("fetch operations: mark success: %w", err)
	}

	return FetchResult{
		Pipeline:       "operations",
		Status:         "success",
		RecordsFetched: recordCount,
		PagesLanded:    1,
		DurationMS:     time.Since(start).Milliseconds(),
	}, nil
}

func (s *Service) FetchDisruptions(ctx context.Context, req FetchDisruptionsRequest) (FetchResult, error) {
	ctx, span := s.tracer.Start(ctx, "disruptions.fetch")
	defer span.End()

	return s.fetchWithRange(ctx, "disruptions", req.DateFrom, req.DateTo, req.Force)
}

func (s *Service) GetFetchStatus(ctx context.Context, pipeline *string, limit int) ([]IngestionRun, error) {
	ctx, span := s.tracer.Start(ctx, "status.fetch")
	defer span.End()

	runs, err := s.repo.ListIngestionRuns(ctx, pipeline, limit)
	if err != nil {
		return nil, fmt.Errorf("get fetch status: %w", err)
	}

	return runs, nil
}

func (s *Service) fetchWithRange(ctx context.Context, pipeline string, dateFrom time.Time, dateTo time.Time, force bool) (FetchResult, error) {
	start := time.Now()

	if !force {
		running, err := s.repo.IsPipelineRunning(ctx, pipeline, dateFrom)
		if err != nil {
			return FetchResult{}, fmt.Errorf("fetch %s: check running: %w", pipeline, err)
		}
		if running {
			return FetchResult{}, ErrPipelineRunning
		}
	}

	runID, err := s.repo.CreateIngestionRun(ctx, pipeline, dateFrom)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetch %s: create ingestion run: %w", pipeline, err)
	}

	var (
		payload     []byte
		recordCount int
		insertErr   error
	)

	switch pipeline {
	case "schedules":
		payload, insertErr = s.plkClient.FetchSchedules(ctx, dateFrom, dateTo, 1, 1000)
		if insertErr == nil {
			recordCount = countRecords(payload)
			insertErr = s.repo.InsertRawSchedules(ctx, dateFrom, dateTo, 1, payload, recordCount, runID)
		}
	case "disruptions":
		payload, insertErr = s.plkClient.FetchDisruptions(ctx, dateFrom, dateTo)
		if insertErr == nil {
			recordCount = countRecords(payload)
			insertErr = s.repo.InsertRawDisruptions(ctx, dateFrom, dateTo, payload, recordCount, runID)
		}
	default:
		insertErr = fmt.Errorf("unsupported pipeline: %s", pipeline)
	}

	if insertErr != nil {
		return FetchResult{}, s.failRun(ctx, runID, insertErr, fmt.Sprintf("fetch %s", pipeline))
	}

	if err := s.repo.MarkIngestionRunSuccess(ctx, runID, recordCount); err != nil {
		return FetchResult{}, fmt.Errorf("fetch %s: mark success: %w", pipeline, err)
	}

	return FetchResult{
		Pipeline:       pipeline,
		Status:         "success",
		RecordsFetched: recordCount,
		PagesLanded:    1,
		DurationMS:     time.Since(start).Milliseconds(),
	}, nil
}

func (s *Service) failRun(ctx context.Context, runID int64, rootErr error, operation string) error {
	if markErr := s.repo.MarkIngestionRunFailed(ctx, runID, rootErr.Error()); markErr != nil {
		return fmt.Errorf("%s: %w; mark ingestion run failed: %v", operation, rootErr, markErr)
	}

	return fmt.Errorf("%s: %w", operation, rootErr)
}

func countRecords(payload []byte) int {
	if len(payload) == 0 {
		return 0
	}

	var asObject map[string]any
	if err := json.Unmarshal(payload, &asObject); err != nil {
		return 0
	}

	for _, key := range []string{"data", "items", "results", "schedules", "operations", "disruptions", "carriers", "stations", "commercialCategories", "stopTypes", "cities"} {
		if value, ok := asObject[key]; ok {
			if asArray, ok := value.([]any); ok {
				return len(asArray)
			}
		}
	}

	for _, value := range asObject {
		if asArray, ok := value.([]any); ok {
			return len(asArray)
		}
	}

	return 0
}
