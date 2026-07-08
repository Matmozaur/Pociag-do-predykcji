package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/service"
)

// HandleQueryDisruptions queries traffic disruptions by filters and returns paginated results.
// @Summary		Query disruptions
// @Description	Returns paginated traffic disruptions matching the given criteria
// @Tags		disruptions
// @Produce		json
// @Param		dateFrom query string false "Filter disruptions from date (YYYY-MM-DD)"
// @Param		dateTo query string false "Filter disruptions until date (YYYY-MM-DD)"
// @Param		limit query int false "Limit (default 50, max 1000)" default(50)
// @Param		offset query int false "Offset for pagination (default 0)" default(0)
// @Success		200 {array} model.Disruption
// @Failure		400 {object} model.ErrorResponse "Bad request"
// @Router		/api/v1/disruptions [get]
func (h *Handler) HandleQueryDisruptions(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "disruptions.query")
	defer span.End()

	limit, offset, err := parseLimitOffset(r, 50, 1000)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	dateFrom, err := parseOptionalDate(r.URL.Query().Get("dateFrom"))
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "dateFrom: "+err.Error())
		return
	}
	dateTo, err := parseOptionalDate(r.URL.Query().Get("dateTo"))
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "dateTo: "+err.Error())
		return
	}
	stationExternalIds, err := parseCommaInts(r.URL.Query().Get("stationExternalIds"))
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "stationExternalIds: "+err.Error())
		return
	}

	disruptions, total, err := h.svc.QueryDisruptions(ctx, service.QueryDisruptionsParams{
		DateFrom:           dateFrom,
		DateTo:             dateTo,
		StationExternalIds: stationExternalIds,
		TypeCode:           r.URL.Query().Get("typeCode"),
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, span, http.StatusInternalServerError, "internal_error", "failed to query disruptions")
		return
	}

	h.writeJSON(w, span, http.StatusOK, model.DisruptionListResponse{
		Data:       disruptions,
		Pagination: model.Pagination{Total: total, Limit: limit, Offset: offset},
	})
}

// HandleGetDisruptionById retrieves a single disruption by ID with full details and affected routes.
func (h *Handler) HandleGetDisruptionById(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "disruption.get_by_id")
	defer span.End()

	disruptionID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "id must be a valid integer")
		return
	}

	detail, err := h.svc.GetDisruptionById(ctx, disruptionID)
	if err != nil {
		h.handleNotFoundOrInternalError(w, span, err, "disruption")
		return
	}

	h.writeJSON(w, span, http.StatusOK, detail)
}
