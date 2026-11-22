package prediction_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pociag-do-predykcji/services/go/api/internal/prediction"
)

const validBody = `{"buckets":[
	{"range":"0-5","probability":0.5},{"range":"5-10","probability":0.25},{"range":"10-20","probability":0.125},
	{"range":"20-40","probability":0.0625},{"range":"40-60","probability":0.03125},
	{"range":"60-120","probability":0.015625},{"range":"120+","probability":0.015625}],
	"model_version":"history-7d","is_mock":false}`

// predictor answers every request with status and body, and records the last request body.
func predictor(t *testing.T, status int, body string) (*prediction.Client, *map[string]any) {
	t.Helper()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/predict-delay" {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return prediction.NewClient(srv.URL + "/"), &got
}

func TestClientPredictDelay(t *testing.T) {
	client, got := predictor(t, http.StatusOK, validBody)

	p, err := client.PredictDelay(context.Background(), prediction.Request{TrainID: 3, FromStationID: 5100, ToStationID: 4900})
	if err != nil {
		t.Fatal(err)
	}
	if want := (map[string]any{"train_id": 3.0, "to_station_id": 4900.0}); len(*got) != 2 || (*got)["train_id"] != want["train_id"] || (*got)["to_station_id"] != want["to_station_id"] {
		t.Errorf("request body = %v, want %v", *got, want)
	}
	if p.Probabilities != [7]float64{0.5, 0.25, 0.125, 0.0625, 0.03125, 0.015625, 0.015625} {
		t.Errorf("probabilities = %v", p.Probabilities)
	}
	if p.ModelVersion != "history-7d" || p.IsMock {
		t.Errorf("model version %q, is mock %v", p.ModelVersion, p.IsMock)
	}
}

func TestClientRejectsBadAnswers(t *testing.T) {
	for name, c := range map[string]struct {
		status int
		body   string
	}{
		"server error":      {http.StatusInternalServerError, `{}`},
		"not json":          {http.StatusOK, `oops`},
		"too few ranges":    {http.StatusOK, `{"buckets":[{"range":"0-5","probability":1}]}`},
		"wrong range order": {http.StatusOK, `{"buckets":[{"range":"5-10","probability":0.5},{"range":"0-5","probability":0.5},{"range":"10-20","probability":0},{"range":"20-40","probability":0},{"range":"40-60","probability":0},{"range":"60-120","probability":0},{"range":"120+","probability":0}]}`},
		"sum is not 1":      {http.StatusOK, `{"buckets":[{"range":"0-5","probability":0.5},{"range":"5-10","probability":0.1},{"range":"10-20","probability":0},{"range":"20-40","probability":0},{"range":"40-60","probability":0},{"range":"60-120","probability":0},{"range":"120+","probability":0}]}`},
		"probability > 1":   {http.StatusOK, `{"buckets":[{"range":"0-5","probability":2},{"range":"5-10","probability":-1},{"range":"10-20","probability":0},{"range":"20-40","probability":0},{"range":"40-60","probability":0},{"range":"60-120","probability":0},{"range":"120+","probability":0}]}`},
	} {
		client, _ := predictor(t, c.status, c.body)
		if _, err := client.PredictDelay(context.Background(), prediction.Request{TrainID: 3, ToStationID: 4900}); !errors.Is(err, prediction.ErrUnavailable) {
			t.Errorf("%s: err = %v, want ErrUnavailable", name, err)
		}
	}
}

func TestClientUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	_, err := prediction.NewClient(srv.URL).PredictDelay(context.Background(), prediction.Request{TrainID: 3, ToStationID: 4900})
	if !errors.Is(err, prediction.ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}
