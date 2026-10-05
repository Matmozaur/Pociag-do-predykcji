package service

import (
	"context"
	"fmt"
	"time"

	"github.com/pociag-do-predykcji/services/go/api/internal/model"
	"github.com/pociag-do-predykcji/services/go/api/internal/position"
)

// Disruptions lists the current disruption snapshot, most affected trains first.
func (s *Service) Disruptions(ctx context.Context, limit, offset int) (*model.DisruptionListView, error) {
	disruptions, total, err := s.repo.ListDisruptions(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list disruptions: %w", err)
	}

	data := make([]model.DisruptionView, 0, len(disruptions))
	for _, d := range disruptions {
		data = append(data, model.DisruptionView{
			ID:                  d.ID,
			StartStation:        d.StartStation,
			EndStation:          d.EndStation,
			Message:             d.Message,
			DateFrom:            dateString(d.DateFrom),
			DateTo:              dateString(d.DateTo),
			AffectedRoutesCount: d.AffectedTrains,
			Severity:            severity(d.AffectedTrains),
		})
	}
	return &model.DisruptionListView{Data: data, Pagination: model.NewPagination(total, limit, offset)}, nil
}

// severity grades a disruption by the number of trains it affects.
func severity(affectedTrains int) string {
	switch {
	case affectedTrains >= 10:
		return "high"
	case affectedTrains >= 3:
		return "medium"
	}
	return "low"
}

func dateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.DateOnly)
	return &s
}

// DashboardOverview summarises today's operating date (Europe/Warsaw).
func (s *Service) DashboardOverview(ctx context.Context) (*model.DashboardOverview, error) {
	freshness, err := s.repo.GetFreshness(ctx)
	if err != nil {
		return nil, fmt.Errorf("dashboard overview: %w", err)
	}
	seenSince := time.Time{}
	if freshness.OperationsUpdatedAt != nil {
		seenSince = freshness.OperationsUpdatedAt.Add(-position.SnapshotWindow)
	}
	today := warsawDate(s.now())
	stats, err := s.repo.GetDayStatistics(ctx, today, seenSince)
	if err != nil {
		return nil, fmt.Errorf("dashboard overview: %w", err)
	}

	overview := &model.DashboardOverview{
		Statistics: model.DashboardStatistics{
			Date:            today.Format(time.DateOnly),
			TotalTrains:     stats.Total,
			InProgress:      stats.InProgress,
			Completed:       stats.Completed,
			Cancelled:       stats.Cancelled,
			AvgDelayMinutes: stats.AvgDelay,
		},
		DisruptionsActive: freshness.Disruptions,
		DataFreshness: model.DataFreshness{
			SchedulesLastUpdated:  freshness.SchedulesUpdatedAt,
			OperationsLastUpdated: freshness.OperationsUpdatedAt,
		},
	}
	if stats.Started > 0 {
		pct := 100 * float64(stats.OnTime) / float64(stats.Started)
		overview.Statistics.OnTimePercentage = &pct
	}
	return overview, nil
}
