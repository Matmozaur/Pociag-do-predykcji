// Package prediction is the api's side of the delay model: the Predictor interface, the fixed
// delay ranges it predicts, and Client, which asks the predictor service
// (specs/openapi/predictor.yml) for the distribution.
package prediction

import (
	"context"
	"time"
)

// HorizonDays is how far ahead a run can be predicted: today plus this many days.
const HorizonDays = 14

// Bucket is an arrival delay range (Min, Max] in minutes; Max 0 means open-ended. The first
// bucket also holds early and on-time arrivals.
type Bucket struct {
	Range string
	Min   int
	Max   int
}

// Buckets are the ranges every Prediction covers, in this order.
var Buckets = [...]Bucket{
	{"0-5", 0, 5},
	{"5-10", 5, 10},
	{"10-20", 10, 20},
	{"20-40", 20, 40},
	{"40-60", 40, 60},
	{"60-120", 60, 120},
	{"120+", 120, 0},
}

// Request identifies the run and the journey to predict: the arrival delay at the To stop of the
// train's run on Date. Weather and historical delays are the predictor's own inputs.
type Request struct {
	TrainID       int64
	Date          time.Time // operating date, UTC midnight
	Category      string
	CarrierCode   string
	FromStationID int
	ToStationID   int
	Departure     time.Time // planned, at From
	Arrival       time.Time // planned, at To
	StopsBetween  int
}

// Prediction is a probability per Buckets entry, summing to 1.
type Prediction struct {
	Probabilities [len(Buckets)]float64
	ModelVersion  string
	IsMock        bool
}

// Predictor predicts the arrival delay distribution of a run.
type Predictor interface {
	PredictDelay(ctx context.Context, req Request) (*Prediction, error)
}
