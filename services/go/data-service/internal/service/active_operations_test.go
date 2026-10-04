package service

import (
	"testing"
	"time"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
	"github.com/pociag-do-predykcji/services/go/data-service/internal/position"
)

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptrTime(s string) *time.Time { t := mustTime(s); return &t }
func ptrFloat(v float64) *float64 { return &v }
func ptrStr(v string) *string     { return &v }

// candidate is a two-stop operation: departs station 1 at 10:00 (confirmed, 2 min late), arrives
// at station 2 at 11:00.
func candidate(id int64, status string) ActiveOperationCandidate {
	return ActiveOperationCandidate{
		OperationID:   id,
		ScheduleID:    2026,
		OrderID:       int(id),
		OperatingDate: mustTime("2026-10-02T00:00:00Z"),
		TrainStatus:   status,
		UpdatedAt:     mustTime("2026-10-02T10:25:00Z"),
		RouteName:     ptrStr("IC 123"),
		Stops: []position.Stop{
			{
				StationExternalID: 1, StationName: ptrStr("A"), SequenceNumber: 1,
				ActualDeparture: ptrTime("2026-10-02T10:00:00Z"), DepartureDelayMinutes: func() *int { v := 2; return &v }(),
				IsConfirmed: true, Latitude: ptrFloat(52), Longitude: ptrFloat(21),
			},
			{
				StationExternalID: 2, SequenceNumber: 2,
				ActualArrival: ptrTime("2026-10-02T11:00:00Z"), Latitude: ptrFloat(52), Longitude: ptrFloat(22),
			},
		},
	}
}

func TestActiveTrains(t *testing.T) {
	t.Parallel()

	at := mustTime("2026-10-02T10:30:00Z")
	latest := mustTime("2026-10-02T10:25:00Z")
	unconfirmed := candidate(4, "P")
	unconfirmed.Stops[0].IsConfirmed = false
	cands := []ActiveOperationCandidate{
		candidate(1, "P"),
		candidate(2, "Q"), // excluded
		candidate(3, "S"), // past its departure: active
		unconfirmed,
	}

	data, total := activeTrains(cands, at, latest, 2)
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(data) != 2 || data[0].OperationID != 1 || data[1].OperationID != 3 {
		t.Fatalf("data ids = %v, want [1 3]", ids(data))
	}

	got := data[0]
	if got.Phase != position.PhaseEnRoute || got.TrainStatus != "P" || got.OperatingDate != "2026-10-02" {
		t.Errorf("phase/status/date = %s/%s/%s", got.Phase, got.TrainStatus, got.OperatingDate)
	}
	if got.Origin == nil || got.Origin.StationExternalID != 1 || got.Destination == nil || got.Destination.StationExternalID != 2 {
		t.Errorf("origin/destination = %+v / %+v", got.Origin, got.Destination)
	}
	if got.PreviousStop == nil || !got.PreviousStop.Time.Equal(mustTime("2026-10-02T10:00:00Z")) {
		t.Errorf("previous_stop = %+v, want station 1 at its departure", got.PreviousStop)
	}
	if got.NextStop == nil || !got.NextStop.Time.Equal(mustTime("2026-10-02T11:00:00Z")) || got.NextStop.StationName != nil {
		t.Errorf("next_stop = %+v, want station 2 at its arrival, no name", got.NextStop)
	}
	if got.DelayMinutes == nil || *got.DelayMinutes != 2 || got.LastConfirmedStop == nil {
		t.Errorf("delay/last_confirmed = %v / %+v", got.DelayMinutes, got.LastConfirmedStop)
	}
	if got.Position == nil || got.Position.Method != position.MethodInterpolated || got.Position.Progress != 0.5 || got.Position.Longitude != 21.5 {
		t.Errorf("position = %+v", got.Position)
	}
	if got.Confidence != position.ConfidenceMedium {
		t.Errorf("confidence = %s, want medium (confirmation 30 min old)", got.Confidence)
	}

	// Without a limit cut the unconfirmed train has no delay or last confirmed stop.
	all, _ := activeTrains(cands, at, latest, 10)
	if len(all) != 3 || all[2].DelayMinutes != nil || all[2].LastConfirmedStop != nil {
		t.Errorf("unconfirmed train = %+v", all[len(all)-1])
	}
}

func TestActiveTrains_Empty(t *testing.T) {
	t.Parallel()
	data, total := activeTrains(nil, mustTime("2026-10-02T10:30:00Z"), time.Time{}, 10)
	if data == nil || len(data) != 0 || total != 0 {
		t.Errorf("data = %v, total = %d, want empty non-nil slice and 0", data, total)
	}
}

func ids(data []model.ActiveTrain) []int64 {
	out := make([]int64, len(data))
	for i, d := range data {
		out[i] = d.OperationID
	}
	return out
}
