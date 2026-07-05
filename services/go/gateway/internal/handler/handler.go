package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/pociag-do-predykcji/services/go/gateway/internal/client/dataservice"
	"github.com/pociag-do-predykcji/services/go/gateway/internal/model"
	"github.com/pociag-do-predykcji/services/go/gateway/internal/service"
	"github.com/pociag-do-predykcji/services/go/shared/trainutil"
)

type Handler struct {
	svc    *service.Service
	tracer trace.Tracer
}

func New(svc *service.Service) *Handler {
	return &Handler{
		svc:    svc,
		tracer: otel.Tracer("pociag.gateway"),
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/healthz", h.HandleHealthz)
	r.Get("/readyz", h.HandleReadyz)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/search/stations", h.HandleSearchStations)
		r.Get("/schedules/search", h.HandleSearchSchedules)
		r.Get("/schedules/{routeId}", h.HandleGetScheduleDetail)
		r.Get("/trains/live", h.HandleGetLiveTrains)
		r.Get("/trains/{operationId}", h.HandleGetTrainDetail)
		r.Get("/disruptions", h.HandleListDisruptions)
		r.Get("/disruptions/{disruptionId}", h.HandleGetDisruptionDetail)
		r.Get("/dashboard/overview", h.HandleGetDashboardOverview)
	})
}

func (h *Handler) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	_, span := h.tracer.Start(r.Context(), "health.check")
	defer span.End()

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) HandleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "ready.check")
	defer span.End()

	if err := h.svc.Ready(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, http.StatusServiceUnavailable, "not_ready", "service is not ready")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) HandleSearchStations(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "stations.search")
	defer span.End()

	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "q must have at least 2 characters")
		return
	}

	limit, err := parseLimit(r, 10, 20)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	response, err := h.svc.SearchStations(ctx, q, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, http.StatusInternalServerError, "internal_error", "failed to search stations")
		return
	}

	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) HandleSearchSchedules(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "schedules.search")
	defer span.End()

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	date := r.URL.Query().Get("date")
	if from == "" || to == "" || date == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "from, to and date are required")
		return
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "date must be in YYYY-MM-DD format")
		return
	}

	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "departure"
	}
	if sortBy != "departure" && sortBy != "arrival" && sortBy != "duration" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "sort must be one of: departure, arrival, duration")
		return
	}

	limit, offset, err := parseLimitOffset(r, 20, 100)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	response, err := h.svc.SearchSchedules(
		ctx,
		from,
		to,
		date,
		trainutil.ParseCSV(r.URL.Query().Get("carriers")),
		trainutil.ParseCSV(r.URL.Query().Get("categories")),
		sortBy,
		limit,
		offset,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, http.StatusInternalServerError, "internal_error", "failed to search schedules")
		return
	}

	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) HandleGetScheduleDetail(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "schedule.detail")
	defer span.End()

	routeID, err := strconv.ParseInt(chi.URLParam(r, "routeId"), 10, 64)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "routeId must be a valid integer")
		return
	}

	response, err := h.svc.GetScheduleDetail(ctx, routeID)
	if err != nil {
		h.handleDataServiceError(w, span, err, "failed to fetch schedule detail")
		return
	}

	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) HandleGetLiveTrains(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "trains.live")
	defer span.End()

	limit, offset, err := parseLimitOffset(r, 20, 100)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	stationIDs, err := parseCSVInts(r.URL.Query().Get("stations"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "stations must be comma-separated integers")
		return
	}

	response, err := h.svc.GetLiveTrains(ctx, trainutil.ParseCSV(r.URL.Query().Get("carriers")), stationIDs, limit, offset)
	if err != nil {
		h.handleDataServiceError(w, span, err, "failed to fetch live trains")
		return
	}

	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) HandleGetTrainDetail(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "train.detail")
	defer span.End()

	operationID, err := strconv.ParseInt(chi.URLParam(r, "operationId"), 10, 64)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "operationId must be a valid integer")
		return
	}

	response, err := h.svc.GetTrainDetail(ctx, operationID)
	if err != nil {
		h.handleDataServiceError(w, span, err, "failed to fetch train detail")
		return
	}

	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) HandleListDisruptions(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "disruptions.list")
	defer span.End()

	limit, offset, err := parseLimitOffset(r, 20, 100)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	active := true
	if raw := r.URL.Query().Get("active"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid_request", "active must be a boolean")
			return
		}
		active = parsed
	}

	response, err := h.svc.ListDisruptions(ctx, active, limit, offset)
	if err != nil {
		h.handleDataServiceError(w, span, err, "failed to list disruptions")
		return
	}

	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) HandleGetDisruptionDetail(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "disruption.detail")
	defer span.End()

	disruptionID, err := strconv.ParseInt(chi.URLParam(r, "disruptionId"), 10, 64)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "disruptionId must be a valid integer")
		return
	}

	response, err := h.svc.GetDisruptionDetail(ctx, disruptionID)
	if err != nil {
		h.handleDataServiceError(w, span, err, "failed to fetch disruption detail")
		return
	}

	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) HandleGetDashboardOverview(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "dashboard.overview")
	defer span.End()

	response, err := h.svc.GetDashboardOverview(ctx)
	if err != nil {
		h.handleDataServiceError(w, span, err, "failed to fetch dashboard overview")
		return
	}

	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(w, status, model.ErrorResponse{Error: code, Message: message})
}

func (h *Handler) handleDataServiceError(w http.ResponseWriter, span trace.Span, err error, message string) {
	span.RecordError(err)
	if errors.Is(err, dataservice.ErrNotFound) {
		h.writeError(w, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	span.SetStatus(codes.Error, err.Error())
	h.writeError(w, http.StatusInternalServerError, "internal_error", message)
}

func parseLimit(r *http.Request, defaultValue, maxValue int) (int, error) {
	limit := defaultValue
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			return 0, fmt.Errorf("limit must be a positive integer")
		}
		if parsed > maxValue {
			parsed = maxValue
		}
		limit = parsed
	}
	return limit, nil
}

func parseLimitOffset(r *http.Request, defaultLimit, maxLimit int) (int, int, error) {
	limit, err := parseLimit(r, defaultLimit, maxLimit)
	if err != nil {
		return 0, 0, err
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			return 0, 0, fmt.Errorf("offset must be a non-negative integer")
		}
		offset = parsed
	}
	return limit, offset, nil
}

func parseCSVInts(raw string) ([]int, error) {
	if raw == "" {
		return nil, nil
	}
	items := trainutil.ParseCSV(raw)
	result := make([]int, 0, len(items))
	for _, item := range items {
		value, err := strconv.Atoi(item)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}
