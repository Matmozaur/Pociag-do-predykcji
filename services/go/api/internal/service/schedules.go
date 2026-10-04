package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pociag-do-predykcji/services/go/api/internal/model"
)

// Sort orders of SearchSchedules.
const (
	SortDeparture = "departure"
	SortArrival   = "arrival"
	SortDuration  = "duration"
)

// ParsePlace reads a search endpoint: a positive station id, else a city or station-name prefix.
func ParsePlace(raw string) Place {
	raw = strings.TrimSpace(raw)
	if id, err := strconv.Atoi(raw); err == nil && id > 0 {
		return Place{StationID: id}
	}
	return Place{Name: raw}
}

// SearchSchedules lists the trains running on q.Date from q.From to q.To, sorted and paginated.
func (s *Service) SearchSchedules(ctx context.Context, q ConnectionQuery, sortBy string, limit, offset int) (*model.ScheduleSearchResponse, error) {
	connections, err := s.repo.SearchConnections(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("search schedules: %w", err)
	}

	results := make([]model.ScheduleSearchResult, 0, len(connections))
	for _, c := range connections {
		results = append(results, searchResult(c))
	}
	less := func(a, b model.ScheduleSearchResult) bool { return a.Departure.Time < b.Departure.Time }
	switch sortBy {
	case SortArrival:
		less = func(a, b model.ScheduleSearchResult) bool { return a.Arrival.Time < b.Arrival.Time }
	case SortDuration:
		less = func(a, b model.ScheduleSearchResult) bool { return a.DurationMinutes < b.DurationMinutes }
	}
	sort.SliceStable(results, func(i, j int) bool { return less(results[i], results[j]) })

	return &model.ScheduleSearchResponse{
		Data:       results[min(offset, len(results)):min(offset+limit, len(results))],
		Pagination: model.NewPagination(len(results), limit, offset),
		Query:      model.ScheduleSearchQuery{From: placeString(q.From), To: placeString(q.To), Date: q.Date.Format(time.DateOnly)},
	}, nil
}

func searchResult(c Connection) model.ScheduleSearchResult {
	result := model.ScheduleSearchResult{
		RouteID:            c.Train.ID,
		TrainName:          trainName(c.Train),
		Carrier:            model.Carrier{Code: trimmed(c.Train.CarrierCode), Name: c.Train.CarrierName},
		CommercialCategory: c.Train.Category,
		Departure:          model.ScheduleEndpoint{StationName: c.From.StationName, StationExternalID: c.From.StationID},
		Arrival:            model.ScheduleEndpoint{StationName: c.To.StationName, StationExternalID: c.To.StationID},
		StopsCount:         max(c.To.Seq-c.From.Seq-1, 0),
	}
	if t := offsetClock(c.From.Departure); t != nil {
		result.Departure.Time = *t
	}
	if t := offsetClock(c.To.Arrival); t != nil {
		result.Arrival.Time = *t
	}
	if c.From.Departure != nil && c.To.Arrival != nil {
		result.DurationMinutes = int((*c.To.Arrival - *c.From.Departure).Minutes())
	}
	return result
}

func placeString(p Place) string {
	if p.StationID > 0 {
		return strconv.Itoa(p.StationID)
	}
	return p.Name
}

// ScheduleDetail is a train's timetable and its upcoming operating dates.
func (s *Service) ScheduleDetail(ctx context.Context, trainID int64) (*model.ScheduleDetailView, error) {
	schedule, err := s.repo.GetTrainSchedule(ctx, trainID)
	if err != nil {
		return nil, fmt.Errorf("schedule detail: %w", err)
	}

	train := schedule.Train
	view := &model.ScheduleDetailView{
		RouteID:            train.ID,
		TrainName:          trainName(train),
		Carrier:            model.Carrier{Code: trimmed(train.CarrierCode), Name: train.CarrierName},
		CommercialCategory: train.Category,
		NationalNumber:     train.Number,
		Stops:              make([]model.ScheduleStopView, 0, len(schedule.Stops)),
		OperatingDates:     []string{},
	}
	today := warsawDate(s.now())
	for _, d := range schedule.OperatingDates {
		if !d.Before(today) {
			view.OperatingDates = append(view.OperatingDates, d.Format(time.DateOnly))
		}
	}
	for _, st := range schedule.Stops {
		view.Stops = append(view.Stops, model.ScheduleStopView{
			StationName:       st.StationName,
			StationExternalID: st.StationID,
			Order:             st.Seq,
			ArrivalTime:       offsetClock(st.Arrival),
			DepartureTime:     offsetClock(st.Departure),
			Platform:          st.Platform,
		})
	}
	if n := len(schedule.Stops); n > 1 && schedule.Stops[0].Departure != nil && schedule.Stops[n-1].Arrival != nil {
		minutes := int((*schedule.Stops[n-1].Arrival - *schedule.Stops[0].Departure).Minutes())
		view.TotalDurationMinutes = &minutes
	}
	return view, nil
}
