package service

import (
	"context"
	"fmt"
	"time"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/position"
)

// ActiveOperationCandidate is one operation that may be active, with all its non-cancelled stops
// ordered by actual sequence number.
type ActiveOperationCandidate struct {
	OperationID   int64
	ScheduleID    int
	OrderID       int
	OperatingDate time.Time
	TrainStatus   string
	UpdatedAt     time.Time // train_operations.updated_at
	RouteName     *string
	CarrierCode   *string
	Stops         []position.Stop
}

// ListActiveOperations returns the trains active at `at` (the current time when nil), ordered by
// operation_id and truncated to limit; Total counts them all.
func (s *Service) ListActiveOperations(ctx context.Context, at *time.Time, carrierCodes []string, limit int) (*model.ActiveTrainListResponse, error) {
	generatedAt := time.Now().UTC()
	ref := generatedAt
	if at != nil {
		ref = at.UTC()
	}

	cands, dataAsOf, err := s.ListActiveOperationCandidates(ctx, ref, position.OperatingDates(ref), carrierCodes)
	if err != nil {
		return nil, fmt.Errorf("list active operations: %w", err)
	}

	data, total := activeTrains(cands, ref, dataAsOf, limit)
	return &model.ActiveTrainListResponse{
		Data:        data,
		Total:       total,
		GeneratedAt: generatedAt,
		DataAsOf:    dataAsOf.UTC(),
	}, nil
}

// activeTrains applies the active predicate to the candidates and estimates each active train.
func activeTrains(cands []ActiveOperationCandidate, at, latestSnapshot time.Time, limit int) ([]model.ActiveTrain, int) {
	data := []model.ActiveTrain{}
	total := 0
	for _, c := range cands {
		stops := position.EffectiveTimes(c.Stops)
		op := position.Operation{TrainStatus: c.TrainStatus, OperatingDate: c.OperatingDate, LastSeenAt: c.UpdatedAt}
		if !position.IsActive(op, stops, at, latestSnapshot) {
			continue
		}
		total++
		if len(data) >= limit {
			continue
		}
		est, ok := position.Estimate(stops, at)
		if !ok {
			continue
		}

		t := model.ActiveTrain{
			OperationID:   c.OperationID,
			ScheduleID:    c.ScheduleID,
			OrderID:       c.OrderID,
			OperatingDate: c.OperatingDate.Format("2006-01-02"),
			TrainStatus:   c.TrainStatus,
			RouteName:     c.RouteName,
			CarrierCode:   c.CarrierCode,
			Origin:        stopRef(stops[0]),
			Destination:   stopRef(stops[len(stops)-1]),
			Phase:         est.Phase,
			Confidence:    position.ConfidenceLow,
			LastSeenAt:    c.UpdatedAt.UTC(),
		}
		if est.Previous != nil {
			t.PreviousStop = stopTiming(*est.Previous, est.Previous.Departure)
		}
		if est.Next != nil {
			t.NextStop = stopTiming(*est.Next, est.Next.Arrival)
		}
		lastConfirmed, delay, confirmed := position.LastConfirmed(stops)
		if confirmed {
			t.LastConfirmedStop = stopTiming(*lastConfirmed, lastConfirmed.Departure)
			t.DelayMinutes = &delay
		}
		if p := est.Position; p != nil {
			t.Position = &model.TrainPosition{Latitude: p.Latitude, Longitude: p.Longitude, Progress: p.Progress, Method: p.Method}
		}
		t.Confidence = position.Confidence(c.UpdatedAt, lastConfirmed, at)
		data = append(data, t)
	}
	return data, total
}

func stopRef(s position.TimedStop) *model.StopRef {
	return &model.StopRef{StationExternalID: s.StationExternalID, StationName: s.StationName}
}

func stopTiming(s position.TimedStop, t time.Time) *model.StopTiming {
	return &model.StopTiming{
		StationExternalID: s.StationExternalID,
		StationName:       s.StationName,
		SequenceNumber:    s.SequenceNumber,
		Time:              t.UTC(),
		IsConfirmed:       s.IsConfirmed,
		Latitude:          s.Latitude,
		Longitude:         s.Longitude,
	}
}
