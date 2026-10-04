package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/pociag-do-predykcji/services/go/api/internal/model"
	"github.com/pociag-do-predykcji/services/go/api/internal/position"
)

const (
	// boardLookback reads stops planned this long ago, so delayed trains still show up.
	boardLookback = 6 * time.Hour
	// boardHorizon is how far ahead arrivals and departures are listed.
	boardHorizon = 12 * time.Hour
	// terminusDwell is how long a train ending its run at the station still counts as at station.
	terminusDwell = 5 * time.Minute
)

func (s *Service) SearchStations(ctx context.Context, query string, limit int) (*model.StationSuggestionsResponse, error) {
	stations, err := s.repo.SearchStations(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search stations: %w", err)
	}
	out := make([]model.StationSuggestion, 0, len(stations))
	for _, st := range stations {
		out = append(out, model.StationSuggestion{ExternalID: st.ID, Name: st.Name, City: st.City})
	}
	return &model.StationSuggestionsResponse{Suggestions: out}, nil
}

func (s *Service) MapStations(ctx context.Context) (*model.StationMapResponse, error) {
	stations, err := s.repo.ListMappedStations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list map stations: %w", err)
	}
	out := make([]model.StationMapPoint, 0, len(stations))
	for _, st := range stations {
		if st.Latitude == nil || st.Longitude == nil {
			continue
		}
		out = append(out, model.StationMapPoint{
			ExternalID: st.ID, Name: st.Name, City: st.City, Latitude: *st.Latitude, Longitude: *st.Longitude,
		})
	}
	return &model.StationMapResponse{Stations: out}, nil
}

// StationBoard lists the trains at the station now and its next arrivals and departures, each
// section ordered by expected time and truncated to limit. A through train can be both an
// arrival and a departure.
func (s *Service) StationBoard(ctx context.Context, stationID, limit int) (*model.StationBoardView, error) {
	station, err := s.repo.GetStation(ctx, stationID)
	if err != nil {
		return nil, fmt.Errorf("station board: %w", err)
	}
	now := s.now().UTC()
	stops, err := s.repo.ListBoardStops(ctx, stationID, now.Add(-boardLookback), now.Add(boardHorizon))
	if err != nil {
		return nil, fmt.Errorf("station board: %w", err)
	}

	atStation, arrivals, departures := bucketBoard(stops, now)
	view := &model.StationBoardView{
		Station: model.BoardStation{
			ExternalID: station.ID, Name: station.Name, City: station.City,
			Latitude: station.Latitude, Longitude: station.Longitude,
		},
		GeneratedAt: now,
		AtStation:   boardRows(atStation, limit, false),
		Arrivals:    boardRows(arrivals, limit, true),
		Departures:  boardRows(departures, limit, false),
	}
	for _, b := range stops {
		if view.DataAsOf == nil || b.Run.UpdatedAt.After(*view.DataAsOf) {
			t := b.Run.UpdatedAt.UTC()
			view.DataAsOf = &t
		}
	}
	return view, nil
}

// bucketBoard splits stops into at_station (arrived, not yet departed; a terminus for
// terminusDwell after arrival), arrivals and departures expected within boardHorizon.
func bucketBoard(stops []BoardStop, now time.Time) (atStation, arrivals, departures []BoardStop) {
	end := now.Add(boardHorizon)
	inHorizon := func(t *time.Time) bool { return t != nil && t.After(now) && !t.After(end) }

	for _, b := range stops {
		st := b.Stop
		expArr, expDep := expectedArrival(st), expectedDeparture(st)
		hasArr, hasDep := st.PlannedArrival != nil, st.PlannedDeparture != nil

		isAtStation := !st.IsCancelled && hasArr && !expArr.After(now) &&
			((hasDep && expDep.After(now)) || (!hasDep && expArr.After(now.Add(-terminusDwell))))
		switch {
		case isAtStation:
			atStation = append(atStation, b)
		case hasDep && inHorizon(expDep):
			departures = append(departures, b)
		}
		if hasArr && inHorizon(expArr) {
			arrivals = append(arrivals, b)
		}
	}
	return atStation, arrivals, departures
}

// expectedArrival is the actual (or forecast) arrival, else planned plus delay, else nil.
func expectedArrival(st position.Stop) *time.Time {
	return expected(st.ActualArrival, st.PlannedArrival, st.ArrivalDelayMinutes)
}

func expectedDeparture(st position.Stop) *time.Time {
	return expected(st.ActualDeparture, st.PlannedDeparture, st.DepartureDelayMinutes)
}

func expected(actual, planned *time.Time, delayMinutes *int) *time.Time {
	if actual != nil {
		return actual
	}
	if planned == nil {
		return nil
	}
	t := *planned
	if delayMinutes != nil {
		t = t.Add(time.Duration(*delayMinutes) * time.Minute)
	}
	return &t
}

// boardRows orders a section by its board time, truncates it and shapes the rows. Arrivals show
// the arrival side of the stop; other sections the departure side, except a terminus.
func boardRows(stops []BoardStop, limit int, arrivalSide bool) []model.StationBoardRow {
	useArrival := func(b BoardStop) bool { return arrivalSide || b.Stop.PlannedDeparture == nil }
	boardTime := func(b BoardStop) time.Time {
		t := expectedDeparture(b.Stop)
		if useArrival(b) {
			t = expectedArrival(b.Stop)
		}
		if t == nil {
			return time.Time{}
		}
		return *t
	}
	sort.SliceStable(stops, func(i, j int) bool {
		ti, tj := boardTime(stops[i]), boardTime(stops[j])
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return stops[i].Run.OperationID < stops[j].Run.OperationID
	})
	if len(stops) > limit {
		stops = stops[:limit]
	}

	rows := make([]model.StationBoardRow, 0, len(stops))
	for _, b := range stops {
		st, train := b.Stop, b.Run.Train
		planned, exp, delay := st.PlannedDeparture, expectedDeparture(st), st.DepartureDelayMinutes
		if useArrival(b) {
			planned, exp, delay = st.PlannedArrival, expectedArrival(st), st.ArrivalDelayMinutes
		}
		var expectedAt *time.Time
		if exp != nil {
			utc := exp.UTC()
			expectedAt = &utc
		}
		var carrier *model.Carrier
		if code := trimmed(train.CarrierCode); code != "" {
			carrier = &model.Carrier{Code: code, Name: train.CarrierName}
		}
		rows = append(rows, model.StationBoardRow{
			OperationID:        b.Run.OperationID,
			TrainName:          trainName(train),
			TrainNumber:        train.Number,
			CommercialCategory: train.Category,
			Carrier:            carrier,
			Origin:             b.Origin,
			Destination:        b.Destination,
			PlannedTime:        clock(planned),
			ExpectedTime:       clock(exp),
			ExpectedAt:         expectedAt,
			DelayMinutes:       delay,
			Platform:           b.Platform,
			Track:              b.Track,
			Status:             statusLabel(b.Run.Status),
			IsCancelled:        st.IsCancelled,
			IsConfirmed:        st.IsConfirmed,
		})
	}
	return rows
}
