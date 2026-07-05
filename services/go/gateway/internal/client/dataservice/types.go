package dataservice

import "github.com/pociag-do-predykcji/services/go/shared/dsmodel"

// Domain wire types -- single source of truth in shared module
type Pagination = dsmodel.Pagination
type Station = dsmodel.Station
type StationListResponse = dsmodel.StationListResponse
type Carrier = dsmodel.Carrier
type CarrierListResponse = dsmodel.CarrierListResponse
type RouteSummary = dsmodel.RouteSummary
type RouteListResponse = dsmodel.RouteListResponse
type RouteStation = dsmodel.RouteStation
type RouteStationListResponse = dsmodel.RouteStationListResponse
type OperatingDatesResponse = dsmodel.OperatingDatesResponse
type RouteDetail = dsmodel.RouteDetail
type OperationSummary = dsmodel.OperationSummary
type OperationListResponse = dsmodel.OperationListResponse
type OperationStation = dsmodel.OperationStation
type OperationDetail = dsmodel.OperationDetail
type DelayDistribution = dsmodel.DelayDistribution
type OperationStatistics = dsmodel.OperationStatistics
type DisruptionSummary = dsmodel.DisruptionSummary
type DisruptionListResponse = dsmodel.DisruptionListResponse
type DisruptionAffectedRoute = dsmodel.DisruptionAffectedRoute
type DisruptionDetail = dsmodel.DisruptionDetail

// gateway-client-specific types
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
