package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/service"
)

// HandleQueryOperations queries train operations by filters and returns paginated results.
// @Summary		Query train operations
// @Description	Returns paginated train operations matching the given criteria
// @Tags		operations
// @Produce		json
// @Param		date query string false "Filter operations for a specific date (YYYY-MM-DD)"
// @Param		station query string false "Filter by station external ID"
// @Param		limit query int false "Limit (default 50, max 1000)" default(50)
// @Param		offset query int false "Offset for pagination (default 0)" default(0)
// @Success		200 {array} model.TrainOperation
// @Failure		400 {object} model.ErrorResponse "Bad request"
// @Router		/api/v1/operations [get]
func (h *Handler) HandleQueryOperations(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "operations.query")
	defer span.End()

	limit, offset, err := parseLimitOffset(r, 50, 1000)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	date, err := parseOptionalDate(r.URL.Query().Get("date"))
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "date: "+err.Error())
		return
	}
	if date == nil {
		today := time.Now()
		date = &today
	}
	stationExternalIds, err := parseCommaInts(r.URL.Query().Get("stationExternalIds"))
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "stationExternalIds: "+err.Error())
		return
	}

	var minDelay *int
	if rawMinDelay := r.URL.Query().Get("minDelay"); rawMinDelay != "" {
		v, err := strconv.Atoi(rawMinDelay)
		if err != nil || v < 0 {
			h.writeError(w, span, http.StatusBadRequest, "invalid_request", "minDelay must be a non-negative integer")
			return
		}
		minDelay = &v
	}

	operations, total, err := h.svc.QueryOperations(ctx, service.QueryOperationsParams{
		Date:               date,
		StationExternalIds: stationExternalIds,
		Status:             r.URL.Query().Get("status"),
		CarrierCodes:       parseCommaStrings(r.URL.Query().Get("carrierCodes")),
		MinDelay:           minDelay,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, span, http.StatusInternalServerError, "internal_error", "failed to query operations")
		return
	}

	h.writeJSON(w, span, http.StatusOK, model.OperationListResponse{
		Data:       operations,
		Pagination: model.Pagination{Total: total, Limit: limit, Offset: offset},
	})
}

// HandleGetOperationById retrieves a single operation by ID with full details including stops and delay info.
func (h *Handler) HandleGetOperationById(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "operation.get_by_id")
	defer span.End()

	operationID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "id must be a valid integer")
		return
	}

	detail, err := h.svc.GetOperationById(ctx, operationID)
	if err != nil {
		h.handleNotFoundOrInternalError(w, span, err, "operation")
		return
	}

	h.writeJSON(w, span, http.StatusOK, detail)
}

// HandleGetOperationStatistics retrieves aggregate statistics for train operations on a given date.
func (h *Handler) HandleGetOperationStatistics(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "operation.statistics")
	defer span.End()

	datePtr, err := parseOptionalDate(r.URL.Query().Get("date"))
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "date: "+err.Error())
		return
	}

	date := time.Now()
	if datePtr != nil {
		date = *datePtr
	}

	stats, err := h.svc.GetOperationStatistics(ctx, date)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, span, http.StatusInternalServerError, "internal_error", "failed to get operation statistics")
		return
	}

	h.writeJSON(w, span, http.StatusOK, stats)
}
