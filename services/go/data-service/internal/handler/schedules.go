package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/service"
)

func (h *Handler) HandleQueryRoutes(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "routes.query")
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

	routes, total, err := h.svc.QueryRoutes(ctx, service.QueryRoutesParams{
		DateFrom:           dateFrom,
		DateTo:             dateTo,
		StationExternalIds: stationExternalIds,
		FromCity:           r.URL.Query().Get("fromCity"),
		ToCity:             r.URL.Query().Get("toCity"),
		CarrierCodes:       parseCommaStrings(r.URL.Query().Get("carrierCodes")),
		CommercialCategory: r.URL.Query().Get("commercialCategory"),
		Name:               r.URL.Query().Get("name"),
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.writeError(w, span, http.StatusInternalServerError, "internal_error", "failed to query routes")
		return
	}

	h.writeJSON(w, span, http.StatusOK, model.RouteListResponse{
		Data:       routes,
		Pagination: model.Pagination{Total: total, Limit: limit, Offset: offset},
	})
}

func (h *Handler) HandleGetRouteById(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "route.get_by_id")
	defer span.End()

	routeID, err := strconv.ParseInt(chi.URLParam(r, "routeId"), 10, 64)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "routeId must be a valid integer")
		return
	}

	detail, err := h.svc.GetRouteById(ctx, routeID)
	if err != nil {
		h.handleNotFoundOrInternalError(w, span, err, "route")
		return
	}

	h.writeJSON(w, span, http.StatusOK, detail)
}

func (h *Handler) HandleGetRouteByKey(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "route.get_by_key")
	defer span.End()

	scheduleID, err := strconv.Atoi(chi.URLParam(r, "scheduleId"))
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "scheduleId must be a valid integer")
		return
	}
	orderID, err := strconv.Atoi(chi.URLParam(r, "orderId"))
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "orderId must be a valid integer")
		return
	}

	detail, err := h.svc.GetRouteByKey(ctx, scheduleID, orderID)
	if err != nil {
		h.handleNotFoundOrInternalError(w, span, err, "route")
		return
	}

	h.writeJSON(w, span, http.StatusOK, detail)
}

func (h *Handler) HandleGetRouteStations(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "route.stations")
	defer span.End()

	routeID, err := strconv.ParseInt(chi.URLParam(r, "routeId"), 10, 64)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "routeId must be a valid integer")
		return
	}

	stations, err := h.svc.GetRouteStations(ctx, routeID)
	if err != nil {
		h.handleNotFoundOrInternalError(w, span, err, "route")
		return
	}

	h.writeJSON(w, span, http.StatusOK, model.RouteStationListResponse{Data: stations})
}

func (h *Handler) HandleGetRouteOperatingDates(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "route.operating_dates")
	defer span.End()

	routeID, err := strconv.ParseInt(chi.URLParam(r, "routeId"), 10, 64)
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "routeId must be a valid integer")
		return
	}

	from, err := parseOptionalDate(r.URL.Query().Get("from"))
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "from: "+err.Error())
		return
	}
	to, err := parseOptionalDate(r.URL.Query().Get("to"))
	if err != nil {
		h.writeError(w, span, http.StatusBadRequest, "invalid_request", "to: "+err.Error())
		return
	}

	dates, err := h.svc.GetRouteOperatingDates(ctx, routeID, from, to)
	if err != nil {
		h.handleNotFoundOrInternalError(w, span, err, "route")
		return
	}

	dateStrs := make([]string, 0, len(dates))
	for _, d := range dates {
		dateStrs = append(dateStrs, d.Format("2006-01-02"))
	}

	h.writeJSON(w, span, http.StatusOK, model.OperatingDatesResponse{
		RouteID: routeID,
		Dates:   dateStrs,
	})
}
