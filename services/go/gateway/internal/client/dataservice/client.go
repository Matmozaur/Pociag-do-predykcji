package dataservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

var ErrNotFound = errors.New("resource not found")

type QueryRoutesParams struct {
	DateFrom           *string
	DateTo             *string
	StationExternalIDs []int
	FromCity           string
	ToCity             string
	CarrierCodes       []string
	CommercialCategory string
	Limit              int
	Offset             int
}

type QueryOperationsParams struct {
	Date               *string
	StationExternalIDs []int
	Status             string
	CarrierCodes       []string
	Limit              int
	Offset             int
}

type QueryDisruptionsParams struct {
	DateFrom *string
	DateTo   *string
	Limit    int
	Offset   int
}

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

func New(baseURL string, timeout time.Duration, httpClient *http.Client) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}

	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	return &Client{baseURL: parsed, httpClient: client}, nil
}

func (c *Client) Ready(ctx context.Context) error {
	_, err := c.doRequest(ctx, http.MethodGet, "/readyz", nil)
	if err != nil {
		return fmt.Errorf("check data-service readiness: %w", err)
	}
	return nil
}

func (c *Client) QueryStations(ctx context.Context, search string, limit, offset int) (*StationListResponse, error) {
	q := url.Values{}
	if search != "" {
		q.Set("search", search)
	}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))

	body, err := c.doRequest(ctx, http.MethodGet, "/api/v1/stations", q)
	if err != nil {
		return nil, fmt.Errorf("query stations: %w", err)
	}

	var out StationListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode station response: %w", err)
	}
	return &out, nil
}

func (c *Client) GetStationByExternalID(ctx context.Context, externalID int) (*Station, error) {
	body, err := c.doRequest(ctx, http.MethodGet, "/api/v1/stations/"+strconv.Itoa(externalID), nil)
	if err != nil {
		return nil, fmt.Errorf("get station by external ID: %w", err)
	}

	var out Station
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode station: %w", err)
	}
	return &out, nil
}

func (c *Client) ListCarriers(ctx context.Context) (*CarrierListResponse, error) {
	body, err := c.doRequest(ctx, http.MethodGet, "/api/v1/carriers", nil)
	if err != nil {
		return nil, fmt.Errorf("list carriers: %w", err)
	}

	var out CarrierListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode carriers response: %w", err)
	}
	return &out, nil
}

func (c *Client) QueryRoutes(ctx context.Context, p QueryRoutesParams) (*RouteListResponse, error) {
	q := url.Values{}
	if p.DateFrom != nil {
		q.Set("dateFrom", *p.DateFrom)
	}
	if p.DateTo != nil {
		q.Set("dateTo", *p.DateTo)
	}
	if len(p.StationExternalIDs) > 0 {
		parts := make([]string, 0, len(p.StationExternalIDs))
		for _, id := range p.StationExternalIDs {
			parts = append(parts, strconv.Itoa(id))
		}
		q.Set("stationExternalIds", strings.Join(parts, ","))
	}
	if p.FromCity != "" {
		q.Set("fromCity", p.FromCity)
	}
	if p.ToCity != "" {
		q.Set("toCity", p.ToCity)
	}
	if len(p.CarrierCodes) > 0 {
		q.Set("carrierCodes", strings.Join(p.CarrierCodes, ","))
	}
	if p.CommercialCategory != "" {
		q.Set("commercialCategory", p.CommercialCategory)
	}
	q.Set("limit", strconv.Itoa(p.Limit))
	q.Set("offset", strconv.Itoa(p.Offset))

	body, err := c.doRequest(ctx, http.MethodGet, "/api/v1/schedules", q)
	if err != nil {
		return nil, fmt.Errorf("query routes: %w", err)
	}

	var out RouteListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode routes response: %w", err)
	}
	return &out, nil
}

func (c *Client) GetRouteByID(ctx context.Context, routeID int64) (*RouteDetail, error) {
	body, err := c.doRequest(ctx, http.MethodGet, "/api/v1/schedules/"+strconv.FormatInt(routeID, 10), nil)
	if err != nil {
		return nil, fmt.Errorf("get route by ID: %w", err)
	}

	var out RouteDetail
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode route detail: %w", err)
	}
	return &out, nil
}

func (c *Client) GetRouteByKey(ctx context.Context, scheduleID, orderID int) (*RouteDetail, error) {
	p := "/api/v1/schedules/by-key/" + strconv.Itoa(scheduleID) + "/" + strconv.Itoa(orderID)
	body, err := c.doRequest(ctx, http.MethodGet, p, nil)
	if err != nil {
		return nil, fmt.Errorf("get route by key: %w", err)
	}

	var out RouteDetail
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode route detail by key: %w", err)
	}
	return &out, nil
}

func (c *Client) GetRouteStations(ctx context.Context, routeID int64) (*RouteStationListResponse, error) {
	p := "/api/v1/schedules/" + strconv.FormatInt(routeID, 10) + "/stations"
	body, err := c.doRequest(ctx, http.MethodGet, p, nil)
	if err != nil {
		return nil, fmt.Errorf("get route stations: %w", err)
	}

	var out RouteStationListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode route stations: %w", err)
	}
	return &out, nil
}

func (c *Client) GetRouteOperatingDates(ctx context.Context, routeID int64) (*OperatingDatesResponse, error) {
	p := "/api/v1/schedules/" + strconv.FormatInt(routeID, 10) + "/operating-dates"
	body, err := c.doRequest(ctx, http.MethodGet, p, nil)
	if err != nil {
		return nil, fmt.Errorf("get route operating dates: %w", err)
	}

	var out OperatingDatesResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode route operating dates: %w", err)
	}
	return &out, nil
}

func (c *Client) QueryOperations(ctx context.Context, p QueryOperationsParams) (*OperationListResponse, error) {
	q := url.Values{}
	if p.Date != nil {
		q.Set("date", *p.Date)
	}
	if len(p.StationExternalIDs) > 0 {
		parts := make([]string, 0, len(p.StationExternalIDs))
		for _, id := range p.StationExternalIDs {
			parts = append(parts, strconv.Itoa(id))
		}
		q.Set("stationExternalIds", strings.Join(parts, ","))
	}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if len(p.CarrierCodes) > 0 {
		q.Set("carrierCodes", strings.Join(p.CarrierCodes, ","))
	}
	q.Set("limit", strconv.Itoa(p.Limit))
	q.Set("offset", strconv.Itoa(p.Offset))

	body, err := c.doRequest(ctx, http.MethodGet, "/api/v1/operations", q)
	if err != nil {
		return nil, fmt.Errorf("query operations: %w", err)
	}

	var out OperationListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode operations response: %w", err)
	}
	return &out, nil
}

func (c *Client) GetOperationByID(ctx context.Context, operationID int64) (*OperationDetail, error) {
	body, err := c.doRequest(ctx, http.MethodGet, "/api/v1/operations/"+strconv.FormatInt(operationID, 10), nil)
	if err != nil {
		return nil, fmt.Errorf("get operation by ID: %w", err)
	}

	var out OperationDetail
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode operation detail: %w", err)
	}
	return &out, nil
}

func (c *Client) GetOperationStatistics(ctx context.Context, date string) (*OperationStatistics, error) {
	q := url.Values{}
	if date != "" {
		q.Set("date", date)
	}

	body, err := c.doRequest(ctx, http.MethodGet, "/api/v1/operations/statistics", q)
	if err != nil {
		return nil, fmt.Errorf("get operation statistics: %w", err)
	}

	var out OperationStatistics
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode operation statistics: %w", err)
	}
	return &out, nil
}

func (c *Client) QueryDisruptions(ctx context.Context, p QueryDisruptionsParams) (*DisruptionListResponse, error) {
	q := url.Values{}
	if p.DateFrom != nil {
		q.Set("dateFrom", *p.DateFrom)
	}
	if p.DateTo != nil {
		q.Set("dateTo", *p.DateTo)
	}
	q.Set("limit", strconv.Itoa(p.Limit))
	q.Set("offset", strconv.Itoa(p.Offset))

	body, err := c.doRequest(ctx, http.MethodGet, "/api/v1/disruptions", q)
	if err != nil {
		return nil, fmt.Errorf("query disruptions: %w", err)
	}

	var out DisruptionListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode disruptions response: %w", err)
	}
	return &out, nil
}

func (c *Client) GetDisruptionByID(ctx context.Context, disruptionID int64) (*DisruptionDetail, error) {
	p := "/api/v1/disruptions/" + strconv.FormatInt(disruptionID, 10)
	body, err := c.doRequest(ctx, http.MethodGet, p, nil)
	if err != nil {
		return nil, fmt.Errorf("get disruption by ID: %w", err)
	}

	var out DisruptionDetail
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode disruption detail: %w", err)
	}
	return &out, nil
}

func (c *Client) doRequest(ctx context.Context, method, endpoint string, query url.Values) ([]byte, error) {
	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, endpoint)
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return body, nil
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}

	var out errorResponse
	if err := json.Unmarshal(body, &out); err == nil && out.Message != "" {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, out.Message)
	}

	return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
}
