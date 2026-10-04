// Package servicetest provides an in-memory service.Repository for tests.
package servicetest

import (
	"context"
	"time"

	"github.com/pociag-do-predykcji/services/go/api/internal/service"
)

// Repository returns its fields. Err, when set, fails every call; a nil Station, Run or
// Schedule is service.ErrNotFound. Calls record their filter arguments.
type Repository struct {
	Err error

	Stations    []service.Station
	Station     *service.Station
	BoardStops  []service.BoardStop
	Snapshot    time.Time
	ActiveRuns  []service.Run
	Run         *service.Run
	Connections []service.Connection
	Schedule    *service.TrainSchedule
	Disruptions []service.Disruption
	Statistics  service.DayStatistics
	Freshness   service.Freshness

	BoardWindow     [2]time.Time
	ActiveSeenSince time.Time
	StatsDate       time.Time
	StatsSeenSince  time.Time
	Query           service.ConnectionQuery
}

func (r *Repository) Ping(context.Context) error { return r.Err }

func (r *Repository) SearchStations(_ context.Context, _ string, limit int) ([]service.Station, error) {
	return r.Stations[:min(limit, len(r.Stations))], r.Err
}

func (r *Repository) ListMappedStations(context.Context) ([]service.Station, error) {
	return r.Stations, r.Err
}

func (r *Repository) GetStation(context.Context, int) (*service.Station, error) {
	return found(r.Station, r.Err)
}

func (r *Repository) ListBoardStops(_ context.Context, _ int, from, to time.Time) ([]service.BoardStop, error) {
	r.BoardWindow = [2]time.Time{from, to}
	return r.BoardStops, r.Err
}

func (r *Repository) LatestSnapshot(context.Context) (time.Time, error) { return r.Snapshot, r.Err }

func (r *Repository) ListActiveRuns(_ context.Context, _, seenSince time.Time, _ []time.Time) ([]service.Run, error) {
	r.ActiveSeenSince = seenSince
	return r.ActiveRuns, r.Err
}

func (r *Repository) GetRun(context.Context, int64) (*service.Run, error) { return found(r.Run, r.Err) }

func (r *Repository) SearchConnections(_ context.Context, q service.ConnectionQuery) ([]service.Connection, error) {
	r.Query = q
	return r.Connections, r.Err
}

func (r *Repository) GetTrainSchedule(context.Context, int64) (*service.TrainSchedule, error) {
	return found(r.Schedule, r.Err)
}

func (r *Repository) ListDisruptions(_ context.Context, limit, offset int) ([]service.Disruption, int, error) {
	page := r.Disruptions[min(offset, len(r.Disruptions)):min(offset+limit, len(r.Disruptions))]
	return page, len(r.Disruptions), r.Err
}

func (r *Repository) GetDayStatistics(_ context.Context, date, seenSince time.Time) (*service.DayStatistics, error) {
	r.StatsDate, r.StatsSeenSince = date, seenSince
	return &r.Statistics, r.Err
}

func (r *Repository) GetFreshness(context.Context) (*service.Freshness, error) {
	return &r.Freshness, r.Err
}

func found[T any](v *T, err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, service.ErrNotFound
	}
	return v, nil
}
