package trainutil

import (
	"strings"
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

func FormatClock(ts *time.Time) *string {
	if ts == nil {
		return nil
	}
	clock := ts.Format("15:04")
	return &clock
}
