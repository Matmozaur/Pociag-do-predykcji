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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/repository"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/service"
)

// Handler handles all HTTP endpoints for the data-service.
// RegisterRoutes registers all chi routes. See specs/openapi/data-service.yml for full API contract.
type Handler struct {
	svc    *service.Service
	tracer trace.Tracer
}

func New(svc *service.Service) *Handler {
	return &Handler{
		svc:    svc,
		tracer: otel.Tracer("pociag.data-service"),
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/healthz", h.HandleHealthz)
	r.Get("/readyz", h.HandleReadyz)

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/schedules", func(r chi.Router) {
			r.Get("/", h.HandleQueryRoutes)
			// Static path before parametric.
			r.Get("/by-key/{scheduleId}/{orderId}", h.HandleGetRouteByKey)
			r.Get("/{routeId}", h.HandleGetRouteById)
			r.Get("/{routeId}/stations", h.HandleGetRouteStations)
			r.Get("/{routeId}/operating-dates", h.HandleGetRouteOperatingDates)
		})
		r.Route("/operations", func(r chi.Router) {
			r.Get("/", h.HandleQueryOperations)
			// Static path before parametric.
			r.Get("/statistics", h.HandleGetOperationStatistics)
			r.Get("/{id}", h.HandleGetOperationById)
		})
		r.Route("/disruptions", func(r chi.Router) {
			r.Get("/", h.HandleQueryDisruptions)
			r.Get("/{id}", h.HandleGetDisruptionById)
		})
		r.Get("/stations", h.HandleQueryStations)
		r.Get("/stations/{externalId}", h.HandleGetStationByExternalId)
		r.Get("/carriers", h.HandleListCarriers)
		r.Get("/commercial-categories", h.HandleListCommercialCategories)
		r.Get("/stop-types", h.HandleListStopTypes)
	})
}

// HandleHealthz handles the liveness probe endpoint.
// @Summary		Liveness probe
// @Description	Service is alive
// @Tags		health
// @Success		200 "OK"
// @Router		/healthz [get]
func (h *Handler) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	_, span := h.tracer.Start(r.Context(), "health.check")
	defer span.End()

	w.WriteHeader(http.StatusOK)
}

// HandleReadyz handles the readiness probe endpoint.
// @Summary		Readiness probe
// @Description	Service is ready to accept traffic
// @Tags		health
// @Success		200 "OK"
// @Failure		503 "Service is not ready"
// @Router		/readyz [get]
func (h *Handler) HandleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "readiness.check")
	defer span.End()

	if err := h.svc.Ready(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, span, http.StatusServiceUnavailable, "not_ready", "service is not ready")
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (h *Handler) writeJSON(w http.ResponseWriter, span trace.Span, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

func (h *Handler) writeError(w http.ResponseWriter, span trace.Span, status int, errCode, msg string) {
	traceID := span.SpanContext().TraceID().String()
	h.writeJSON(w, span, status, struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		TraceID string `json:"trace_id,omitempty"`
	}{Error: errCode, Message: msg, TraceID: traceID})
}

func (h *Handler) handleNotFoundOrInternalError(w http.ResponseWriter, span trace.Span, err error, resource string) {
	span.RecordError(err)
	if isNotFound(err) {
		h.writeError(w, span, http.StatusNotFound, "not_found", resource+" not found")
		return
	}
	span.SetStatus(codes.Error, err.Error())
	h.writeError(w, span, http.StatusInternalServerError, "internal_error", fmt.Sprintf("failed to fetch %s", resource))
}

func isNotFound(err error) bool {
	return errors.Is(err, repository.ErrNotFound)
}

func parseLimitOffset(r *http.Request, defaultLimit, maxLimit int) (int, int, error) {
	limit := defaultLimit
	offset := 0

	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		l, err := strconv.Atoi(rawLimit)
		if err != nil || l < 1 {
			return 0, 0, fmt.Errorf("limit must be a positive integer")
		}
		if l > maxLimit {
			l = maxLimit
		}
		limit = l
	}

	if rawOffset := r.URL.Query().Get("offset"); rawOffset != "" {
		o, err := strconv.Atoi(rawOffset)
		if err != nil || o < 0 {
			return 0, 0, fmt.Errorf("offset must be a non-negative integer")
		}
		offset = o
	}

	return limit, offset, nil
}

func parseCommaInts(s string) ([]int, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	result := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q", p)
		}
		result = append(result, n)
	}
	return result, nil
}

func parseCommaStrings(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func parseOptionalDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, fmt.Errorf("invalid date %q, expected YYYY-MM-DD", s)
	}
	return &t, nil
}
