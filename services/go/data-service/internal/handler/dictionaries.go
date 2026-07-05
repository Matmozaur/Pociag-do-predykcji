package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/service"
)

func (h *Handler) HandleQueryStations(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "stations.query")
	defer span.End()

	limit, offset, err := parseLimitOffset(r, 50, 1000)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	externalIds, err := parseCommaInts(r.URL.Query().Get("externalIds"))
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "externalIds: "+err.Error())
		return
	}

	stations, total, err := h.svc.QueryStations(ctx, service.QueryStationsParams{
		Search:      r.URL.Query().Get("search"),
		City:        r.URL.Query().Get("city"),
		ExternalIds: externalIds,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, span, http.StatusInternalServerError, "internal_error", "failed to query stations")
		return
	}

	h.writeJSON(w, span, http.StatusOK, model.StationListResponse{
		Data:       stations,
		Pagination: model.Pagination{Total: total, Limit: limit, Offset: offset},
	})
}

func (h *Handler) HandleGetStationByExternalId(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "station.get_by_external_id")
	defer span.End()

	externalID, err := strconv.Atoi(chi.URLParam(r, "externalId"))
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "externalId must be a valid integer")
		return
	}

	station, err := h.svc.GetStationByExternalId(ctx, externalID)
	if err != nil {
		h.handleNotFoundOrInternalError(w, span, err, "station")
		return
	}

	h.writeJSON(w, span, http.StatusOK, station)
}

func (h *Handler) HandleListCarriers(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "carriers.list")
	defer span.End()

	carriers, err := h.svc.ListCarriers(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, span, http.StatusInternalServerError, "internal_error", "failed to list carriers")
		return
	}

	h.writeJSON(w, span, http.StatusOK, model.CarrierListResponse{Data: carriers})
}

func (h *Handler) HandleListCommercialCategories(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "commercial_categories.list")
	defer span.End()

	categories, err := h.svc.ListCommercialCategories(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, span, http.StatusInternalServerError, "internal_error", "failed to list commercial categories")
		return
	}

	h.writeJSON(w, span, http.StatusOK, model.CommercialCategoryListResponse{Data: categories})
}

func (h *Handler) HandleListStopTypes(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "stop_types.list")
	defer span.End()

	stopTypes, err := h.svc.ListStopTypes(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, span, http.StatusInternalServerError, "internal_error", "failed to list stop types")
		return
	}

	h.writeJSON(w, span, http.StatusOK, model.StopTypeListResponse{Data: stopTypes})
}
