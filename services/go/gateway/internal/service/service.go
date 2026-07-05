package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/pociag-do-predykcji/services/go/gateway/internal/client/dataservice"
	"github.com/pociag-do-predykcji/services/go/gateway/internal/mapper"
	"github.com/pociag-do-predykcji/services/go/gateway/internal/model"
	"github.com/pociag-do-predykcji/services/go/shared/trainutil"
)

type DataServiceClient interface {
	Ready(ctx context.Context) error
	QueryStations(ctx context.Context, search string, limit, offset int) (*dataservice.StationListResponse, error)
	GetStationByExternalID(ctx context.Context, externalID int) (*dataservice.Station, error)
	ListCarriers(ctx context.Context) (*dataservice.CarrierListResponse, error)

	QueryRoutes(ctx context.Context, p dataservice.QueryRoutesParams) (*dataservice.RouteListResponse, error)
	GetRouteByID(ctx context.Context, routeID int64) (*dataservice.RouteDetail, error)
	GetRouteByKey(ctx context.Context, scheduleID, orderID int) (*dataservice.RouteDetail, error)
	GetRouteStations(ctx context.Context, routeID int64) (*dataservice.RouteStationListResponse, error)
	GetRouteOperatingDates(ctx context.Context, routeID int64) (*dataservice.OperatingDatesResponse, error)

	QueryOperations(ctx context.Context, p dataservice.QueryOperationsParams) (*dataservice.OperationListResponse, error)
	GetOperationByID(ctx context.Context, operationID int64) (*dataservice.OperationDetail, error)
	GetOperationStatistics(ctx context.Context, date string) (*dataservice.OperationStatistics, error)

	QueryDisruptions(ctx context.Context, p dataservice.QueryDisruptionsParams) (*dataservice.DisruptionListResponse, error)
	GetDisruptionByID(ctx context.Context, disruptionID int64) (*dataservice.DisruptionDetail, error)
}

type Service struct {
	client DataServiceClient
	tracer trace.Tracer
}

func New(client DataServiceClient) *Service {
	return &Service{
		client: client,
		tracer: otel.Tracer("pociag.gateway"),
	}
}

func (s *Service) Ready(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "gateway.ready")
	defer span.End()

	if err := s.client.Ready(ctx); err != nil {
		return fmt.Errorf("ready check: %w", err)
	}
	return nil
}

func (s *Service) SearchStations(ctx context.Context, q string, limit int) (*model.StationSuggestionsResponse, error) {
	ctx, span := s.tracer.Start(ctx, "stations.search")
	defer span.End()

	stations, err := s.client.QueryStations(ctx, q, limit, 0)
	if err != nil {
		return nil, fmt.Errorf("search stations: %w", err)
	}

	suggestions := make([]model.StationSuggestion, 0, len(stations.Data))
	for _, st := range stations.Data {
		suggestions = append(suggestions, model.StationSuggestion{
			ExternalID: st.ExternalID,
			Name:       st.Name,
			City:       st.City,
		})
	}

	return &model.StationSuggestionsResponse{Suggestions: suggestions}, nil
}

func (s *Service) SearchSchedules(ctx context.Context, from, to, date string, carriers, categories []string, sortBy string, limit, offset int) (*model.ScheduleSearchResponse, error) {
	ctx, span := s.tracer.Start(ctx, "schedules.search")
	defer span.End()

	carrierMap, err := s.loadCarrierMap(ctx)
	if err != nil {
		return nil, err
	}

	dateFrom := date
	dateTo := date

	fromID, fromIsID := parseExternalID(from)
	toID, toIsID := parseExternalID(to)
	fromCity := ""
	if !fromIsID {
		fromCity = from
	}
	toCity := ""
	if !toIsID {
		toCity = to
	}

	baseParams := dataservice.QueryRoutesParams{
		DateFrom:     &dateFrom,
		DateTo:       &dateTo,
		FromCity:     fromCity,
		ToCity:       toCity,
		CarrierCodes: carriers,
		Limit:        1000,
		Offset:       0,
	}
	if fromIsID || toIsID {
		stationIDs := make([]int, 0, 2)
		if fromIsID {
			stationIDs = append(stationIDs, fromID)
		}
		if toIsID {
			stationIDs = append(stationIDs, toID)
		}
		baseParams.StationExternalIDs = stationIDs
	}

	queryCategories := categories
	if len(queryCategories) == 0 {
		queryCategories = []string{""}
	}

	merged := make(map[int64]dataservice.RouteSummary)
	for _, category := range queryCategories {
		params := baseParams
		params.CommercialCategory = category

		resp, err := s.client.QueryRoutes(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("search schedules query routes: %w", err)
		}
		for _, route := range resp.Data {
			merged[route.ID] = route
		}
	}

	routes := make([]dataservice.RouteSummary, 0, len(merged))
	for _, route := range merged {
		routes = append(routes, route)
	}

	constraints := routeSearchConstraints{
		FromIsID: fromIsID,
		FromID:   fromID,
		ToIsID:   toIsID,
		ToID:     toID,
		FromCity: fromCity,
		ToCity:   toCity,
	}

	if constraints.needsPostFilter() {
		filtered, err := s.filterRoutesByConstraints(ctx, routes, constraints)
		if err != nil {
			return nil, err
		}
		routes = filtered
	}

	results := make([]scheduleProjection, 0, len(routes))
	for _, route := range routes {
		result := mapRouteSummaryToSearchResult(route, carrierMap)
		projection := scheduleProjection{
			Result:        result,
			DepartureTime: result.Departure.Time,
			ArrivalTime:   result.Arrival.Time,
			Duration:      valueOrZero(result.DurationMinutes),
		}
		results = append(results, projection)
	}

	sort.SliceStable(results, func(i, j int) bool {
		switch sortBy {
		case "arrival":
			return results[i].ArrivalTime < results[j].ArrivalTime
		case "duration":
			return results[i].Duration < results[j].Duration
		default:
			return results[i].DepartureTime < results[j].DepartureTime
		}
	})

	total := len(results)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	paged := make([]model.ScheduleSearchResult, 0, end-start)
	for _, item := range results[start:end] {
		paged = append(paged, item.Result)
	}

	response := &model.ScheduleSearchResponse{
		Data: paged,
		Pagination: model.PaginationMeta{
			Total:   int64(total),
			Limit:   limit,
			Offset:  offset,
			HasMore: end < total,
		},
		Query: model.ScheduleSearchQuery{
			From: from,
			To:   to,
			Date: date,
		},
	}

	return response, nil
}

func (s *Service) GetScheduleDetail(ctx context.Context, routeID int64) (*model.ScheduleDetailView, error) {
	ctx, span := s.tracer.Start(ctx, "schedule.detail")
	defer span.End()

	route, err := s.client.GetRouteByID(ctx, routeID)
	if err != nil {
		return nil, fmt.Errorf("get schedule detail route: %w", err)
	}

	stationsResp, err := s.client.GetRouteStations(ctx, routeID)
	if err != nil {
		return nil, fmt.Errorf("get schedule detail stations: %w", err)
	}

	operatingDatesResp, err := s.client.GetRouteOperatingDates(ctx, routeID)
	if err != nil {
		return nil, fmt.Errorf("get schedule detail operating dates: %w", err)
	}

	carrierMap, err := s.loadCarrierMap(ctx)
	if err != nil {
		return nil, err
	}

	stationNames := make(map[int]string)
	stops := make([]model.ScheduleStopView, 0, len(stationsResp.Data))
	for _, st := range stationsResp.Data {
		name := ""
		if st.StationName != nil {
			name = *st.StationName
		} else {
			resolved, ok := stationNames[st.StationExternalID]
			if !ok {
				station, stationErr := s.client.GetStationByExternalID(ctx, st.StationExternalID)
				if stationErr == nil {
					resolved = station.Name
					stationNames[st.StationExternalID] = resolved
				}
			}
			name = resolved
		}
		extID := st.StationExternalID
		stops = append(stops, model.ScheduleStopView{
			StationName:       name,
			StationExternalID: &extID,
			Order:             st.OrderNumber,
			ArrivalTime:       st.ArrivalTime,
			DepartureTime:     st.DepartureTime,
			Platform:          st.Platform,
			StopType:          st.StopType,
		})
	}

	carrier := carrierInfo(route.CarrierCode, carrierMap)
	trainName := stringOrEmpty(route.Name)
	totalDuration := computeRouteDuration(stationsResp.Data)

	return &model.ScheduleDetailView{
		RouteID:              route.ID,
		TrainName:            trainName,
		Carrier:              carrier,
		CommercialCategory:   route.CommercialCategorySymbol,
		NationalNumber:       route.NationalNumber,
		Stops:                stops,
		OperatingDates:       operatingDatesResp.Dates,
		TotalDurationMinutes: totalDuration,
	}, nil
}

func (s *Service) GetLiveTrains(ctx context.Context, carriers []string, stationIDs []int, limit, offset int) (*model.LiveTrainsResponse, error) {
	ctx, span := s.tracer.Start(ctx, "trains.live")
	defer span.End()

	today := time.Now().UTC().Format("2006-01-02")
	operations, err := s.client.QueryOperations(ctx, dataservice.QueryOperationsParams{
		Date:               &today,
		StationExternalIDs: stationIDs,
		Status:             "P",
		CarrierCodes:       carriers,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, fmt.Errorf("query live operations: %w", err)
	}

	items := make([]model.LiveTrainSummary, 0, len(operations.Data))
	for _, op := range operations.Data {
		detail, err := s.client.GetOperationByID(ctx, op.ID)
		if err != nil {
			return nil, fmt.Errorf("enrich live operation %d: %w", op.ID, err)
		}

		current, next, delay, origin, destination := currentAndNextStations(detail.Stations)
		items = append(items, model.LiveTrainSummary{
			OperationID:    op.ID,
			TrainName:      stringOrEmpty(detail.RouteName),
			CarrierCode:    detail.CarrierCode,
			Status:         trainutil.StatusLabel(op.TrainStatus),
			StatusCode:     op.TrainStatus,
			CurrentStation: current,
			NextStation:    next,
			DelayMinutes:   delay,
			Origin:         origin,
			Destination:    destination,
		})
	}

	total := operations.Pagination.Total
	return &model.LiveTrainsResponse{
		Data: items,
		Pagination: model.PaginationMeta{
			Total:   total,
			Limit:   limit,
			Offset:  offset,
			HasMore: int64(offset+limit) < total,
		},
		GeneratedAt: time.Now().UTC(),
	}, nil
}

func (s *Service) GetTrainDetail(ctx context.Context, operationID int64) (*model.TrainDetailView, error) {
	ctx, span := s.tracer.Start(ctx, "train.detail")
	defer span.End()

	detail, err := s.client.GetOperationByID(ctx, operationID)
	if err != nil {
		return nil, fmt.Errorf("get train detail: %w", err)
	}

	carrierMap, err := s.loadCarrierMap(ctx)
	if err != nil {
		return nil, err
	}

	var carrier *model.TrainCarrier
	if detail.CarrierCode != nil {
		name := carrierMap[*detail.CarrierCode]
		code := *detail.CarrierCode
		carrier = &model.TrainCarrier{Code: &code}
		if name != "" {
			carrier.Name = &name
		}
	}

	stops := make([]model.TrainStopView, 0, len(detail.Stations))
	for _, st := range detail.Stations {
		plannedArrival := trainutil.FormatClock(st.PlannedArrival)
		plannedDeparture := trainutil.FormatClock(st.PlannedDeparture)
		actualArrival := trainutil.FormatClock(st.ActualArrival)
		actualDeparture := trainutil.FormatClock(st.ActualDeparture)
		extID := st.StationExternalID

		stops = append(stops, model.TrainStopView{
			StationName:           stringOrEmpty(st.StationName),
			StationExternalID:     &extID,
			Sequence:              st.ActualSequenceNumber,
			PlannedArrival:        plannedArrival,
			PlannedDeparture:      plannedDeparture,
			ActualArrival:         actualArrival,
			ActualDeparture:       actualDeparture,
			ArrivalDelayMinutes:   st.ArrivalDelayMinutes,
			DepartureDelayMinutes: st.DepartureDelayMinutes,
			IsConfirmed:           st.IsConfirmed,
			IsCancelled:           st.IsCancelled,
		})
	}

	return &model.TrainDetailView{
		OperationID:   detail.ID,
		TrainName:     stringOrEmpty(detail.RouteName),
		Carrier:       carrier,
		OperatingDate: detail.OperatingDate,
		Status:        trainutil.StatusLabel(detail.TrainStatus),
		StatusCode:    detail.TrainStatus,
		Stops:         stops,
	}, nil
}

func (s *Service) ListDisruptions(ctx context.Context, active bool, limit, offset int) (*model.DisruptionListView, error) {
	ctx, span := s.tracer.Start(ctx, "disruptions.list")
	defer span.End()

	var dateFrom *string
	var dateTo *string
	if active {
		today := time.Now().UTC().Format("2006-01-02")
		dateFrom = &today
		dateTo = &today
	}

	list, err := s.client.QueryDisruptions(ctx, dataservice.QueryDisruptionsParams{
		DateFrom: dateFrom,
		DateTo:   dateTo,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list disruptions: %w", err)
	}

	items := make([]model.DisruptionSummaryView, 0, len(list.Data))
	for _, d := range list.Data {
		affected := d.AffectedRoutesCount
		severity := trainutil.SeverityByAffectedRoutes(affected)
		items = append(items, model.DisruptionSummaryView{
			ID:                  d.ID,
			TypeName:            d.DisruptionTypeName,
			StartStation:        d.StartStationName,
			EndStation:          d.EndStationName,
			Message:             stringOrEmpty(d.Message),
			DateFrom:            d.DateFrom,
			DateTo:              d.DateTo,
			AffectedRoutesCount: &affected,
			Severity:            &severity,
		})
	}

	total := list.Pagination.Total
	return &model.DisruptionListView{
		Data: items,
		Pagination: model.PaginationMeta{
			Total:   total,
			Limit:   limit,
			Offset:  offset,
			HasMore: int64(offset+limit) < total,
		},
	}, nil
}

func (s *Service) GetDisruptionDetail(ctx context.Context, disruptionID int64) (*model.DisruptionDetailView, error) {
	ctx, span := s.tracer.Start(ctx, "disruption.detail")
	defer span.End()

	detail, err := s.client.GetDisruptionByID(ctx, disruptionID)
	if err != nil {
		return nil, fmt.Errorf("get disruption detail: %w", err)
	}

	routeCache := make(map[string]*dataservice.RouteDetail)
	affected := make([]model.DisruptionAffectedRouteView, 0, len(detail.AffectedRoutes))
	for _, route := range detail.AffectedRoutes {
		cacheKey := strconv.Itoa(route.ScheduleID) + ":" + strconv.Itoa(route.OrderID)
		cached, ok := routeCache[cacheKey]
		if !ok {
			resolved, resolveErr := s.client.GetRouteByKey(ctx, route.ScheduleID, route.OrderID)
			if resolveErr == nil {
				cached = resolved
				routeCache[cacheKey] = resolved
			}
		}

		trainName := ""
		var carrierCode *string
		if cached != nil {
			trainName = stringOrEmpty(cached.Name)
			carrierCode = cached.CarrierCode
		}
		operatingDate := ""
		operatingDate, err := normalizeDateString(route.OperatingDate)
		if err != nil {
			return nil, fmt.Errorf("get disruption detail affected route %d/%d operating date: %w", route.ScheduleID, route.OrderID, err)
		}

		affected = append(affected, model.DisruptionAffectedRouteView{
			TrainName:     trainName,
			CarrierCode:   carrierCode,
			OperatingDate: operatingDate,
			StationName:   route.StationName,
		})
	}

	return &model.DisruptionDetailView{
		ID:             detail.ID,
		TypeName:       detail.DisruptionTypeName,
		StartStation:   detail.StartStationName,
		EndStation:     detail.EndStationName,
		Message:        stringOrEmpty(detail.Message),
		DateFrom:       detail.DateFrom,
		DateTo:         detail.DateTo,
		AffectedRoutes: affected,
	}, nil
}

func (s *Service) GetDashboardOverview(ctx context.Context) (*model.DashboardOverview, error) {
	ctx, span := s.tracer.Start(ctx, "dashboard.overview")
	defer span.End()

	today := time.Now().UTC().Format("2006-01-02")
	stats, err := s.client.GetOperationStatistics(ctx, today)
	if err != nil {
		return nil, fmt.Errorf("get dashboard statistics: %w", err)
	}

	disruptions, err := s.client.QueryDisruptions(ctx, dataservice.QueryDisruptionsParams{
		DateFrom: &today,
		DateTo:   &today,
		Limit:    1,
		Offset:   0,
	})
	if err != nil {
		return nil, fmt.Errorf("get dashboard disruptions: %w", err)
	}

	inProgress := stats.ByStatus["P"]
	completed := stats.ByStatus["C"]
	cancelled := stats.ByStatus["X"] + stats.ByStatus["Q"]
	var onTimePct *float64
	if stats.Total > 0 {
		pct := (float64(stats.DelayDistribution.OnTime) / float64(stats.Total)) * 100
		onTimePct = &pct
	}

	return &model.DashboardOverview{
		Statistics: model.DashboardStatistics{
			Date:             stats.Date,
			TotalTrains:      stats.Total,
			InProgress:       &inProgress,
			Completed:        &completed,
			Cancelled:        &cancelled,
			AvgDelayMinutes:  stats.AvgDelayMinutes,
			OnTimePercentage: onTimePct,
		},
		DisruptionsActive: int(disruptions.Pagination.Total),
		DataFreshness:     model.DashboardDataFreshness{},
	}, nil
}

func (s *Service) loadCarrierMap(ctx context.Context) (map[string]string, error) {
	carriers, err := s.client.ListCarriers(ctx)
	if err != nil {
		return nil, fmt.Errorf("load carriers: %w", err)
	}

	carrierMap := make(map[string]string, len(carriers.Data))
	for _, c := range carriers.Data {
		carrierMap[c.Code] = c.Name
	}
	return carrierMap, nil
}

type routeSearchConstraints struct {
	FromIsID bool
	FromID   int
	ToIsID   bool
	ToID     int
	FromCity string
	ToCity   string
}

func (c routeSearchConstraints) needsPostFilter() bool {
	return c.FromIsID || c.ToIsID
}

func (s *Service) filterRoutesByConstraints(ctx context.Context, routes []dataservice.RouteSummary, constraints routeSearchConstraints) ([]dataservice.RouteSummary, error) {
	filtered := make([]dataservice.RouteSummary, 0, len(routes))
	stationCache := make(map[int]*dataservice.Station)

	for _, route := range routes {
		stationsResp, err := s.client.GetRouteStations(ctx, route.ID)
		if err != nil {
			if errors.Is(err, dataservice.ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("filter route constraints for route %d: %w", route.ID, err)
		}

		if !matchesStationOrder(stationsResp.Data, constraints.FromIsID, constraints.FromID, constraints.ToIsID, constraints.ToID) {
			continue
		}

		if constraints.FromCity != "" {
			matches, err := s.matchesEndpointCity(ctx, stationsResp.Data, true, constraints.FromCity, stationCache)
			if err != nil {
				return nil, fmt.Errorf("filter route from city for route %d: %w", route.ID, err)
			}
			if !matches {
				continue
			}
		}

		if constraints.ToCity != "" {
			matches, err := s.matchesEndpointCity(ctx, stationsResp.Data, false, constraints.ToCity, stationCache)
			if err != nil {
				return nil, fmt.Errorf("filter route to city for route %d: %w", route.ID, err)
			}
			if !matches {
				continue
			}
		}

		filtered = append(filtered, route)
	}

	return filtered, nil
}

func (s *Service) matchesEndpointCity(ctx context.Context, stations []dataservice.RouteStation, fromEndpoint bool, city string, cache map[int]*dataservice.Station) (bool, error) {
	endpoint, ok := endpointStation(stations, fromEndpoint)
	if !ok {
		return false, nil
	}

	resolved, err := s.resolveStation(ctx, endpoint.StationExternalID, cache)
	if err != nil {
		if errors.Is(err, dataservice.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("resolve station %d: %w", endpoint.StationExternalID, err)
	}

	return equalFoldTrim(valueOrEmpty(resolved.City), city), nil
}

func (s *Service) resolveStation(ctx context.Context, externalID int, cache map[int]*dataservice.Station) (*dataservice.Station, error) {
	if cached, ok := cache[externalID]; ok {
		return cached, nil
	}

	station, err := s.client.GetStationByExternalID(ctx, externalID)
	if err != nil {
		return nil, err
	}

	cache[externalID] = station
	return station, nil
}

func endpointStation(stations []dataservice.RouteStation, fromEndpoint bool) (dataservice.RouteStation, bool) {
	if len(stations) == 0 {
		return dataservice.RouteStation{}, false
	}

	selected := stations[0]
	for _, st := range stations[1:] {
		if fromEndpoint {
			if st.OrderNumber < selected.OrderNumber {
				selected = st
			}
			continue
		}
		if st.OrderNumber > selected.OrderNumber {
			selected = st
		}
	}
	return selected, true
}

func normalizeDateString(raw *string) (string, error) {
	if raw == nil {
		return "", fmt.Errorf("missing required date")
	}

	value := strings.TrimSpace(*raw)
	if value == "" {
		return "", fmt.Errorf("missing required date")
	}

	if _, err := time.Parse("2006-01-02", value); err != nil {
		return "", fmt.Errorf("invalid date format: %w", err)
	}

	return value, nil
}

func matchesStationOrder(stations []dataservice.RouteStation, fromIsID bool, fromID int, toIsID bool, toID int) bool {
	if !fromIsID && !toIsID {
		return true
	}

	fromOrder := -1
	toOrder := -1
	for _, st := range stations {
		if fromIsID && st.StationExternalID == fromID {
			fromOrder = st.OrderNumber
		}
		if toIsID && st.StationExternalID == toID {
			toOrder = st.OrderNumber
		}
	}

	if fromIsID && fromOrder == -1 {
		return false
	}
	if toIsID && toOrder == -1 {
		return false
	}
	if fromIsID && toIsID {
		return fromOrder < toOrder
	}
	return true
}

func mapRouteSummaryToSearchResult(route dataservice.RouteSummary, carrierMap map[string]string) model.ScheduleSearchResult {
	carrier := carrierInfo(route.CarrierCode, carrierMap)
	departureStationName := stringOrEmpty(route.FirstStationName)
	arrivalStationName := stringOrEmpty(route.LastStationName)
	departureTime := stringOrEmpty(route.FirstDepartureTime)
	arrivalTime := stringOrEmpty(route.LastArrivalTime)
	duration := mapper.DurationMinutesFromClock(route.FirstDepartureTime, route.LastArrivalTime, 0, 0)

	stops := route.StationCount - 2
	if stops < 0 {
		stops = 0
	}

	return model.ScheduleSearchResult{
		RouteID:            route.ID,
		TrainName:          stringOrEmpty(route.Name),
		Carrier:            carrier,
		CommercialCategory: route.CommercialCategorySymbol,
		Departure: model.ScheduleEndpoint{
			StationName: departureStationName,
			Time:        departureTime,
		},
		Arrival: model.ScheduleEndpoint{
			StationName: arrivalStationName,
			Time:        arrivalTime,
		},
		DurationMinutes: duration,
		StopsCount:      &stops,
	}
}

func carrierInfo(carrierCode *string, carrierMap map[string]string) model.CarrierInfo {
	if carrierCode == nil {
		return model.CarrierInfo{}
	}
	return model.CarrierInfo{Code: *carrierCode, Name: carrierMap[*carrierCode]}
}

func parseExternalID(raw string) (int, bool) {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

func computeRouteDuration(stations []dataservice.RouteStation) *int {
	if len(stations) == 0 {
		return nil
	}
	first := stations[0]
	last := stations[len(stations)-1]

	departureDay := 0
	if first.DepartureDay != nil {
		departureDay = *first.DepartureDay
	}
	arrivalDay := departureDay
	if last.ArrivalDay != nil {
		arrivalDay = *last.ArrivalDay
	}

	return mapper.DurationMinutesFromClock(first.DepartureTime, last.ArrivalTime, departureDay, arrivalDay)
}

func currentAndNextStations(stations []dataservice.OperationStation) (*string, *string, *int, *string, *string) {
	if len(stations) == 0 {
		return nil, nil, nil, nil, nil
	}

	sorted := make([]dataservice.OperationStation, len(stations))
	copy(sorted, stations)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].ActualSequenceNumber < sorted[j].ActualSequenceNumber
	})

	origin := sorted[0].StationName
	destination := sorted[len(sorted)-1].StationName

	var current *string
	var next *string
	var delay *int
	currentIndex := -1
	for idx, st := range sorted {
		if st.IsConfirmed && !st.IsCancelled {
			current = st.StationName
			currentIndex = idx
			if st.DepartureDelayMinutes != nil {
				delay = st.DepartureDelayMinutes
			} else if st.ArrivalDelayMinutes != nil {
				delay = st.ArrivalDelayMinutes
			}
		}
	}

	if currentIndex >= 0 {
		for i := currentIndex + 1; i < len(sorted); i++ {
			if !sorted[i].IsCancelled {
				next = sorted[i].StationName
				break
			}
		}
	}

	return current, next, delay, origin, destination
}

type scheduleProjection struct {
	Result        model.ScheduleSearchResult
	DepartureTime string
	ArrivalTime   string
	Duration      int
}

func valueOrZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func stringOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func equalFoldTrim(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
