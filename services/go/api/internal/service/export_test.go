package service

import (
	"time"

	"github.com/pociag-do-predykcji/services/go/api/internal/prediction"
)

// NewAt is New with a fixed clock and no predictor.
func NewAt(repo Repository, now time.Time) *Service { return NewAtWithPredictor(repo, nil, now) }

// NewAtWithPredictor is New with a fixed clock.
func NewAtWithPredictor(repo Repository, predictor prediction.Predictor, now time.Time) *Service {
	return &Service{repo: repo, predictor: predictor, now: func() time.Time { return now }}
}

var (
	TrainName   = trainName
	OffsetClock = offsetClock
)
