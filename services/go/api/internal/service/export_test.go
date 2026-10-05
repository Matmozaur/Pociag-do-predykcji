package service

import "time"

// NewAt is New with a fixed clock.
func NewAt(repo Repository, now time.Time) *Service {
	return &Service{repo: repo, now: func() time.Time { return now }}
}

var (
	TrainName   = trainName
	OffsetClock = offsetClock
)
