package trainutil

import (
	"testing"
	"time"
)

func TestFormatClock(t *testing.T) {
	t.Parallel()

	summer := time.Date(2026, 7, 1, 8, 30, 0, 0, time.UTC)
	winter := time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		name string
		ts   *time.Time
		want *string
	}{
		{name: "summer CEST", ts: &summer, want: strPtr("10:30")},
		{name: "winter CET", ts: &winter, want: strPtr("10:30")},
		{name: "nil", ts: nil, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FormatClock(tt.ts)
			if (got == nil) != (tt.want == nil) {
				t.Fatalf("FormatClock() = %v, want %v", got, tt.want)
			}
			if got != nil && *got != *tt.want {
				t.Errorf("FormatClock() = %q, want %q", *got, *tt.want)
			}
		})
	}
}

func strPtr(v string) *string {
	return &v
}
