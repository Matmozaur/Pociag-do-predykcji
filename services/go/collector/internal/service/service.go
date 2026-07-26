package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var ErrPipelineRunning = errors.New("pipeline already running")

const markIngestionRunFailedTimeout = 5 * time.Second

type FetchSchedulesRequest struct {
	DateFrom time.Time
	DateTo   time.Time
	Force    bool
}

type FetchOperationsRequest struct {
	Force bool
}

type FetchDisruptionsRequest struct {
	DateFrom time.Time
	DateTo   time.Time
	Force    bool
}

type FetchDictionariesResult struct {
	RunID        int64          `json:"run_id"`
	FetchedTypes map[string]int `json:"fetched_types"`
	LakePrefix   string         `json:"lake_prefix,omitempty"`
	DurationMS   int64          `json:"duration_ms,omitempty"`
}

type FetchResult struct {
	RunID          int64  `json:"run_id"`
	Pipeline       string `json:"pipeline"`
	Status         string `json:"status"`
	RecordsFetched int    `json:"records_fetched"`
	PagesLanded    int    `json:"pages_landed"`
	LakePrefix     string `json:"lake_prefix,omitempty"`
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
	ListIngestionRuns(ctx context.Context, pipeline *string, limit int) ([]IngestionRun, error)
}

type Lake interface {
	Ping(ctx context.Context) error
	PutRawDictionaries(ctx context.Context, dictionaryType string, page int, payload []byte, recordCount int, runID int64) (string, error)
	PutRawSchedules(ctx context.Context, dateFrom time.Time, dateTo time.Time, page int, payload []byte, recordCount int, runID int64) (string, error)
	PutRawOperations(ctx context.Context, captureDate time.Time, page int, payload []byte, recordCount int, runID int64) (string, error)
	PutRawDisruptions(ctx context.Context, dateFrom time.Time, dateTo time.Time, payload []byte, recordCount int, runID int64) (string, error)
}

type PLKClient interface {
	FetchDictionaries(ctx context.Context) (map[string][]byte, error)
	FetchStationPage(ctx context.Context, page int, pageSize int) ([]byte, error)
	FetchScheduleRoutes(ctx context.Context, date time.Time) ([]byte, error)
	FetchScheduleRoute(ctx context.Context, scheduleID int, orderID int) ([]byte, error)
	FetchOperations(ctx context.Context, page int, pageSize int) ([]byte, error)
	FetchDisruptions(ctx context.Context, dateFrom time.Time, dateTo time.Time) ([]byte, error)
}

type Service struct {
	repo      Repository
	lake      Lake
	plkClient PLKClient
	tracer    trace.Tracer
}

func New(repo Repository, lake Lake, plkClient PLKClient) *Service {
	return &Service{
		repo:      repo,
		lake:      lake,
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

	if err := s.lake.Ping(ctx); err != nil {
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
	var lakePrefix string
	for dictionaryType, payload := range dictionaryPayloads {
		recordCount := countRecords(payload)
		key, err := s.lake.PutRawDictionaries(ctx, dictionaryType, 1, payload, recordCount, runID)
		if err != nil {
			return FetchDictionariesResult{}, s.failRun(ctx, runID, err, "fetch dictionaries: put to lake")
		}
		if lakePrefix == "" {
			lakePrefix = path.Dir(key) + "/"
		}

		totalRecords += recordCount
		fetchedTypes[dictionaryType] = recordCount
	}

	stationRecords, stationPrefix, err := s.fetchStations(ctx, runID)
	if err != nil {
		return FetchDictionariesResult{}, s.failRun(ctx, runID, err, "fetch dictionaries")
	}
	if lakePrefix == "" {
		lakePrefix = stationPrefix
	}
	totalRecords += stationRecords
	fetchedTypes["stations"] = stationRecords

	if err := s.repo.MarkIngestionRunSuccess(ctx, runID, totalRecords); err != nil {
		return FetchDictionariesResult{}, fmt.Errorf("fetch dictionaries: mark success: %w", err)
	}

	return FetchDictionariesResult{
		RunID:        runID,
		FetchedTypes: fetchedTypes,
		LakePrefix:   lakePrefix,
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
	captureDate := start.UTC()

	if !req.Force {
		running, err := s.repo.IsPipelineRunning(ctx, "operations", captureDate)
		if err != nil {
			return FetchResult{}, fmt.Errorf("fetch operations: check running: %w", err)
		}
		if running {
			return FetchResult{}, ErrPipelineRunning
		}
	}

	runID, err := s.repo.CreateIngestionRun(ctx, "operations", captureDate)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetch operations: create ingestion run: %w", err)
	}

	recordCount, pagesLanded, lakePrefix, err := s.fetchOperationsPages(ctx, captureDate, runID)
	if err != nil {
		return FetchResult{}, s.failRun(ctx, runID, err, "fetch operations")
	}

	if err := s.repo.MarkIngestionRunSuccess(ctx, runID, recordCount); err != nil {
		return FetchResult{}, fmt.Errorf("fetch operations: mark success: %w", err)
	}

	return FetchResult{
		RunID:          runID,
		Pipeline:       "operations",
		Status:         "success",
		RecordsFetched: recordCount,
		PagesLanded:    pagesLanded,
		LakePrefix:     lakePrefix,
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
		recordCount int
		pagesLanded int
		lakePrefix  string
		fetchErr    error
	)

	switch pipeline {
	case "schedules":
		recordCount, pagesLanded, lakePrefix, fetchErr = s.fetchScheduleDetails(ctx, dateFrom, dateTo, runID)
	case "disruptions":
		payload, err := s.plkClient.FetchDisruptions(ctx, dateFrom, dateTo)
		fetchErr = err
		if fetchErr == nil {
			recordCount = countRecords(payload)
			key, putErr := s.lake.PutRawDisruptions(ctx, dateFrom, dateTo, payload, recordCount, runID)
			fetchErr = putErr
			if fetchErr == nil {
				pagesLanded, lakePrefix = 1, path.Dir(key)+"/"
			}
		}
	default:
		fetchErr = fmt.Errorf("unsupported pipeline: %s", pipeline)
	}

	if fetchErr != nil {
		return FetchResult{}, s.failRun(ctx, runID, fetchErr, fmt.Sprintf("fetch %s", pipeline))
	}

	if err := s.repo.MarkIngestionRunSuccess(ctx, runID, recordCount); err != nil {
		return FetchResult{}, fmt.Errorf("fetch %s: mark success: %w", pipeline, err)
	}

	return FetchResult{
		RunID:          runID,
		Pipeline:       pipeline,
		Status:         "success",
		RecordsFetched: recordCount,
		PagesLanded:    pagesLanded,
		LakePrefix:     lakePrefix,
		DurationMS:     time.Since(start).Milliseconds(),
	}, nil
}

func (s *Service) failRun(ctx context.Context, runID int64, rootErr error, operation string) error {
	traceContext := trace.SpanContextFromContext(ctx)
	failureCtx := context.Background()
	if traceContext.IsValid() {
		failureCtx = trace.ContextWithSpanContext(failureCtx, traceContext)
	}
	failureCtx, cancel := context.WithTimeout(failureCtx, markIngestionRunFailedTimeout)
	defer cancel()

	if markErr := s.repo.MarkIngestionRunFailed(failureCtx, runID, rootErr.Error()); markErr != nil {
		return fmt.Errorf("%s: %w; mark ingestion run failed: %v", operation, rootErr, markErr)
	}

	return fmt.Errorf("%s: %w", operation, rootErr)
}

type stationPageResponse struct {
	Stations   []json.RawMessage `json:"stations"`
	TotalPages int               `json:"totalPages"`
}

type operationPageResponse struct {
	Pagination struct {
		TotalPages  int  `json:"totalPages"`
		HasNextPage bool `json:"hasNextPage"`
	} `json:"pagination"`
}

type scheduleRoutesResponse struct {
	Routes []struct {
		ScheduleID int `json:"scheduleId"`
		OrderID    int `json:"orderId"`
	} `json:"routes"`
}

func (s *Service) fetchStations(ctx context.Context, runID int64) (int, string, error) {
	const pageSize = 10000
	totalRecords := 0
	var lakePrefix string

	for page := 1; ; page++ {
		payload, err := s.plkClient.FetchStationPage(ctx, page, pageSize)
		if err != nil {
			return 0, "", fmt.Errorf("fetch stations page %d: %w", page, err)
		}

		var response stationPageResponse
		if err := json.Unmarshal(payload, &response); err != nil {
			return 0, "", fmt.Errorf("decode stations page %d: %w", page, err)
		}
		totalPages := response.TotalPages
		if totalPages == 0 {
			totalPages = 1 // Empty station responses may report zero pages.
		}
		if totalPages < page {
			return 0, "", fmt.Errorf("decode stations page %d: totalPages %d is invalid", page, response.TotalPages)
		}

		recordCount := len(response.Stations)
		key, err := s.lake.PutRawDictionaries(ctx, "stations", page, payload, recordCount, runID)
		if err != nil {
			return 0, "", fmt.Errorf("land stations page %d: %w", page, err)
		}
		if lakePrefix == "" {
			lakePrefix = path.Dir(key) + "/"
		}
		totalRecords += recordCount

		if page == totalPages {
			return totalRecords, lakePrefix, nil
		}
	}
}

func (s *Service) fetchOperationsPages(ctx context.Context, captureDate time.Time, runID int64) (int, int, string, error) {
	const pageSize = 1000
	totalRecords, pagesLanded := 0, 0
	var lakePrefix string

	for page := 1; ; page++ {
		payload, err := s.plkClient.FetchOperations(ctx, page, pageSize)
		if err != nil {
			return 0, 0, "", fmt.Errorf("fetch operations page %d: %w", page, err)
		}

		var response operationPageResponse
		if err := json.Unmarshal(payload, &response); err != nil {
			return 0, 0, "", fmt.Errorf("decode operations page %d: %w", page, err)
		}
		totalPages := response.Pagination.TotalPages
		if totalPages == 0 {
			totalPages = 1 // Empty operation responses may report zero pages.
		}
		if totalPages < page {
			return 0, 0, "", fmt.Errorf("decode operations page %d: totalPages %d is invalid", page, response.Pagination.TotalPages)
		}
		if response.Pagination.HasNextPage != (page < totalPages) {
			return 0, 0, "", fmt.Errorf("decode operations page %d: hasNextPage conflicts with totalPages", page)
		}

		recordCount := countRecords(payload)
		key, err := s.lake.PutRawOperations(ctx, captureDate, page, payload, recordCount, runID)
		if err != nil {
			return 0, 0, "", fmt.Errorf("land operations page %d: %w", page, err)
		}
		if lakePrefix == "" {
			lakePrefix = path.Dir(key) + "/"
		}
		totalRecords += recordCount
		pagesLanded++

		if !response.Pagination.HasNextPage {
			return totalRecords, pagesLanded, lakePrefix, nil
		}
	}
}

func (s *Service) fetchScheduleDetails(ctx context.Context, dateFrom time.Time, dateTo time.Time, runID int64) (int, int, string, error) {
	type routeKey struct{ scheduleID, orderID int }
	seen := make(map[routeKey]struct{})
	page := 0
	var lakePrefix string

	for date := dateFrom; !date.After(dateTo); date = date.AddDate(0, 0, 1) {
		payload, err := s.plkClient.FetchScheduleRoutes(ctx, date)
		if err != nil {
			return 0, 0, "", fmt.Errorf("fetch schedule routes for %s: %w", date.Format("2006-01-02"), err)
		}

		var routes scheduleRoutesResponse
		if err := json.Unmarshal(payload, &routes); err != nil {
			return 0, 0, "", fmt.Errorf("decode schedule routes for %s: %w", date.Format("2006-01-02"), err)
		}
		for _, route := range routes.Routes {
			key := routeKey{route.ScheduleID, route.OrderID}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}

			detail, err := s.plkClient.FetchScheduleRoute(ctx, route.ScheduleID, route.OrderID)
			if err != nil {
				return 0, 0, "", fmt.Errorf("fetch schedule route %d/%d: %w", route.ScheduleID, route.OrderID, err)
			}
			page++
			landedKey, err := s.lake.PutRawSchedules(ctx, dateFrom, dateTo, page, detail, 1, runID)
			if err != nil {
				return 0, 0, "", fmt.Errorf("land schedule route %d/%d: %w", route.ScheduleID, route.OrderID, err)
			}
			if lakePrefix == "" {
				lakePrefix = path.Dir(landedKey) + "/"
			}
		}
	}

	return page, page, lakePrefix, nil
}

func countRecords(payload []byte) int {
	if len(payload) == 0 {
		return 0
	}

	var asObject map[string]any
	if err := json.Unmarshal(payload, &asObject); err != nil {
		return 0
	}

	for _, key := range []string{"data", "items", "results", "schedules", "operations", "trains", "disruptions", "carriers", "stations", "commercialCategories", "stopTypes", "cities"} {
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
