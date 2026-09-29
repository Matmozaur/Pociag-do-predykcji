package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
)

// Station board buckets (StationBoardEntry.bucket in specs/openapi/data-service.yml).
const (
	BucketAtStation = "at_station"
	BucketArrival   = "arrival"
	BucketDeparture = "departure"
)

// terminusDwell is how long a train that ends its run at the station still counts as at station.
const terminusDwell = 5 * time.Minute

// StationBoardCandidate is one stop row at the board station, before bucketing. Entry.Bucket is
// unset; Entry.ExpectedArrival / ExpectedDeparture hold the board times.
type StationBoardCandidate struct {
	Entry     model.StationBoardEntry
	UpdatedAt time.Time // train_operations.updated_at
}

// GetStationBoard reads candidate stops for the station and buckets them (see BucketStationBoard).
func (s *Service) GetStationBoard(ctx context.Context, stationExtID int, now time.Time, horizon, lookback time.Duration, limit int) (*model.StationBoardResponse, error) {
	rows, err := s.QueryStationBoardCandidates(ctx, stationExtID, now, horizon, lookback)
	if err != nil {
		return nil, fmt.Errorf("get station board: %w", err)
	}

	entries := BucketStationBoard(rows, now, horizon, limit)

	updatedAt := make(map[int64]time.Time, len(rows))
	for _, row := range rows {
		updatedAt[row.Entry.OperationID] = row.UpdatedAt
	}
	var dataAsOf *time.Time
	for _, e := range entries {
		if t := updatedAt[e.OperationID]; dataAsOf == nil || t.After(*dataAsOf) {
			dataAsOf = &t
		}
	}

	return &model.StationBoardResponse{
		StationExternalID: stationExtID,
		At:                now,
		Limit:             limit,
		HorizonMinutes:    int(horizon / time.Minute),
		DataAsOf:          dataAsOf,
		Entries:           entries,
	}, nil
}

// BucketStationBoard assigns candidate stops to the at_station, arrival and departure buckets
// (docs/proposals/station-board.md §2.1), orders each bucket by board time and truncates it to
// limit. The result is at_station entries, then arrivals, then departures. A through train can
// yield both an arrival and a departure entry.
func BucketStationBoard(rows []StationBoardCandidate, now time.Time, horizon time.Duration, limit int) []model.StationBoardEntry {
	end := now.Add(horizon)
	inHorizon := func(t *time.Time) bool { return t != nil && t.After(now) && !t.After(end) }

	var atStation, arrivals, departures []model.StationBoardEntry
	for _, row := range rows {
		e := row.Entry
		hasArr := e.PlannedArrival != nil
		hasDep := e.PlannedDeparture != nil
		expArr := expectedOrPlanned(e.ExpectedArrival, e.PlannedArrival)
		expDep := expectedOrPlanned(e.ExpectedDeparture, e.PlannedDeparture)

		isAtStation := !e.IsCancelled && hasArr && !expArr.After(now) &&
			((hasDep && expDep.After(now)) || (!hasDep && expArr.After(now.Add(-terminusDwell))))

		if isAtStation {
			atStation = append(atStation, withBucket(e, BucketAtStation))
		}
		if hasArr && inHorizon(expArr) {
			arrivals = append(arrivals, withBucket(e, BucketArrival))
		}
		if hasDep && !isAtStation && inHorizon(expDep) {
			departures = append(departures, withBucket(e, BucketDeparture))
		}
	}

	entries := make([]model.StationBoardEntry, 0, 3*limit)
	entries = append(entries, sortAndTruncate(atStation, limit)...)
	entries = append(entries, sortAndTruncate(arrivals, limit)...)
	entries = append(entries, sortAndTruncate(departures, limit)...)
	return entries
}

func expectedOrPlanned(expected, planned *time.Time) *time.Time {
	if expected != nil {
		return expected
	}
	return planned
}

func withBucket(e model.StationBoardEntry, bucket string) model.StationBoardEntry {
	e.Bucket = bucket
	return e
}

// boardTime is the time an entry is ordered by: expected arrival for arrivals, expected
// departure otherwise (a terminus at_station entry falls back to its arrival).
func boardTime(e model.StationBoardEntry) time.Time {
	if e.Bucket != BucketArrival {
		if t := expectedOrPlanned(e.ExpectedDeparture, e.PlannedDeparture); t != nil {
			return *t
		}
	}
	if t := expectedOrPlanned(e.ExpectedArrival, e.PlannedArrival); t != nil {
		return *t
	}
	return time.Time{}
}

func sortAndTruncate(entries []model.StationBoardEntry, limit int) []model.StationBoardEntry {
	sort.SliceStable(entries, func(i, j int) bool {
		ti, tj := boardTime(entries[i]), boardTime(entries[j])
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return entries[i].OperationID < entries[j].OperationID
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}
