package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/service"
)

type activeCall struct {
	at           time.Time
	dates        []time.Time
	carrierCodes []string
}

// fakeActiveRepo implements service.Repository for the active operations handler.
type fakeActiveRepo struct {
	service.Repository

	calls []activeCall
}

func (f *fakeActiveRepo) ListActiveOperationCandidates(_ context.Context, at time.Time, dates []time.Time, carrierCodes []string) ([]service.ActiveOperationCandidate, time.Time, error) {
	f.calls = append(f.calls, activeCall{at, dates, carrierCodes})
	return nil, time.Date(2026, 9, 29, 9, 55, 0, 0, time.UTC), nil
}

func TestHandleListActiveOperations_Params(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		path         string
		wantStatus   int
		wantAt       string // "" = current time
		wantCarriers []string
	}{
		{name: "defaults", path: "/api/v1/operations/active", wantStatus: http.StatusOK},
		{
			name:       "at with offset normalised to UTC, carriers trimmed",
			path:       "/api/v1/operations/active?at=2026-09-29T12:00:00%2B02:00&carrierCodes=IC,%20KM&limit=5000",
			wantStatus: http.StatusOK, wantAt: "2026-09-29T10:00:00Z", wantCarriers: []string{"IC", "KM"},
		},
		{name: "bad at", path: "/api/v1/operations/active?at=2026-09-29", wantStatus: http.StatusBadRequest},
		{name: "bad at garbage", path: "/api/v1/operations/active?at=now", wantStatus: http.StatusBadRequest},
		{name: "limit zero", path: "/api/v1/operations/active?limit=0", wantStatus: http.StatusBadRequest},
		{name: "limit too large", path: "/api/v1/operations/active?limit=5001", wantStatus: http.StatusBadRequest},
		{name: "limit not int", path: "/api/v1/operations/active?limit=x", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakeActiveRepo{}
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
				var body map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] != "invalid_request" {
					t.Errorf("error body = %s", rec.Body.String())
				}
				return
			}
			if len(repo.calls) != 1 {
				t.Fatalf("ListActiveOperationCandidates calls = %d, want 1", len(repo.calls))
			}
			call := repo.calls[0]
			if tt.wantAt != "" && call.at.Format(time.RFC3339) != tt.wantAt {
				t.Errorf("at = %s, want %s", call.at.Format(time.RFC3339), tt.wantAt)
			}
			if tt.wantAt == "" && time.Since(call.at) > time.Minute {
				t.Errorf("at = %s, want the current time", call.at)
			}
			if len(call.dates) != 2 {
				t.Errorf("dates = %v, want today and yesterday", call.dates)
			}
			if !reflect.DeepEqual(call.carrierCodes, tt.wantCarriers) {
				t.Errorf("carrierCodes = %v, want %v", call.carrierCodes, tt.wantCarriers)
			}

			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if data, ok := body["data"].([]any); !ok || len(data) != 0 {
				t.Errorf("data = %v, want empty array", body["data"])
			}
			if body["total"] != float64(0) || body["data_as_of"] != "2026-09-29T09:55:00Z" || body["generated_at"] == nil {
				t.Errorf("body = %v", body)
			}
		})
	}
}

func TestHandleListActiveOperations_Cache(t *testing.T) {
	t.Parallel()

	repo := &fakeActiveRepo{}
	router := newTestRouter(repo)
	get := func(path string) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", path, rec.Code)
		}
	}

	get("/api/v1/operations/active?carrierCodes=KM,IC")
	get("/api/v1/operations/active?carrierCodes=IC,KM") // same params, cached
	if len(repo.calls) != 1 {
		t.Fatalf("calls after cached request = %d, want 1", len(repo.calls))
	}
	get("/api/v1/operations/active?carrierCodes=IC") // different params
	if len(repo.calls) != 2 {
		t.Fatalf("calls after new params = %d, want 2", len(repo.calls))
	}
	get("/api/v1/operations/active?at=2026-09-29T10:00:00Z") // explicit at: never cached
	get("/api/v1/operations/active?at=2026-09-29T10:00:00Z")
	if len(repo.calls) != 4 {
		t.Fatalf("calls after explicit at = %d, want 4", len(repo.calls))
	}
}

func TestResponseCache_Expiry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)
	c := newResponseCache(activeOperationsCacheTTL)
	c.now = func() time.Time { return now }

	c.put("k", nil)
	if _, ok := c.get("k"); !ok {
		t.Fatal("fresh entry missing")
	}
	now = now.Add(activeOperationsCacheTTL)
	if _, ok := c.get("k"); ok {
		t.Fatal("entry still served after TTL")
	}
	c.put("other", nil)
	if _, ok := c.entries["k"]; ok {
		t.Fatal("expired entry not pruned on put")
	}
}
