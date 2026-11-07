// Package prediction is the api's side of the delay model: the Predictor interface the ML
// service client will implement, the fixed delay ranges it predicts, and Mock, a deterministic
// placeholder used until that service exists.
package prediction

import (
	"context"
	"encoding/binary"
	"hash/fnv"
	"math/rand/v2"
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

// Mock returns a plausible, deterministic distribution per (train, date, journey): mostly on
// time, with a tail that grows with the journey's length.
type Mock struct{}

// MockModelVersion is the ModelVersion of Mock predictions.
const MockModelVersion = "mock-0"

func (Mock) PredictDelay(_ context.Context, req Request) (*Prediction, error) {
	h := fnv.New64a()
	for _, v := range []int64{req.TrainID, req.Date.Unix(), int64(req.FromStationID), int64(req.ToStationID)} {
		_ = binary.Write(h, binary.LittleEndian, v) // hash.Hash writes never fail
	}
	rng := rand.New(rand.NewPCG(h.Sum64(), 0))

	// Longer journeys pick up more delay: shift weight from the first bucket to the tail.
	hours := min(req.Arrival.Sub(req.Departure).Hours(), 8)
	base := [len(Buckets)]float64{0.70 - 0.05*hours, 0.13, 0.08 + 0.015*hours, 0.05 + 0.015*hours, 0.02 + 0.01*hours, 0.015 + 0.007*hours, 0.005 + 0.003*hours}

	p := &Prediction{ModelVersion: MockModelVersion, IsMock: true}
	var sum float64
	for i, w := range base {
		p.Probabilities[i] = w * (0.6 + 0.8*rng.Float64())
		sum += p.Probabilities[i]
	}
	for i := range p.Probabilities {
		p.Probabilities[i] /= sum
	}
	return p, nil
}
