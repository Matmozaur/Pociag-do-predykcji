package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/service"
)

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
