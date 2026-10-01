package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/repository"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/service"
)

type boardCall struct {
	stationExtID      int
	now               time.Time
	horizon, lookback time.Duration
}

// fakeBoardRepo implements service.Repository for the station board handler.
type fakeBoardRepo struct {
	service.Repository

	stationMissing bool
	calls          []boardCall
}

func (f *fakeBoardRepo) GetStationByExternalId(_ context.Context, extID int) (*model.Station, error) {
	if f.stationMissing {
		return nil, repository.ErrNotFound
	}
	return &model.Station{ExternalID: extID}, nil
}

func (f *fakeBoardRepo) QueryStationBoardCandidates(_ context.Context, stationExtID int, now time.Time, horizon, lookback time.Duration) ([]service.StationBoardCandidate, error) {
	f.calls = append(f.calls, boardCall{stationExtID, now, horizon, lookback})
	return nil, nil
}

func TestHandleGetStationBoard_Params(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		path         string
		wantStatus   int
		wantAt       string
		wantLimit    int
		wantHorizon  time.Duration
		wantLookback time.Duration
	}{
		{
			name:       "defaults",
			path:       "/api/v1/stations/33506/board?at=2026-09-29T10:00:00Z",
			wantStatus: http.StatusOK, wantAt: "2026-09-29T10:00:00Z",
			wantLimit: 10, wantHorizon: 720 * time.Minute, wantLookback: 360 * time.Minute,
		},
		{
			name:       "explicit values, at with offset normalised to UTC",
			path:       "/api/v1/stations/33506/board?at=2026-09-29T12:00:00%2B02:00&limit=50&horizonMinutes=30&lookbackMinutes=0",
			wantStatus: http.StatusOK, wantAt: "2026-09-29T10:00:00Z",
			wantLimit: 50, wantHorizon: 30 * time.Minute, wantLookback: 0,
		},
		{name: "bad externalId", path: "/api/v1/stations/abc/board", wantStatus: http.StatusBadRequest},
		{name: "bad at", path: "/api/v1/stations/1/board?at=2026-09-29", wantStatus: http.StatusBadRequest},
		{name: "limit zero", path: "/api/v1/stations/1/board?limit=0", wantStatus: http.StatusBadRequest},
		{name: "limit too large", path: "/api/v1/stations/1/board?limit=51", wantStatus: http.StatusBadRequest},
		{name: "limit not int", path: "/api/v1/stations/1/board?limit=x", wantStatus: http.StatusBadRequest},
		{name: "horizon too small", path: "/api/v1/stations/1/board?horizonMinutes=29", wantStatus: http.StatusBadRequest},
		{name: "horizon too large", path: "/api/v1/stations/1/board?horizonMinutes=1441", wantStatus: http.StatusBadRequest},
		{name: "lookback negative", path: "/api/v1/stations/1/board?lookbackMinutes=-1", wantStatus: http.StatusBadRequest},
		{name: "lookback too large", path: "/api/v1/stations/1/board?lookbackMinutes=721", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakeBoardRepo{}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			newTestRouter(repo).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				if len(repo.calls) != 0 {
					t.Fatalf("repository called on invalid request")
				}
				return
			}
			if len(repo.calls) != 1 {
				t.Fatalf("QueryStationBoardCandidates calls = %d, want 1", len(repo.calls))
			}
			call := repo.calls[0]
			if call.stationExtID != 33506 || call.horizon != tt.wantHorizon || call.lookback != tt.wantLookback {
				t.Errorf("call = %+v, want station 33506, horizon %v, lookback %v", call, tt.wantHorizon, tt.wantLookback)
			}

			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["at"] != tt.wantAt {
				t.Errorf("at = %v, want %s", body["at"], tt.wantAt)
			}
			if body["limit"] != float64(tt.wantLimit) {
				t.Errorf("limit = %v, want %d", body["limit"], tt.wantLimit)
			}
			if body["horizon_minutes"] != tt.wantHorizon.Minutes() {
				t.Errorf("horizon_minutes = %v, want %v", body["horizon_minutes"], tt.wantHorizon.Minutes())
			}
			if entries, ok := body["entries"].([]any); !ok || len(entries) != 0 {
				t.Errorf("entries = %v, want empty array", body["entries"])
			}
			if _, ok := body["data_as_of"]; ok {
				t.Errorf("data_as_of present on empty board")
			}
		})
	}
}

func TestHandleGetStationBoard_DefaultsToNow(t *testing.T) {
	t.Parallel()

	repo := &fakeBoardRepo{}
	before := time.Now().UTC()
	rec := httptest.NewRecorder()
	newTestRouter(repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/stations/33506/board", nil))
	after := time.Now().UTC()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if len(repo.calls) != 1 {
		t.Fatalf("QueryStationBoardCandidates calls = %d, want 1", len(repo.calls))
	}
	if now := repo.calls[0].now; now.Before(before) || now.After(after) || now.Location() != time.UTC {
		t.Errorf("now = %v, want UTC between %v and %v", now, before, after)
	}
}

func TestHandleGetStationBoard_StationNotFound(t *testing.T) {
	t.Parallel()

	repo := &fakeBoardRepo{stationMissing: true}
	rec := httptest.NewRecorder()
	newTestRouter(repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/stations/999999/board", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", rec.Code, rec.Body.String())
	}
	if len(repo.calls) != 0 {
		t.Fatalf("board queried for a missing station")
	}
}
