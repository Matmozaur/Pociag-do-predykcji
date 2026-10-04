package service

import (
	"context"
	"fmt"
	"time"

	"github.com/pociag-do-predykcji/services/go/api/internal/model"
	"github.com/pociag-do-predykcji/services/go/api/internal/position"
)

// activeTrain is a run that passes the active predicate at an instant, with its estimate.
type activeTrain struct {
	Run        Run
	Stops      []position.TimedStop
	Estimate   position.Result
	Delay      *int // delay at the last confirmed stop
	Confidence string
}

// activeTrains returns the trains active at `at`, ordered by operation id, optionally filtered by
// carrier code, and the latest snapshot time (nil when there are no operations).
func (s *Service) activeTrains(ctx context.Context, at time.Time, carriers []string) ([]activeTrain, *time.Time, error) {
	snapshot, err := s.repo.LatestSnapshot(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("active trains: %w", err)
	}
	if snapshot.IsZero() {
		return []activeTrain{}, nil, nil
	}
	runs, err := s.repo.ListActiveRuns(ctx, at, snapshot.Add(-position.SnapshotWindow), position.OperatingDates(at))
	if err != nil {
		return nil, nil, fmt.Errorf("active trains: %w", err)
	}

	out := []activeTrain{}
	for _, run := range runs {
		if len(carriers) > 0 && !containsFold(carriers, run.Train.CarrierCode) {
			continue
		}
		stops := position.EffectiveTimes(run.Stops)
		op := position.Operation{TrainStatus: run.Status, OperatingDate: run.OperatingDate, LastSeenAt: run.UpdatedAt}
		if !position.IsActive(op, stops, at, snapshot) {
			continue
		}
		est, ok := position.Estimate(stops, at)
		if !ok {
			continue
		}
		t := activeTrain{Run: run, Stops: stops, Estimate: est}
		lastConfirmed, delay, confirmed := position.LastConfirmed(stops)
		if confirmed {
			t.Delay = &delay
		}
		t.Confidence = position.Confidence(run.UpdatedAt, lastConfirmed, at)
		out = append(out, t)
	}
	snapshotUTC := snapshot.UTC()
	return out, &snapshotUTC, nil
}

// LiveTrains is the paginated list of active trains.
func (s *Service) LiveTrains(ctx context.Context, carriers []string, limit, offset int) (*model.LiveTrainsResponse, error) {
	now := s.now().UTC()
	trains, _, err := s.activeTrains(ctx, now, carriers)
	if err != nil {
		return nil, err
	}
	page := trains[min(offset, len(trains)):min(offset+limit, len(trains))]

	data := make([]model.LiveTrainSummary, 0, len(page))
	for _, t := range page {
		summary := model.LiveTrainSummary{
			OperationID:  t.Run.OperationID,
			TrainName:    trainName(t.Run.Train),
			CarrierCode:  t.Run.Train.CarrierCode,
			Status:       statusLabel(t.Run.Status),
			StatusCode:   t.Run.Status,
			DelayMinutes: t.Delay,
			Origin:       t.Stops[0].StationName,
			Destination:  t.Stops[len(t.Stops)-1].StationName,
		}
		if t.Estimate.Previous != nil {
			summary.CurrentStation = t.Estimate.Previous.StationName
		}
		if t.Estimate.Next != nil {
			summary.NextStation = t.Estimate.Next.StationName
		}
		data = append(data, summary)
	}
	return &model.LiveTrainsResponse{
		Data:        data,
		Pagination:  model.NewPagination(len(trains), limit, offset),
		GeneratedAt: now,
	}, nil
}

// MapTrains returns the active trains that have an estimated position; the others are counted
// in UnpositionedCount.
func (s *Service) MapTrains(ctx context.Context, carriers []string) (*model.TrainMapResponse, error) {
	now := s.now().UTC()
	trains, snapshot, err := s.activeTrains(ctx, now, carriers)
	if err != nil {
		return nil, err
	}

	resp := &model.TrainMapResponse{Trains: []model.TrainMapPoint{}, GeneratedAt: now, DataAsOf: snapshot}
	for _, t := range trains {
		p := t.Estimate.Position
		if p == nil {
			resp.UnpositionedCount++
			continue
		}
		point := model.TrainMapPoint{
			OperationID:  t.Run.OperationID,
			TrainName:    trainName(t.Run.Train),
			CarrierCode:  t.Run.Train.CarrierCode,
			Status:       statusLabel(t.Run.Status),
			Phase:        t.Estimate.Phase,
			DelayMinutes: t.Delay,
			Latitude:     p.Latitude,
			Longitude:    p.Longitude,
			Progress:     p.Progress,
			Method:       p.Method,
			Confidence:   t.Confidence,
			Origin:       t.Stops[0].StationName,
			Destination:  t.Stops[len(t.Stops)-1].StationName,
		}
		if prev := t.Estimate.Previous; prev != nil {
			point.PreviousStop = mapStop(*prev, prev.Departure)
		}
		if next := t.Estimate.Next; next != nil {
			point.NextStop = mapStop(*next, next.Arrival)
		}
		resp.Trains = append(resp.Trains, point)
	}
	return resp, nil
}

func mapStop(s position.TimedStop, t time.Time) *model.TrainMapStop {
	return &model.TrainMapStop{StationName: s.StationName, Time: t.UTC(), Latitude: s.Latitude, Longitude: s.Longitude}
}

// TrainDetail is one run with all its stops, planned and actual.
func (s *Service) TrainDetail(ctx context.Context, operationID int64) (*model.TrainDetailView, error) {
	run, err := s.repo.GetRun(ctx, operationID)
	if err != nil {
		return nil, fmt.Errorf("train detail: %w", err)
	}

	view := &model.TrainDetailView{
		OperationID:   run.OperationID,
		TrainName:     trainName(run.Train),
		OperatingDate: run.OperatingDate.Format(time.DateOnly),
		Status:        statusLabel(run.Status),
		StatusCode:    run.Status,
		Stops:         make([]model.TrainStopView, 0, len(run.Stops)),
	}
	if code := trimmed(run.Train.CarrierCode); code != "" {
		view.Carrier = &model.Carrier{Code: code, Name: run.Train.CarrierName}
	}
	for _, st := range run.Stops {
		name := ""
		if st.StationName != nil {
			name = *st.StationName
		}
		view.Stops = append(view.Stops, model.TrainStopView{
			StationName:           name,
			StationExternalID:     st.StationID,
			Sequence:              st.SequenceNumber,
			PlannedArrival:        clock(st.PlannedArrival),
			PlannedDeparture:      clock(st.PlannedDeparture),
			ActualArrival:         clock(st.ActualArrival),
			ActualDeparture:       clock(st.ActualDeparture),
			ArrivalDelayMinutes:   st.ArrivalDelayMinutes,
			DepartureDelayMinutes: st.DepartureDelayMinutes,
			IsConfirmed:           st.IsConfirmed,
			IsCancelled:           st.IsCancelled,
		})
	}
	return view, nil
}
