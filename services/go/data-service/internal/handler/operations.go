package handler

import (
	"fmt"
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
// @Param		activeOnly query bool false "Only status P operations whose expected end is not older than 30 minutes"
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

	activeOnly := false
	if rawActiveOnly := r.URL.Query().Get("activeOnly"); rawActiveOnly != "" {
		v, err := strconv.ParseBool(rawActiveOnly)
		if err != nil {
			h.writeError(w, span, http.StatusBadRequest, "invalid_request", "activeOnly must be a boolean")
			return
		}
		activeOnly = v
	}

	operations, total, err := h.svc.QueryOperations(ctx, service.QueryOperationsParams{
		Date:               date,
		StationExternalIds: stationExternalIds,
		Status:             r.URL.Query().Get("status"),
		CarrierCodes:       parseCommaStrings(r.URL.Query().Get("carrierCodes")),
		MinDelay:           minDelay,
		ActiveOnly:         activeOnly,
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

// HandleGetStationBoard returns the bucketed station board (at station, arrivals, departures).
// @Summary		Get station board (arrivals, departures, at station)
// @Description	Returns stops at the station bucketed into at_station, arrival and departure, each ordered by board time and truncated to limit
// @Tags		operations
// @Produce		json
// @Param		externalId path int true "Station external ID"
// @Param		at query string false "Reference instant (RFC 3339); defaults to the current time"
// @Param		limit query int false "Maximum number of entries per bucket (1-50)" default(10)
// @Param		horizonMinutes query int false "How far ahead of at to look for trains, in minutes (30-1440)" default(720)
// @Param		lookbackMinutes query int false "How far before at to read stops by planned time, in minutes (0-720)" default(360)
// @Success		200 {object} model.StationBoardResponse
// @Failure		400 {object} model.ErrorResponse "Bad request"
// @Failure		404 {object} model.ErrorResponse "Station not found"
// @Router		/api/v1/stations/{externalId}/board [get]
func (h *Handler) HandleGetStationBoard(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "station.board")
	defer span.End()

	externalID, err := strconv.Atoi(chi.URLParam(r, "externalId"))
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "externalId must be a valid integer")
		return
	}

	now, err := boardNow(r)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "at: "+err.Error())
		return
	}

	limit, err := parseIntInRange(r, "limit", 10, 1, 50)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	horizonMinutes, err := parseIntInRange(r, "horizonMinutes", 720, 30, 1440)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	lookbackMinutes, err := parseIntInRange(r, "lookbackMinutes", 360, 0, 720)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if _, err := h.svc.GetStationByExternalId(ctx, externalID); err != nil {
		h.handleNotFoundOrInternalError(w, span, err, "station")
		return
	}

	board, err := h.svc.GetStationBoard(ctx, externalID, now,
		time.Duration(horizonMinutes)*time.Minute, time.Duration(lookbackMinutes)*time.Minute, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, span, http.StatusInternalServerError, "internal_error", "failed to get station board")
		return
	}

	h.writeJSON(w, span, http.StatusOK, board)
}

// boardNow is the station board reference instant: the optional `at` query param (RFC 3339)
// or the current time. Stored stop times are true UTC instants, so no timezone shim is needed.
func boardNow(r *http.Request) (time.Time, error) {
	raw := r.URL.Query().Get("at")
	if raw == "" {
		return time.Now().UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date-time %q, expected RFC 3339", raw)
	}
	return t.UTC(), nil
}

// parseIntInRange parses an optional integer query param, rejecting values outside [lo, hi].
func parseIntInRange(r *http.Request, name string, def, lo, hi int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < lo || v > hi {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, lo, hi)
	}
	return v, nil
}
