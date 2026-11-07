package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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
		"/api/v1/trains/live?sort=speed",
		"/api/v1/trains/live?offset=1000001",
		"/api/v1/disruptions?offset=1000001",
		"/api/v1/schedules/search?from=1&to=2&date=2026-10-05&offset=1000001",
		"/api/v1/trains/-5",
		"/api/v1/disruptions?limit=101",
		"/api/v1/schedules/3/delay-prediction",
		"/api/v1/schedules/3/delay-prediction?date=2026-10-05&from=0",
		"/api/v1/schedules/3/delay-prediction?date=2000-01-01",
	} {
		rec := serve(&servicetest.Repository{}, target)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", target, rec.Code)
		}
	}
}

func TestNotFound(t *testing.T) {
	for _, target := range []string{"/api/v1/trains/42", "/api/v1/schedules/42", "/api/v1/stations/42/board", "/api/v1/schedules/42/delay-prediction?date=" + time.Now().AddDate(0, 0, 1).Format(time.DateOnly)} {
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

func TestLiveTrainsAcceptsSortByDelay(t *testing.T) {
	for _, target := range []string{"/api/v1/trains/live", "/api/v1/trains/live?sort=", "/api/v1/trains/live?sort=delay"} {
		rec := serve(&servicetest.Repository{}, target)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", target, rec.Code)
		}
	}
}

func TestOffsetAtSpecMaximum(t *testing.T) {
	for _, target := range []string{
		"/api/v1/trains/live?offset=1000000",
		"/api/v1/disruptions?offset=1000000",
		"/api/v1/schedules/search?from=1&to=2&date=2026-10-05&offset=1000000",
	} {
		if rec := serve(&servicetest.Repository{}, target); rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", target, rec.Code)
		}
	}
}

func TestServerSpansAreNamedByRoute(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	reader := sdkmetric.NewManualReader()
	meters := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	h := handler.New(service.New(&servicetest.Repository{}), zap.NewNop())
	server := otelhttp.NewHandler(h.Routes(), "pociag.api",
		otelhttp.WithTracerProvider(provider), otelhttp.WithMeterProvider(meters))

	for _, c := range []struct {
		method, target, name, route string
	}{
		{http.MethodGet, "/api/v1/trains/42", "GET /api/v1/trains/{operationId}", "/api/v1/trains/{operationId}"},
		{http.MethodGet, "/api/v1/trains/43", "GET /api/v1/trains/{operationId}", "/api/v1/trains/{operationId}"},
		{http.MethodGet, "/api/v1/dashboard/overview", "GET /api/v1/dashboard/overview", "/api/v1/dashboard/overview"},
		{http.MethodGet, "/healthz", "GET /healthz", "/healthz"},
		// Unrouted requests must not put the raw path in the span name.
		{http.MethodGet, "/no/such/path", "GET", ""},
		{http.MethodGet, "/api/v1/no/such/path", "GET", ""},
		{http.MethodOptions, "/api/v1/trains/42", "OPTIONS", ""},
	} {
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, httptest.NewRequest(c.method, c.target, nil))

		spans := recorder.Ended()
		span := spans[len(spans)-1]
		if span.Name() != c.name {
			t.Errorf("%s %s: span name = %q, want %q", c.method, c.target, span.Name(), c.name)
		}
		route := ""
		for _, attr := range span.Attributes() {
			if attr.Key == attribute.Key("http.route") {
				route = attr.Value.AsString()
			}
		}
		if route != c.route {
			t.Errorf("%s %s: http.route = %q, want %q", c.method, c.target, route, c.route)
		}
	}

	// The request metrics carry the route too, so they can be split per endpoint.
	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &metrics); err != nil {
		t.Fatal(err)
	}
	routes := map[string]bool{}
	for _, scope := range metrics.ScopeMetrics {
		for _, m := range scope.Metrics {
			if hist, ok := m.Data.(metricdata.Histogram[float64]); ok {
				for _, point := range hist.DataPoints {
					if v, ok := point.Attributes.Value("http.route"); ok {
						routes[v.AsString()] = true
					}
				}
			}
		}
	}
	for _, want := range []string{"/api/v1/trains/{operationId}", "/api/v1/dashboard/overview", "/healthz"} {
		if !routes[want] {
			t.Errorf("no request metric labelled http.route=%q; got %v", want, routes)
		}
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
