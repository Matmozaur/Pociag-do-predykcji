package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/pociag-do-predykcji/services/go/api/internal/handler"
	"github.com/pociag-do-predykcji/services/go/api/internal/service"
	"github.com/pociag-do-predykcji/services/go/api/internal/service/servicetest"
)

func serve(repo *servicetest.Repository, target string) *httptest.ResponseRecorder {
	h := handler.New(service.New(repo), zap.NewNop())
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestBadRequests(t *testing.T) {
	for _, target := range []string{
		"/api/v1/search/stations?q=W",
		"/api/v1/search/stations?q=Wa&limit=21",
		"/api/v1/search/stations?q=Wa&limit=abc",
		"/api/v1/stations/abc/board",
		"/api/v1/stations/0/board",
		"/api/v1/stations/5100/board?limit=51",
		"/api/v1/schedules/search?from=Warszawa&to=Krak%C3%B3w",
		"/api/v1/schedules/search?from=Warszawa&to=Krak%C3%B3w&date=05.10.2026",
		"/api/v1/schedules/search?from=&to=Krak%C3%B3w&date=2026-10-05",
		"/api/v1/schedules/search?from=1&to=2&date=2026-10-05&sort=price",
		"/api/v1/schedules/x",
		"/api/v1/trains/live?limit=0",
		"/api/v1/trains/live?offset=-1",
		"/api/v1/trains/-5",
		"/api/v1/disruptions?limit=101",
	} {
		rec := serve(&servicetest.Repository{}, target)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", target, rec.Code)
		}
	}
}

func TestNotFound(t *testing.T) {
	for _, target := range []string{"/api/v1/trains/42", "/api/v1/schedules/42", "/api/v1/stations/42/board"} {
		rec := serve(&servicetest.Repository{}, target)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, rec.Code)
		}
	}
}

func TestInternalError(t *testing.T) {
	rec := serve(&servicetest.Repository{Err: errors.New("db down")}, "/api/v1/dashboard/overview")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "internal_error" || body["message"] == "" {
		t.Errorf("body = %v", body)
	}
}

func TestMapStations(t *testing.T) {
	lat, lon := 52.23, 21.0
	repo := &servicetest.Repository{Stations: []service.Station{
		{ID: 5100, Name: "Warszawa Centralna", Latitude: &lat, Longitude: &lon},
	}}
	rec := serve(repo, "/api/v1/map/stations")

	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("code = %d, content type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	want := `{"stations":[{"external_id":5100,"name":"Warszawa Centralna","latitude":52.23,"longitude":21}]}` + "\n"
	if rec.Body.String() != want {
		t.Errorf("body = %s, want %s", rec.Body.String(), want)
	}
}

func TestSearchSchedulesParsesPlaces(t *testing.T) {
	repo := &servicetest.Repository{}
	rec := serve(repo, "/api/v1/schedules/search?from=5100&to=Krak%C3%B3w&date=2026-10-05&carriers=IC,%20KM,")

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d: %s", rec.Code, rec.Body.String())
	}
	if repo.Query.From.StationID != 5100 || repo.Query.To.Name != "Kraków" {
		t.Errorf("query places = %+v → %+v", repo.Query.From, repo.Query.To)
	}
	if len(repo.Query.Carriers) != 2 || repo.Query.Carriers[1] != "KM" {
		t.Errorf("carriers = %q", repo.Query.Carriers)
	}
}

func TestHealthAndCORS(t *testing.T) {
	rec := serve(&servicetest.Repository{}, "/healthz")
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("healthz = %d, CORS %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec := serve(&servicetest.Repository{Err: errors.New("down")}, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz with db down = %d, want 503", rec.Code)
	}
}
