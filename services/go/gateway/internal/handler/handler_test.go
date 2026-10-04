package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pociag-do-predykcji/services/go/gateway/internal/client/dataservice"
	"github.com/pociag-do-predykcji/services/go/gateway/internal/service"
)

func TestParseCSVInts_InvalidValue_ReturnsError(t *testing.T) {
	_, err := parseCSVInts("10,abc")
	if err == nil {
		t.Fatal("expected error for invalid CSV integer")
	}
}

func TestParseLimitOffset_ValidQuery_ReturnsValues(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/schedules/search?limit=25&offset=5", nil)
	limit, offset, err := parseLimitOffset(req, 20, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limit != 25 || offset != 5 {
		t.Fatalf("unexpected values: limit=%d offset=%d", limit, offset)
	}
}

// newBoardRouter wires the real handler, service and data-service client against a fake
// data-service that serves the given handler.
func newBoardRouter(t *testing.T, ds http.HandlerFunc) http.Handler {
	t.Helper()
	srv := httptest.NewServer(ds)
	t.Cleanup(srv.Close)

	client, err := dataservice.New(srv.URL, time.Second, nil)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	r := chi.NewRouter()
	New(service.New(client)).RegisterRoutes(r)
	return r
}

func TestHandleGetStationBoard_InvalidParams_Returns400(t *testing.T) {
	router := newBoardRouter(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("data-service should not be called, got %s", r.URL.Path)
	})

	for _, target := range []string{
		"/api/v1/stations/abc/board",
		"/api/v1/stations/33506/board?limit=0",
		"/api/v1/stations/33506/board?limit=51",
		"/api/v1/stations/33506/board?limit=x",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", target, rec.Code)
		}
	}
}

func TestHandleGetStationBoard_StationNotFound_Returns404(t *testing.T) {
	router := newBoardRouter(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/stations/999/board", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleGetStationBoard_DefaultLimit_ReturnsEmptySections(t *testing.T) {
	var gotLimit string
	router := newBoardRouter(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/stations/33506":
			_, _ = w.Write([]byte(`{"id":1,"external_id":33506,"name":"Kraków Główny"}`))
		case "/api/v1/stations/33506/board":
			gotLimit = r.URL.Query().Get("limit")
			_, _ = w.Write([]byte(`{"station_external_id":33506,"at":"2026-09-29T18:00:00Z","limit":10,"horizon_minutes":720,"entries":[]}`))
		case "/api/v1/carriers":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/stations/33506/board", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if gotLimit != "10" {
		t.Fatalf("expected default limit 10, got %q", gotLimit)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	for _, key := range []string{"at_station", "arrivals", "departures"} {
		if string(body[key]) != "[]" {
			t.Errorf("expected %s to be [], got %s", key, body[key])
		}
	}
	if _, ok := body["data_as_of"]; ok {
		t.Error("expected data_as_of to be omitted for an empty board")
	}
}

func TestHandleGetMapTrains_InvalidCarriers_Returns400(t *testing.T) {
	router := newBoardRouter(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("data-service should not be called, got %s", r.URL.Path)
	})

	for _, target := range []string{
		"/api/v1/map/trains?carriers=,",
		"/api/v1/map/trains?carriers=%20,%20",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", target, rec.Code)
		}
	}
}

func TestHandleGetMapTrains_ForwardsCarriers_Returns200(t *testing.T) {
	var gotCarriers string
	router := newBoardRouter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/operations/active" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		gotCarriers = r.URL.Query().Get("carrierCodes")
		_, _ = w.Write([]byte(`{"data":[
			{"operation_id":1,"schedule_id":2026,"order_id":11,"operating_date":"2026-10-02","train_status":"P",
			 "phase":"en_route","confidence":"high","last_seen_at":"2026-10-02T09:58:00Z",
			 "position":{"latitude":50.1,"longitude":19.9,"progress":0.4,"method":"interpolated"}},
			{"operation_id":2,"schedule_id":2026,"order_id":12,"operating_date":"2026-10-02","train_status":"S",
			 "phase":"not_departed","confidence":"low","last_seen_at":"2026-10-02T09:58:00Z"}
		],"total":2,"generated_at":"2026-10-02T10:00:00Z","data_as_of":"2026-10-02T09:58:00Z"}`))
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/map/trains?carriers=IC,%20KM", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if gotCarriers != "IC,KM" {
		t.Fatalf("expected carrierCodes=IC,KM, got %q", gotCarriers)
	}
	var body struct {
		Trains            []map[string]json.RawMessage `json:"trains"`
		UnpositionedCount int                          `json:"unpositioned_count"`
		GeneratedAt       string                       `json:"generated_at"`
		DataAsOf          string                       `json:"data_as_of"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Trains) != 1 || body.UnpositionedCount != 1 {
		t.Fatalf("unexpected trains/unpositioned: %d/%d", len(body.Trains), body.UnpositionedCount)
	}
	if body.GeneratedAt != "2026-10-02T10:00:00Z" || body.DataAsOf != "2026-10-02T09:58:00Z" {
		t.Fatalf("unexpected timestamps: %s %s", body.GeneratedAt, body.DataAsOf)
	}
	if got := string(body.Trains[0]["train_name"]); got != `"Pociąg 2026/11"` {
		t.Fatalf("unexpected train_name: %s", got)
	}
	for _, key := range []string{"carrier_code", "delay_minutes", "previous_stop", "next_stop", "origin", "destination"} {
		if _, ok := body.Trains[0][key]; ok {
			t.Errorf("expected %s to be omitted, got %s", key, body.Trains[0][key])
		}
	}
}
