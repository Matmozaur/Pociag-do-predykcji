package plk

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	tracer     trace.Tracer
}

func New(baseURL string, apiKey string, httpClient *http.Client) *Client {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: client,
		tracer:     otel.Tracer("pociag.collector"),
	}
}

func (c *Client) FetchDictionaries(ctx context.Context) (map[string][]byte, error) {
	endpoints := map[string]string{
		"carriers":              "/api/v1/dictionaries/carriers",
		"stations":              "/api/v1/dictionaries/stations",
		"commercial_categories": "/api/v1/dictionaries/commercial-categories",
		"stop_types":            "/api/v1/dictionaries/stop-types",
		"cities":                "/api/v1/dictionaries/cities",
	}

	results := make(map[string][]byte, len(endpoints))
	for dictionaryType, endpoint := range endpoints {
		payload, err := c.doGET(ctx, "plk.dictionaries.fetch", endpoint, map[string]string{})
		if err != nil {
			return nil, fmt.Errorf("fetch dictionary %s: %w", dictionaryType, err)
		}
		results[dictionaryType] = payload
	}

	return results, nil
}

func (c *Client) FetchSchedules(ctx context.Context, dateFrom time.Time, dateTo time.Time, page int, pageSize int) ([]byte, error) {
	return c.doGET(ctx, "plk.schedules.fetch", "/api/v1/schedules", map[string]string{
		"dateFrom": dateFrom.Format("2006-01-02"),
		"dateTo":   dateTo.Format("2006-01-02"),
		"page":     fmt.Sprintf("%d", page),
		"pageSize": fmt.Sprintf("%d", pageSize),
	})
}

func (c *Client) FetchOperations(ctx context.Context, operatingDate time.Time, page int, pageSize int) ([]byte, error) {
	return c.doGET(ctx, "plk.operations.fetch", "/api/v1/operations", map[string]string{
		"date":     operatingDate.Format("2006-01-02"),
		"page":     fmt.Sprintf("%d", page),
		"pageSize": fmt.Sprintf("%d", pageSize),
	})
}

func (c *Client) FetchDisruptions(ctx context.Context, dateFrom time.Time, dateTo time.Time) ([]byte, error) {
	return c.doGET(ctx, "plk.disruptions.fetch", "/api/v1/disruptions", map[string]string{
		"dateFrom": dateFrom.Format("2006-01-02"),
		"dateTo":   dateTo.Format("2006-01-02"),
	})
}

func (c *Client) doGET(ctx context.Context, spanName string, endpoint string, queryParams map[string]string) ([]byte, error) {
	ctx, span := c.tracer.Start(ctx, spanName)
	defer span.End()

	targetURL, err := url.Parse(c.baseURL + endpoint)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("parse target URL: %w", err)
	}

	values := targetURL.Query()
	for key, value := range queryParams {
		if value != "" {
			values.Set(key, value)
		}
	}
	targetURL.RawQuery = values.Encode()

	span.SetAttributes(
		attribute.String("http.method", http.MethodGet),
		attribute.String("http.url", targetURL.String()),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			span.RecordError(closeErr)
		}
	}()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("request %s: %w", endpoint, err)
	}

	return payload, nil
}
