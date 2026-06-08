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
	insertErr      error
	markSuccessErr error
	listRuns       []service.IngestionRun
	listErr        error
}

type mockPLKClient struct {
	schedulesPayload []byte
	schedulesErr     error
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
	if m.createRunID == 0 {
		return 1, nil
	}
	return m.createRunID, nil
}

func (m *mockRepository) MarkIngestionRunSuccess(ctx context.Context, runID int64, recordsFetched int) error {
	return m.markSuccessErr
}

func (m *mockRepository) MarkIngestionRunFailed(ctx context.Context, runID int64, errorMessage string) error {
	return nil
}

func (m *mockRepository) InsertRawDictionaries(ctx context.Context, dictionaryType string, payload []byte, recordCount int, ingestionRunID int64) error {
	return m.insertErr
}

func (m *mockRepository) InsertRawSchedules(ctx context.Context, dateFrom time.Time, dateTo time.Time, page int, payload []byte, recordCount int, ingestionRunID int64) error {
	return m.insertErr
}

func (m *mockRepository) InsertRawOperations(ctx context.Context, operatingDate time.Time, page int, payload []byte, recordCount int, ingestionRunID int64) error {
	return m.insertErr
}

func (m *mockRepository) InsertRawDisruptions(ctx context.Context, dateFrom time.Time, dateTo time.Time, payload []byte, recordCount int, ingestionRunID int64) error {
	return m.insertErr
}

func (m *mockRepository) ListIngestionRuns(ctx context.Context, pipeline *string, limit int) ([]service.IngestionRun, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listRuns, nil
}

func (m *mockPLKClient) FetchDictionaries(ctx context.Context) (map[string][]byte, error) {
	return map[string][]byte{"carriers": []byte(`{"items":[]}`)}, nil
}

func (m *mockPLKClient) FetchSchedules(ctx context.Context, dateFrom time.Time, dateTo time.Time, page int, pageSize int) ([]byte, error) {
	if m.schedulesErr != nil {
		return nil, m.schedulesErr
	}
	if len(m.schedulesPayload) == 0 {
		return []byte(`{"items":[]}`), nil
	}
	return m.schedulesPayload, nil
}

func (m *mockPLKClient) FetchOperations(ctx context.Context, operatingDate time.Time, page int, pageSize int) ([]byte, error) {
	return []byte(`{"items":[]}`), nil
}

func (m *mockPLKClient) FetchDisruptions(ctx context.Context, dateFrom time.Time, dateTo time.Time) ([]byte, error) {
	return []byte(`{"items":[]}`), nil
}

func TestService_FetchSchedules_Success(t *testing.T) {
	t.Parallel()

	repo := &mockRepository{}
	plkClient := &mockPLKClient{schedulesPayload: []byte(`{"items":[{"id":1},{"id":2}]}`)}
	svc := service.New(repo, plkClient)

	result, err := svc.FetchSchedules(context.Background(), service.FetchSchedulesRequest{
		DateFrom: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		DateTo:   time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC),
	})

	require.NoError(t, err)
	assert.Equal(t, "schedules", result.Pipeline)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, 2, result.RecordsFetched)
	assert.Equal(t, 1, result.PagesLanded)
}

func TestService_FetchSchedules_AlreadyRunning_ReturnsConflictError(t *testing.T) {
	t.Parallel()

	repo := &mockRepository{isRunning: true}
	plkClient := &mockPLKClient{}
	svc := service.New(repo, plkClient)

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
	plkClient := &mockPLKClient{}
	svc := service.New(repo, plkClient)

	_, err := svc.GetFetchStatus(context.Background(), nil, 10)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get fetch status")
}
