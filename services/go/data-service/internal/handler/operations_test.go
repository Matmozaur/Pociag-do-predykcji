package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/service"
)

// fakeRepo implements service.Repository; only the methods under test are overridden.
type fakeRepo struct {
	service.Repository

	queryOperationsCalls []service.QueryOperationsParams
	statistics           *model.OperationStatistics
}

func (f *fakeRepo) QueryOperations(_ context.Context, p service.QueryOperationsParams) ([]model.OperationSummary, int64, error) {
	f.queryOperationsCalls = append(f.queryOperationsCalls, p)
	return []model.OperationSummary{}, 0, nil
}

func (f *fakeRepo) GetOperationStatistics(_ context.Context, _ time.Time) (*model.OperationStatistics, error) {
	return f.statistics, nil
}

func newTestRouter(repo service.Repository) http.Handler {
	r := chi.NewRouter()
	New(service.New(repo)).RegisterRoutes(r)
	return r
}

func TestHandleQueryOperations_ActiveOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantActive bool
	}{
		{name: "absent defaults to false", query: "", wantStatus: http.StatusOK, wantActive: false},
		{name: "true", query: "activeOnly=true", wantStatus: http.StatusOK, wantActive: true},
		{name: "false", query: "activeOnly=false", wantStatus: http.StatusOK, wantActive: false},
		{name: "invalid", query: "activeOnly=maybe", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakeRepo{}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/operations/?date=2026-09-27&"+tt.query, nil)
			newTestRouter(repo).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				if len(repo.queryOperationsCalls) != 0 {
					t.Fatalf("repository called on invalid request")
				}
				return
			}
			if len(repo.queryOperationsCalls) != 1 {
				t.Fatalf("QueryOperations calls = %d, want 1", len(repo.queryOperationsCalls))
			}
			got := repo.queryOperationsCalls[0]
			if got.ActiveOnly != tt.wantActive {
				t.Errorf("ActiveOnly = %v, want %v", got.ActiveOnly, tt.wantActive)
			}
			if got.Date == nil || got.Date.Format("2006-01-02") != "2026-09-27" {
				t.Errorf("Date = %v, want 2026-09-27", got.Date)
			}
		})
	}
}

func TestHandleGetOperationStatistics_LastUpdatedAt(t *testing.T) {
	t.Parallel()

	lastUpdated := time.Date(2026, 9, 27, 6, 13, 20, 0, time.UTC)
	repo := &fakeRepo{statistics: &model.OperationStatistics{
		Date:          "2026-09-27",
		Total:         1,
		ByStatus:      map[string]int{"P": 1},
		LastUpdatedAt: &lastUpdated,
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/operations/statistics?date=2026-09-27", nil)
	newTestRouter(repo).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["last_updated_at"] != "2026-09-27T06:13:20Z" {
		t.Errorf("last_updated_at = %v, want 2026-09-27T06:13:20Z", body["last_updated_at"])
	}
}
