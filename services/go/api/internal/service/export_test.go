package service

import (
	"time"

	"github.com/pociag-do-predykcji/services/go/api/internal/prediction"
)

// NewAt is New with a fixed clock.
func NewAt(repo Repository, now time.Time) *Service {
	return &Service{repo: repo, predictor: prediction.Mock{}, now: func() time.Time { return now }}
}

var (
	TrainName   = trainName
	OffsetClock = offsetClock
)
