package service

import (
	"context"
	"testing"
	"time"

	"github.com/pociag-do-predykcji/services/go/data-service/internal/model"
)

var boardRef = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

// at returns boardRef shifted by the given number of minutes.
func at(minutes int) *time.Time {
	t := boardRef.Add(time.Duration(minutes) * time.Minute)
	return &t
}

type stop struct {
	op                       int64
	plannedArr, plannedDep   *time.Time
	expectedArr, expectedDep *time.Time
	cancelled                bool
}

func (s stop) candidate() StationBoardCandidate {
	expArr, expDep := s.expectedArr, s.expectedDep
	if expArr == nil {
		expArr = s.plannedArr
	}
	if expDep == nil {
		expDep = s.plannedDep
	}
	return StationBoardCandidate{Entry: model.StationBoardEntry{
		OperationID:       s.op,
		PlannedArrival:    s.plannedArr,
		PlannedDeparture:  s.plannedDep,
		ExpectedArrival:   expArr,
		ExpectedDeparture: expDep,
		IsCancelled:       s.cancelled,
	}}
}

type boardItem struct {
	op     int64
	bucket string
}

func TestBucketStationBoard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		now     time.Time
		horizon time.Duration
		limit   int
		stops   []stop
		want    []boardItem
	}{
		{
			name:  "origin departs only",
			stops: []stop{{op: 1, plannedDep: at(30)}},
			want:  []boardItem{{1, BucketDeparture}},
		},
		{
			name:  "terminus arrives only",
			stops: []stop{{op: 1, plannedArr: at(20)}},
			want:  []boardItem{{1, BucketArrival}},
		},
		{
			name: "terminus at station for 5 minutes after arrival",
			stops: []stop{
				{op: 1, plannedArr: at(-3)},
				{op: 2, plannedArr: at(-5)},
				{op: 3, plannedArr: at(-10)},
			},
			want: []boardItem{{1, BucketAtStation}},
		},
		{
			name:  "through train in both arrivals and departures",
			stops: []stop{{op: 1, plannedArr: at(20), plannedDep: at(22)}},
			want:  []boardItem{{1, BucketArrival}, {1, BucketDeparture}},
		},
		{
			name:  "standing at platform only at station",
			stops: []stop{{op: 1, plannedArr: at(-2), plannedDep: at(3)}},
			want:  []boardItem{{1, BucketAtStation}},
		},
		{
			name:  "departing exactly now is no longer at station",
			stops: []stop{{op: 1, plannedArr: at(-2), plannedDep: at(0)}},
			want:  nil,
		},
		{
			name: "delayed into horizon",
			stops: []stop{{
				op:          1,
				plannedArr:  at(-30),
				plannedDep:  at(-28),
				expectedArr: at(10),
				expectedDep: at(12),
			}},
			want: []boardItem{{1, BucketArrival}, {1, BucketDeparture}},
		},
		{
			name:    "delayed out of horizon",
			horizon: 2 * time.Hour,
			stops: []stop{{
				op:          1,
				plannedArr:  at(100),
				plannedDep:  at(110),
				expectedArr: at(125),
				expectedDep: at(130),
			}},
			want: nil,
		},
		{
			name: "cancelled stop never at station but stays in departures",
			stops: []stop{
				{op: 1, plannedArr: at(-2), plannedDep: at(5), cancelled: true},
				{op: 2, plannedArr: at(-1), cancelled: true},
			},
			want: []boardItem{{1, BucketDeparture}},
		},
		{
			name:  "cancelled stop stays in arrivals",
			stops: []stop{{op: 1, plannedArr: at(15), cancelled: true}},
			want:  []boardItem{{1, BucketArrival}},
		},
		{
			name: "midnight crossing",
			// 23:50 Europe/Warsaw; the stops are after local midnight.
			now:     time.Date(2026, 9, 29, 21, 50, 0, 0, time.UTC),
			horizon: time.Hour,
			stops: []stop{
				{
					op:         1,
					plannedArr: ptr(time.Date(2026, 9, 29, 22, 10, 0, 0, time.UTC)),
					plannedDep: ptr(time.Date(2026, 9, 29, 22, 12, 0, 0, time.UTC)),
				},
				{
					op:          2,
					plannedDep:  ptr(time.Date(2026, 9, 29, 21, 40, 0, 0, time.UTC)),
					expectedDep: ptr(time.Date(2026, 9, 29, 22, 5, 0, 0, time.UTC)),
				},
			},
			want: []boardItem{{1, BucketArrival}, {2, BucketDeparture}, {1, BucketDeparture}},
		},
		{
			name:  "ordered by board time and truncated to limit",
			limit: 2,
			stops: []stop{
				{op: 1, plannedDep: at(40)},
				{op: 2, plannedDep: at(10)},
				{op: 3, plannedDep: at(30)},
				{op: 4, plannedArr: at(50)},
				{op: 5, plannedArr: at(5)},
				{op: 6, plannedArr: at(25)},
				{op: 7, plannedArr: at(-1), plannedDep: at(20)},
				{op: 8, plannedArr: at(-2), plannedDep: at(8)},
				{op: 9, plannedArr: at(-3), plannedDep: at(1)},
			},
			want: []boardItem{
				{9, BucketAtStation}, {8, BucketAtStation},
				{5, BucketArrival}, {6, BucketArrival},
				{2, BucketDeparture}, {3, BucketDeparture},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			now := tt.now
			if now.IsZero() {
				now = boardRef
			}
			horizon := tt.horizon
			if horizon == 0 {
				horizon = 12 * time.Hour
			}
			limit := tt.limit
			if limit == 0 {
				limit = 10
			}
			rows := make([]StationBoardCandidate, 0, len(tt.stops))
			for _, s := range tt.stops {
				rows = append(rows, s.candidate())
			}

			got := BucketStationBoard(rows, now, horizon, limit)

			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries %v, want %d %v", len(got), items(got), len(tt.want), tt.want)
			}
			for i, w := range tt.want {
				if got[i].OperationID != w.op || got[i].Bucket != w.bucket {
					t.Fatalf("entries = %v, want %v", items(got), tt.want)
				}
			}
		})
	}
}

func TestGetStationBoard_DataAsOf(t *testing.T) {
	t.Parallel()

	older := boardRef.Add(-20 * time.Minute)
	newer := boardRef.Add(-5 * time.Minute)
	repo := &fakeBoardRepo{rows: []StationBoardCandidate{
		{Entry: model.StationBoardEntry{OperationID: 1, PlannedDeparture: at(10), ExpectedDeparture: at(10)}, UpdatedAt: older},
		// Past stop, not on the board: its updated_at must not count.
		{Entry: model.StationBoardEntry{OperationID: 2, PlannedDeparture: at(-60), ExpectedDeparture: at(-60)}, UpdatedAt: newer},
	}}

	got, err := New(repo).GetStationBoard(context.Background(), 33506, boardRef, 12*time.Hour, 6*time.Hour, 10)
	if err != nil {
		t.Fatalf("GetStationBoard: %v", err)
	}
	if got.DataAsOf == nil || !got.DataAsOf.Equal(older) {
		t.Errorf("DataAsOf = %v, want %v", got.DataAsOf, older)
	}
	if got.HorizonMinutes != 720 || got.Limit != 10 || !got.At.Equal(boardRef) || got.StationExternalID != 33506 {
		t.Errorf("unexpected response header fields: %+v", got)
	}

	empty, err := New(&fakeBoardRepo{}).GetStationBoard(context.Background(), 1, boardRef, time.Hour, 0, 10)
	if err != nil {
		t.Fatalf("GetStationBoard (empty): %v", err)
	}
	if empty.DataAsOf != nil || empty.Entries == nil || len(empty.Entries) != 0 {
		t.Errorf("empty board = %+v, want no data_as_of and empty entries", empty)
	}
}

type fakeBoardRepo struct {
	Repository
	rows []StationBoardCandidate
}

func (f *fakeBoardRepo) QueryStationBoardCandidates(_ context.Context, _ int, _ time.Time, _, _ time.Duration) ([]StationBoardCandidate, error) {
	return f.rows, nil
}

func ptr(t time.Time) *time.Time { return &t }

func items(entries []model.StationBoardEntry) []boardItem {
	out := make([]boardItem, 0, len(entries))
	for _, e := range entries {
		out = append(out, boardItem{e.OperationID, e.Bucket})
	}
	return out
}
