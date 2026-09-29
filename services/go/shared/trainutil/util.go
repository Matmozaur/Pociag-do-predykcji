package trainutil

import (
	"strings"
	"sync"
	"time"
)

func StatusLabel(code string) string {
	switch code {
	case "S":
		return "not_started"
	case "P":
		return "in_progress"
	case "C":
		return "completed"
	case "X":
		return "cancelled"
	case "Q":
		return "partial_cancelled"
	default:
		return "not_started"
	}
}

func SeverityByAffectedRoutes(affectedRoutes int) string {
	if affectedRoutes >= 10 {
		return "high"
	}
	if affectedRoutes >= 3 {
		return "medium"
	}
	return "low"
}

func ParseCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

// warsawLocation is loaded lazily so a binary's embedded time/tzdata is registered first.
var warsawLocation = sync.OnceValue(func() *time.Location {
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		return time.UTC
	}
	return loc
})

// FormatClock formats ts as an HH:MM wall-clock time in Europe/Warsaw.
func FormatClock(ts *time.Time) *string {
	if ts == nil {
		return nil
	}
	clock := ts.In(warsawLocation()).Format("15:04")
	return &clock
}
