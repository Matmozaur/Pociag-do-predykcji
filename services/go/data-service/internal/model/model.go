package model

import "github.com/pociag-do-predykcji/services/go/shared/dsmodel"

type Pagination = dsmodel.Pagination
type ErrorResponse = dsmodel.ErrorResponse

// Dictionaries
type Station = dsmodel.Station
type StationListResponse = dsmodel.StationListResponse
type Carrier = dsmodel.Carrier
type CarrierListResponse = dsmodel.CarrierListResponse
type CommercialCategory = dsmodel.CommercialCategory
type CommercialCategoryListResponse = dsmodel.CommercialCategoryListResponse
type StopType = dsmodel.StopType
type StopTypeListResponse = dsmodel.StopTypeListResponse

// Schedules
type RouteSummary = dsmodel.RouteSummary
type RouteListResponse = dsmodel.RouteListResponse
type RouteStation = dsmodel.RouteStation
type RouteStationListResponse = dsmodel.RouteStationListResponse
type OperatingDatesResponse = dsmodel.OperatingDatesResponse
type RouteConnection = dsmodel.RouteConnection
type RouteDetail = dsmodel.RouteDetail

// Operations
type OperationSummary = dsmodel.OperationSummary
type OperationListResponse = dsmodel.OperationListResponse
type OperationStation = dsmodel.OperationStation
type OperationDetail = dsmodel.OperationDetail
type DelayDistribution = dsmodel.DelayDistribution
type OperationStatistics = dsmodel.OperationStatistics

// Disruptions
type DisruptionSummary = dsmodel.DisruptionSummary
type DisruptionListResponse = dsmodel.DisruptionListResponse
type DisruptionAffectedRoute = dsmodel.DisruptionAffectedRoute
type DisruptionDetail = dsmodel.DisruptionDetail
