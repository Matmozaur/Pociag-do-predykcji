package prediction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// ErrUnavailable is a prediction that could not be obtained: the predictor service is
// unreachable, answered with an error, or answered with something that is not a distribution
// over Buckets.
var ErrUnavailable = errors.New("prediction service unavailable")

// Client is the Predictor backed by the predictor service.
type Client struct {
	endpoint string
	http     *http.Client
}

// NewClient talks to the predictor service at baseURL, e.g. "http://predictor:8000".
func NewClient(baseURL string) *Client {
	return &Client{
		endpoint: strings.TrimRight(baseURL, "/") + "/v1/predict-delay",
		http:     &http.Client{Timeout: 5 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

type predictRequest struct {
	TrainID     int64 `json:"train_id"`
	ToStationID int   `json:"to_station_id"`
}

type predictResponse struct {
	Buckets []struct {
		Range       string  `json:"range"`
		Probability float64 `json:"probability"`
	} `json:"buckets"`
	ModelVersion string `json:"model_version"`
	IsMock       bool   `json:"is_mock"`
}

func (c *Client) PredictDelay(ctx context.Context, req Request) (*Prediction, error) {
	body, err := json.Marshal(predictRequest{TrainID: req.TrainID, ToStationID: req.ToStationID})
	if err != nil {
		return nil, fmt.Errorf("predict delay: encode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("predict delay: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("predict delay: %w: %w", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("predict delay: %w: status %d", ErrUnavailable, resp.StatusCode)
	}

	var out predictResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("predict delay: %w: decode response: %w", ErrUnavailable, err)
	}
	p, err := out.prediction()
	if err != nil {
		return nil, fmt.Errorf("predict delay: %w: %w", ErrUnavailable, err)
	}
	return p, nil
}

// prediction checks that the response covers Buckets in order with probabilities summing to 1.
func (r predictResponse) prediction() (*Prediction, error) {
	if len(r.Buckets) != len(Buckets) {
		return nil, fmt.Errorf("got %d ranges, want %d", len(r.Buckets), len(Buckets))
	}
	p := &Prediction{ModelVersion: r.ModelVersion, IsMock: r.IsMock}
	var sum float64
	for i, b := range r.Buckets {
		if b.Range != Buckets[i].Range {
			return nil, fmt.Errorf("range %d is %q, want %q", i, b.Range, Buckets[i].Range)
		}
		if b.Probability < 0 || b.Probability > 1 {
			return nil, fmt.Errorf("probability of %q is %v", b.Range, b.Probability)
		}
		p.Probabilities[i] = b.Probability
		sum += b.Probability
	}
	if math.Abs(sum-1) > 1e-3 {
		return nil, fmt.Errorf("probabilities sum to %v", sum)
	}
	return p, nil
}
