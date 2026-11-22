package service

import (
	"context"
	"fmt"
	"time"

	"github.com/pociag-do-predykcji/services/go/api/internal/model"
	"github.com/pociag-do-predykcji/services/go/api/internal/prediction"
)

// InvalidRequestError is a request the service rejects; the handler answers it with 400.
type InvalidRequestError struct{ Message string }

func (e *InvalidRequestError) Error() string { return e.Message }

// DelayPrediction predicts the arrival delay at station toID of route trainID's run on date,
// boarding at fromID. A zero fromID / toID is the route's first / last stop.
func (s *Service) DelayPrediction(ctx context.Context, trainID int64, date time.Time, fromID, toID int) (*model.DelayPrediction, error) {
	today := warsawDate(s.now())
	if date.Before(today) || date.After(today.AddDate(0, 0, prediction.HorizonDays)) {
		return nil, &InvalidRequestError{fmt.Sprintf("date must be within the next %d days", prediction.HorizonDays)}
	}

	schedule, err := s.repo.GetTrainSchedule(ctx, trainID)
	if err != nil {
		return nil, fmt.Errorf("delay prediction: %w", err)
	}
	if !runsOn(schedule.OperatingDates, date) || len(schedule.Stops) < 2 {
		return nil, fmt.Errorf("delay prediction: route %d on %s: %w", trainID, date.Format(time.DateOnly), ErrNotFound)
	}

	from, to := 0, len(schedule.Stops)-1
	if fromID != 0 {
		if from = stopIndex(schedule.Stops, fromID); from < 0 {
			return nil, &InvalidRequestError{"from is not a stop of the route"}
		}
	}
	if toID != 0 {
		if to = stopIndex(schedule.Stops, toID); to < 0 {
			return nil, &InvalidRequestError{"to is not a stop of the route"}
		}
	}
	if to <= from {
		return nil, &InvalidRequestError{"to must come after from on the route"}
	}
	fromStop, toStop := schedule.Stops[from], schedule.Stops[to]

	train := schedule.Train
	midnight := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, warsaw())
	req := prediction.Request{
		TrainID:       train.ID,
		Date:          date,
		Category:      trimmed(train.Category),
		CarrierCode:   trimmed(train.CarrierCode),
		FromStationID: fromStop.StationID,
		ToStationID:   toStop.StationID,
		Departure:     midnight.Add(offsetOrZero(fromStop.Departure)),
		Arrival:       midnight.Add(offsetOrZero(toStop.Arrival)),
		StopsBetween:  to - from - 1,
	}
	p, err := s.predictor.PredictDelay(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("delay prediction: %w", err)
	}

	view := &model.DelayPrediction{
		RouteID:      train.ID,
		TrainName:    trainName(train),
		Date:         date.Format(time.DateOnly),
		From:         model.ScheduleEndpoint{StationName: fromStop.StationName, StationExternalID: fromStop.StationID},
		To:           model.ScheduleEndpoint{StationName: toStop.StationName, StationExternalID: toStop.StationID},
		Buckets:      make([]model.DelayBucket, 0, len(prediction.Buckets)),
		ModelVersion: p.ModelVersion,
		IsMock:       p.IsMock,
		GeneratedAt:  s.now().UTC(),
	}
	if t := offsetClock(fromStop.Departure); t != nil {
		view.From.Time = *t
	}
	if t := offsetClock(toStop.Arrival); t != nil {
		view.To.Time = *t
	}
	best := 0
	for i, b := range prediction.Buckets {
		bucket := model.DelayBucket{Range: b.Range, MinMinutes: b.Min, Probability: p.Probabilities[i]}
		if b.Max > 0 {
			bucket.MaxMinutes = &b.Max
		}
		view.Buckets = append(view.Buckets, bucket)
		if p.Probabilities[i] > p.Probabilities[best] {
			best = i
		}
	}
	view.MostLikely = prediction.Buckets[best].Range
	return view, nil
}

func runsOn(dates []time.Time, date time.Time) bool {
	for _, d := range dates {
		if d.Equal(date) {
			return true
		}
	}
	return false
}

func stopIndex(stops []ScheduleStop, stationID int) int {
	for i, st := range stops {
		if st.StationID == stationID {
			return i
		}
	}
	return -1
}
