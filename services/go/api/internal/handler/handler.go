// Package handler is the HTTP layer of the api service: routing, parameter validation and JSON
// encoding. The contract is specs/openapi/api.yml.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/pociag-do-predykcji/services/go/api/internal/model"
	"github.com/pociag-do-predykcji/services/go/api/internal/service"
)

type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

func New(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(cors)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", h.ready)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/search/stations", h.searchStations)
		r.Get("/map/stations", h.mapStations)
		r.Get("/map/trains", h.mapTrains)
		r.Get("/stations/{stationId}/board", h.stationBoard)
		r.Get("/schedules/search", h.searchSchedules)
		r.Get("/schedules/{routeId}", h.scheduleDetail)
		r.Get("/trains/live", h.liveTrains)
		r.Get("/trains/{operationId}", h.trainDetail)
		r.Get("/disruptions", h.disruptions)
		r.Get("/dashboard/overview", h.dashboardOverview)
	})
	return r
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Ready(r.Context()); err != nil {
		h.logger.Warn("not ready", zap.Error(err))
		writeJSON(w, http.StatusServiceUnavailable, model.ErrorResponse{Error: "not_ready", Message: "service is not ready"})
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) searchStations(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		badRequest(w, "q must have at least 2 characters")
		return
	}
	limit, ok := intParam(w, r, "limit", 10, 1, 20)
	if !ok {
		return
	}
	h.respond(w, r, "failed to search stations")(h.svc.SearchStations(r.Context(), q, limit))
}

func (h *Handler) mapStations(w http.ResponseWriter, r *http.Request) {
	h.respond(w, r, "failed to load map stations")(h.svc.MapStations(r.Context()))
}

func (h *Handler) mapTrains(w http.ResponseWriter, r *http.Request) {
	h.respond(w, r, "failed to load map trains")(h.svc.MapTrains(r.Context(), csv(r, "carriers")))
}

func (h *Handler) stationBoard(w http.ResponseWriter, r *http.Request) {
	stationID, ok := idParam(w, r, "stationId")
	if !ok {
		return
	}
	limit, ok := intParam(w, r, "limit", 10, 1, 50)
	if !ok {
		return
	}
	h.respond(w, r, "failed to load station board")(h.svc.StationBoard(r.Context(), int(stationID), limit))
}

func (h *Handler) searchSchedules(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	from, to := service.ParsePlace(query.Get("from")), service.ParsePlace(query.Get("to"))
	if (from.StationID == 0 && from.Name == "") || (to.StationID == 0 && to.Name == "") {
		badRequest(w, "from and to are required")
		return
	}
	date, err := time.Parse(time.DateOnly, query.Get("date"))
	if err != nil {
		badRequest(w, "date must be in YYYY-MM-DD format")
		return
	}
	sortBy := query.Get("sort")
	switch sortBy {
	case "":
		sortBy = service.SortDeparture
	case service.SortDeparture, service.SortArrival, service.SortDuration:
	default:
		badRequest(w, "sort must be one of: departure, arrival, duration")
		return
	}
	limit, ok := intParam(w, r, "limit", 20, 1, 100)
	if !ok {
		return
	}
	offset, ok := intParam(w, r, "offset", 0, 0, 1_000_000)
	if !ok {
		return
	}
	q := service.ConnectionQuery{
		Date: date, From: from, To: to, Carriers: csv(r, "carriers"), Categories: csv(r, "categories"),
	}
	h.respond(w, r, "failed to search schedules")(h.svc.SearchSchedules(r.Context(), q, sortBy, limit, offset))
}

func (h *Handler) scheduleDetail(w http.ResponseWriter, r *http.Request) {
	trainID, ok := idParam(w, r, "routeId")
	if !ok {
		return
	}
	h.respond(w, r, "failed to load schedule")(h.svc.ScheduleDetail(r.Context(), trainID))
}

func (h *Handler) liveTrains(w http.ResponseWriter, r *http.Request) {
	limit, ok := intParam(w, r, "limit", 20, 1, 100)
	if !ok {
		return
	}
	offset, ok := intParam(w, r, "offset", 0, 0, 1_000_000)
	if !ok {
		return
	}
	h.respond(w, r, "failed to load live trains")(h.svc.LiveTrains(r.Context(), csv(r, "carriers"), limit, offset))
}

func (h *Handler) trainDetail(w http.ResponseWriter, r *http.Request) {
	operationID, ok := idParam(w, r, "operationId")
	if !ok {
		return
	}
	h.respond(w, r, "failed to load train")(h.svc.TrainDetail(r.Context(), operationID))
}

func (h *Handler) disruptions(w http.ResponseWriter, r *http.Request) {
	limit, ok := intParam(w, r, "limit", 20, 1, 100)
	if !ok {
		return
	}
	offset, ok := intParam(w, r, "offset", 0, 0, 1_000_000)
	if !ok {
		return
	}
	h.respond(w, r, "failed to list disruptions")(h.svc.Disruptions(r.Context(), limit, offset))
}

func (h *Handler) dashboardOverview(w http.ResponseWriter, r *http.Request) {
	h.respond(w, r, "failed to load dashboard")(h.svc.DashboardOverview(r.Context()))
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// respond returns a function that writes a service result: 200 with the payload, 404 for
// service.ErrNotFound, 500 (logged) otherwise. Usage: h.respond(w, r, msg)(h.svc.X(...)).
func (h *Handler) respond(w http.ResponseWriter, r *http.Request, failure string) func(any, error) {
	return func(payload any, err error) {
		if err != nil {
			h.fail(w, r, err, failure)
			return
		}
		writeJSON(w, http.StatusOK, payload)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error, message string) {
	span := trace.SpanFromContext(r.Context())
	span.RecordError(err)
	if errors.Is(err, service.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: "resource not found"})
		return
	}
	span.SetStatus(codes.Error, err.Error())
	h.logger.Error(message, zap.Error(err), zap.String("path", r.URL.Path),
		zap.String("trace_id", span.SpanContext().TraceID().String()))
	writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload) // the status line is already sent
}

func badRequest(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "invalid_request", Message: message})
}

// intParam reads an optional integer query parameter in [lo, hi]; on error it writes a 400.
func intParam(w http.ResponseWriter, r *http.Request, name string, fallback, lo, hi int) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < lo || v > hi {
		badRequest(w, fmt.Sprintf("%s must be an integer between %d and %d", name, lo, hi))
		return 0, false
	}
	return v, true
}

// idParam reads a positive integer path parameter; on error it writes a 400.
func idParam(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	v, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || v < 1 {
		badRequest(w, name+" must be a positive integer")
		return 0, false
	}
	return v, true
}

// csv reads a comma-separated query parameter, dropping empty items.
func csv(r *http.Request, name string) []string {
	var out []string
	for _, item := range strings.Split(r.URL.Query().Get(name), ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
