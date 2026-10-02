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

func TestDisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                                       string
		routeName, category, national, trainNumber *string
		want                                       string
	}{
		{name: "route name wins", routeName: strPtr("Pieniny"), category: strPtr("IC"), national: strPtr("3810"), trainNumber: strPtr("38100"), want: "Pieniny"},
		{name: "route name trimmed", routeName: strPtr("  Pieniny "), want: "Pieniny"},
		{name: "category and national number", routeName: strPtr("   "), category: strPtr("IC"), national: strPtr("3810"), trainNumber: strPtr("38100"), want: "IC 3810"},
		{name: "national number without category", national: strPtr("3810"), trainNumber: strPtr("38100"), want: "Pociąg 3810"},
		{name: "category without national number", category: strPtr("REG"), trainNumber: strPtr("91234"), want: "Pociąg 91234"},
		{name: "train number", category: strPtr(""), national: strPtr(" "), trainNumber: strPtr("91234"), want: "Pociąg 91234"},
		{name: "schedule and order fallback", trainNumber: strPtr(""), want: "Pociąg 2026/123"},
		{name: "all nil", want: "Pociąg 2026/123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := DisplayName(tt.routeName, tt.category, tt.national, tt.trainNumber, 2026, 123)
			if got != tt.want {
				t.Errorf("DisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}
