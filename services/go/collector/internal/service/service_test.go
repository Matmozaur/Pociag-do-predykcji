package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pociag-do-predykcji/services/go/collector/internal/service"
)

type mockRepository struct {
	isRunning      bool
	isRunningErr   error
	createRunID    int64
	createRunErr   error
	onCreateRun    func()
	markSuccessErr error
	markFailedCtx  context.Context
	markFailedErr  error
	markFailedAt   time.Time
	listRuns       []service.IngestionRun
	listErr        error
}

type mockLake struct {
	putErr error
}

type mockPLKClient struct {
	stationPages       map[int][]byte
	operationPages     map[int][]byte
	scheduleRoutes     map[string][]byte
	fetchedScheduleIDs [][2]int
	schedulesErr       error
}

func (m *mockRepository) Ping(ctx context.Context) error {
	return nil
}

func (m *mockRepository) IsPipelineRunning(ctx context.Context, pipeline string, runDate time.Time) (bool, error) {
	return m.isRunning, m.isRunningErr
}

func (m *mockRepository) CreateIngestionRun(ctx context.Context, pipeline string, runDate time.Time) (int64, error) {
	if m.createRunErr != nil {
		return 0, m.createRunErr
	}
	if m.onCreateRun != nil {
		m.onCreateRun()
	}
	if m.createRunID == 0 {
		return 1, nil
	}
	return m.createRunID, nil
}

func (m *mockRepository) MarkIngestionRunSuccess(ctx context.Context, runID int64, recordsFetched int) error {
	return m.markSuccessErr
}

func (m *mockRepository) MarkIngestionRunFailed(ctx context.Context, runID int64, errorMessage string) error {
	m.markFailedCtx = ctx
	m.markFailedErr = ctx.Err()
	m.markFailedAt, _ = ctx.Deadline()
	return nil
}

func (m *mockRepository) ListIngestionRuns(ctx context.Context, pipeline *string, limit int) ([]service.IngestionRun, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listRuns, nil
}

func (m *mockLake) Ping(ctx context.Context) error {
	return nil
}

func (m *mockLake) PutRawDictionaries(ctx context.Context, dictionaryType string, page int, payload []byte, recordCount int, runID int64) (string, error) {
	if m.putErr != nil {
		return "", m.putErr
	}
	return "raw/dictionaries/2026/06/14/run_1_" + dictionaryType + ".parquet", nil
}

func (m *mockLake) PutRawSchedules(ctx context.Context, dateFrom time.Time, dateTo time.Time, page int, payload []byte, recordCount int, runID int64) (string, error) {
	if m.putErr != nil {
		return "", m.putErr
	}
	return "raw/schedules/2026/06/14/run_1_page_1.parquet", nil
}

func (m *mockLake) PutRawOperations(ctx context.Context, captureDate time.Time, page int, payload []byte, recordCount int, runID int64) (string, error) {
	if m.putErr != nil {
		return "", m.putErr
	}
	return "raw/operations/2026/06/14/run_1_page_1.parquet", nil
}

func (m *mockLake) PutRawDisruptions(ctx context.Context, dateFrom time.Time, dateTo time.Time, payload []byte, recordCount int, runID int64) (string, error) {
	if m.putErr != nil {
		return "", m.putErr
	}
	return "raw/disruptions/2026/06/14/run_1.parquet", nil
}

func (m *mockPLKClient) FetchDictionaries(ctx context.Context) (map[string][]byte, error) {
	return map[string][]byte{"carriers": []byte(`{"items":[]}`)}, nil
}

func (m *mockPLKClient) FetchStationPage(ctx context.Context, page int, pageSize int) ([]byte, error) {
	if payload, ok := m.stationPages[page]; ok {
		return payload, nil
	}
	return []byte(`{"stations":[],"totalPages":1}`), nil
}

func (m *mockPLKClient) FetchScheduleRoutes(ctx context.Context, date time.Time) ([]byte, error) {
	if m.schedulesErr != nil {
		return nil, m.schedulesErr
	}
	if payload, ok := m.scheduleRoutes[date.Format("2006-01-02")]; ok {
		return payload, nil
	}
	return []byte(`{"routes":[]}`), nil
}

func (m *mockPLKClient) FetchScheduleRoute(ctx context.Context, scheduleID int, orderID int) ([]byte, error) {
	m.fetchedScheduleIDs = append(m.fetchedScheduleIDs, [2]int{scheduleID, orderID})
	return []byte(`{"route":[]}`), nil
}

func (m *mockPLKClient) FetchOperations(ctx context.Context, page int, pageSize int) ([]byte, error) {
	if payload, ok := m.operationPages[page]; ok {
		return payload, nil
	}
	return []byte(`{"pagination":{"totalPages":1,"hasNextPage":false},"trains":[]}`), nil
}

func (m *mockPLKClient) FetchDisruptions(ctx context.Context, dateFrom time.Time, dateTo time.Time) ([]byte, error) {
	return []byte(`{"items":[]}`), nil
}

func TestService_FetchSchedules_Success(t *testing.T) {
	t.Parallel()

	repo := &mockRepository{}
	lake := &mockLake{}
	plkClient := &mockPLKClient{scheduleRoutes: map[string][]byte{
		"2026-05-01": []byte(`{"routes":[{"scheduleId":1,"orderId":2}]}`),
		"2026-05-02": []byte(`{"routes":[{"scheduleId":1,"orderId":2},{"scheduleId":3,"orderId":4}]}`),
	}}
	svc := service.New(repo, lake, plkClient)

	result, err := svc.FetchSchedules(context.Background(), service.FetchSchedulesRequest{
		DateFrom: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		DateTo:   time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
	})

	require.NoError(t, err)
	assert.Equal(t, "schedules", result.Pipeline)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, int64(1), result.RunID)
	assert.Equal(t, 2, result.RecordsFetched)
	assert.Equal(t, 2, result.PagesLanded)
	assert.Equal(t, [][2]int{{1, 2}, {3, 4}}, plkClient.fetchedScheduleIDs)
}

func TestService_FetchOperations_PaginatesUsingDocumentedPagination(t *testing.T) {
	t.Parallel()

	repo := &mockRepository{createRunID: 42}
	plkClient := &mockPLKClient{operationPages: map[int][]byte{
		1: []byte(`{"pagination":{"totalPages":2,"hasNextPage":true},"trains":[{"id":1}]}`),
		2: []byte(`{"pagination":{"totalPages":2,"hasNextPage":false},"trains":[{"id":2}]}`),
	}}
	svc := service.New(repo, &mockLake{}, plkClient)

	result, err := svc.FetchOperations(context.Background(), service.FetchOperationsRequest{})

	require.NoError(t, err)
	assert.Equal(t, int64(42), result.RunID)
	assert.Equal(t, 2, result.RecordsFetched)
	assert.Equal(t, 2, result.PagesLanded)
}

func TestService_FetchDictionaries_PaginatesStationsAndReturnsRunID(t *testing.T) {
	t.Parallel()

	plkClient := &mockPLKClient{stationPages: map[int][]byte{
		1: []byte(`{"stations":[{"id":1}],"totalPages":2}`),
		2: []byte(`{"stations":[{"id":2}],"totalPages":2}`),
	}}
	svc := service.New(&mockRepository{createRunID: 7}, &mockLake{}, plkClient)

	result, err := svc.FetchDictionaries(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(7), result.RunID)
	assert.Equal(t, 2, result.FetchedTypes["stations"])
}

func TestService_FetchSchedules_AlreadyRunning_ReturnsConflictError(t *testing.T) {
	t.Parallel()

	repo := &mockRepository{isRunning: true}
	lake := &mockLake{}
	plkClient := &mockPLKClient{}
	svc := service.New(repo, lake, plkClient)

	_, err := svc.FetchSchedules(context.Background(), service.FetchSchedulesRequest{
		DateFrom: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		DateTo:   time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC),
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrPipelineRunning)
}

func TestService_GetFetchStatus_RepositoryError_ReturnsWrappedError(t *testing.T) {
	t.Parallel()

	repo := &mockRepository{listErr: errors.New("db unavailable")}
	lake := &mockLake{}
	plkClient := &mockPLKClient{}
	svc := service.New(repo, lake, plkClient)

	_, err := svc.GetFetchStatus(context.Background(), nil, 10)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get fetch status")
}

func TestService_FetchSchedules_CanceledContextMarksRunFailed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	rootErr := errors.New("upstream unavailable")
	repo := &mockRepository{onCreateRun: cancel}
	svc := service.New(repo, &mockLake{}, &mockPLKClient{schedulesErr: rootErr})

	_, err := svc.FetchSchedules(ctx, service.FetchSchedulesRequest{
		DateFrom: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		DateTo:   time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, rootErr)
	require.NotNil(t, repo.markFailedCtx)
	assert.NoError(t, repo.markFailedErr)
	assert.WithinDuration(t, time.Now().Add(5*time.Second), repo.markFailedAt, time.Second)
}
