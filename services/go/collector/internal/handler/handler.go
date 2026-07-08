package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/pociag-do-predykcji/services/go/collector/internal/service"
)

// Handler handles all HTTP endpoints for the collector service.
// RegisterRoutes registers all chi routes. See specs/openapi/collector.yml for full API contract.
type Handler struct {
	svc    *service.Service
	tracer trace.Tracer
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	TraceID string `json:"trace_id,omitempty"`
}

type fetchSchedulesRequest struct {
	DateFrom string `json:"date_from"`
	DateTo   string `json:"date_to"`
	Force    bool   `json:"force"`
}

type fetchOperationsRequest struct {
	Date  string `json:"date"`
	Force bool   `json:"force"`
}

type fetchDisruptionsRequest struct {
	DateFrom string `json:"date_from"`
	DateTo   string `json:"date_to"`
	Force    bool   `json:"force"`
}

type ingestionStatusResponse struct {
	Runs []service.IngestionRun `json:"runs"`
}

var allowedPipelines = map[string]struct{}{
	"schedules":    {},
	"operations":   {},
	"disruptions":  {},
	"dictionaries": {},
}

func New(svc *service.Service) *Handler {
	return &Handler{
		svc:    svc,
		tracer: otel.Tracer("pociag.collector"),
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/healthz", h.HandleHealthz)
	r.Get("/readyz", h.HandleReadyz)
	r.Post("/api/v1/fetch/dictionaries", h.HandleFetchDictionaries)
	r.Post("/api/v1/fetch/schedules", h.HandleFetchSchedules)
	r.Post("/api/v1/fetch/operations", h.HandleFetchOperations)
	r.Post("/api/v1/fetch/disruptions", h.HandleFetchDisruptions)
	r.Get("/api/v1/fetch/status", h.HandleFetchStatus)
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

// HandleFetchDictionaries fetches all dictionaries from PLK API into raw landing.
// @Summary		Fetch all dictionaries from PLK API
// @Description	Fetches all PLK dictionaries and lands raw payloads into data lake
// @Tags		fetch
// @Success		200 {object} service.FetchResult
// @Failure		502 {object} errorResponse "Upstream PLK API error"
// @Router		/api/v1/fetch/dictionaries [post]
func (h *Handler) HandleFetchDictionaries(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "dictionaries.fetch")
	defer span.End()

	result, err := h.svc.FetchDictionaries(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, span, http.StatusBadGateway, "upstream_error", "failed to fetch dictionaries")
		return
	}

	h.writeJSON(w, span, http.StatusOK, result)
}

// HandleFetchSchedules fetches schedules from PLK API for a date range into raw landing.
// Pagination is handled internally.
// @Summary		Fetch schedules from PLK API
// @Description	Pulls schedule data for a date range and lands raw payloads. Pagination is handled internally.
// @Tags		fetch
// @Accept		json
// @Produce		json
// @Param		body body fetchSchedulesRequest true "Date range and force flag"
// @Success		200 {object} service.FetchResult
// @Failure		400 {object} errorResponse "Bad request"
// @Failure		409 {object} errorResponse "Fetch already running for this date range"
// @Failure		502 {object} errorResponse "Upstream PLK API error"
// @Router		/api/v1/fetch/schedules [post]
func (h *Handler) HandleFetchSchedules(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "schedules.fetch")
	defer span.End()

	var req fetchSchedulesRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	dateFrom, err := parseDate(req.DateFrom)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "date_from must be in YYYY-MM-DD format")
		return
	}
	dateTo, err := parseDate(req.DateTo)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "date_to must be in YYYY-MM-DD format")
		return
	}

	result, err := h.svc.FetchSchedules(ctx, service.FetchSchedulesRequest{
		DateFrom: dateFrom,
		DateTo:   dateTo,
		Force:    req.Force,
	})
	if err != nil {
		h.handleFetchError(w, span, err, "schedules")
		return
	}

	h.writeJSON(w, span, http.StatusOK, result)
}

// HandleFetchOperations fetches operations from PLK API for a specific date into raw landing.
// @Summary		Fetch train operations from PLK API
// @Description	Pulls operations for a date and lands raw payloads. Includes planned/actual timing data and handles pagination internally.
// @Tags		fetch
// @Accept		json
// @Produce		json
// @Param		body body fetchOperationsRequest true "Date and force flag"
// @Success		200 {object} service.FetchResult
// @Failure		400 {object} errorResponse "Bad request"
// @Failure		409 {object} errorResponse "Fetch already running for this date"
// @Failure		502 {object} errorResponse "Upstream PLK API error"
// @Router		/api/v1/fetch/operations [post]
func (h *Handler) HandleFetchOperations(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "operations.fetch")
	defer span.End()

	var req fetchOperationsRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	date, err := parseDate(req.Date)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "date must be in YYYY-MM-DD format")
		return
	}

	result, err := h.svc.FetchOperations(ctx, service.FetchOperationsRequest{
		Date:  date,
		Force: req.Force,
	})
	if err != nil {
		h.handleFetchError(w, span, err, "operations")
		return
	}

	h.writeJSON(w, span, http.StatusOK, result)
}

// HandleFetchDisruptions fetches disruptions from PLK API for a date range into raw landing.
// @Summary		Fetch disruptions from PLK API
// @Description	Pulls disruption data for a date range and lands raw payloads
// @Tags		fetch
// @Accept		json
// @Produce		json
// @Param		body body fetchDisruptionsRequest true "Date range and force flag"
// @Success		200 {object} service.FetchResult
// @Failure		400 {object} errorResponse "Bad request"
// @Failure		409 {object} errorResponse "Fetch already running for this date range"
// @Failure		502 {object} errorResponse "Upstream PLK API error"
// @Router		/api/v1/fetch/disruptions [post]
func (h *Handler) HandleFetchDisruptions(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "disruptions.fetch")
	defer span.End()

	var req fetchDisruptionsRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	dateFrom, err := parseDate(req.DateFrom)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "date_from must be in YYYY-MM-DD format")
		return
	}
	dateTo, err := parseDate(req.DateTo)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "date_to must be in YYYY-MM-DD format")
		return
	}

	result, err := h.svc.FetchDisruptions(ctx, service.FetchDisruptionsRequest{
		DateFrom: dateFrom,
		DateTo:   dateTo,
		Force:    req.Force,
	})
	if err != nil {
		h.handleFetchError(w, span, err, "disruptions")
		return
	}

	h.writeJSON(w, span, http.StatusOK, result)
}

// HandleFetchStatus retrieves the status of recent ingestion runs.
// @Summary		Get ingestion run status
// @Description	Returns the status of recent data ingestion runs
// @Tags		status
// @Produce		json
// @Success		200 {object} ingestionStatusResponse
// @Router		/api/v1/fetch/status [get]
func (h *Handler) HandleFetchStatus(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "status.fetch")
	defer span.End()

	limit := 10
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			h.writeError(w, span, http.StatusBadRequest, "invalid_request", "limit must be an integer")
			return
		}
		limit = parsed
	}

	if limit > 100 {
		limit = 100
	}

	var pipeline *string
	if rawPipeline := r.URL.Query().Get("pipeline"); rawPipeline != "" {
		if _, ok := allowedPipelines[rawPipeline]; !ok {
			h.writeError(w, span, http.StatusBadRequest, "invalid_request", "pipeline must be one of: schedules, operations, disruptions, dictionaries")
			return
		}
		pipeline = &rawPipeline
	}

	runs, err := h.svc.GetFetchStatus(ctx, pipeline, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, span, http.StatusBadGateway, "upstream_error", "failed to fetch status")
		return
	}

	h.writeJSON(w, span, http.StatusOK, ingestionStatusResponse{Runs: runs})
}

func (h *Handler) handleFetchError(w http.ResponseWriter, span trace.Span, err error, pipeline string) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())

	if errors.Is(err, service.ErrPipelineRunning) {
		h.writeError(w, span, http.StatusConflict, "already_running", fmt.Sprintf("%s fetch already running", pipeline))
		return
	}

	h.writeError(w, span, http.StatusBadGateway, "upstream_error", fmt.Sprintf("failed to fetch %s", pipeline))
}

func (h *Handler) writeError(w http.ResponseWriter, span trace.Span, status int, code string, message string) {
	traceID := span.SpanContext().TraceID().String()
	h.writeJSON(w, span, status, errorResponse{Error: code, Message: message, TraceID: traceID})
}

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

func decodeJSONStrict(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode JSON body: %w", err)
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode JSON body: trailing data")
	}

	return nil
}

func parseDate(value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse date: %w", err)
	}
	return parsed, nil
}
