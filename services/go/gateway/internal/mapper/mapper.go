package mapper

import (
	"sort"
	"time"
)

func SortSchedules(items []SortableSchedule, sortBy string) {
	switch sortBy {
	case "arrival":
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].ArrivalTime < items[j].ArrivalTime
		})
	case "duration":
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].DurationMinutes < items[j].DurationMinutes
		})
	default:
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].DepartureTime < items[j].DepartureTime
		})
	}
}

type SortableSchedule struct {
	DepartureTime   string
	ArrivalTime     string
	DurationMinutes int
}

func DurationMinutesFromClock(departure, arrival *string, departureDay, arrivalDay int) *int {
	if departure == nil || arrival == nil {
		return nil
	}
	dep, err := time.Parse("15:04", *departure)
	if err != nil {
		return nil
	}
	arr, err := time.Parse("15:04", *arrival)
	if err != nil {
		return nil
	}

	depTotal := departureDay*24*60 + dep.Hour()*60 + dep.Minute()
	arrTotal := arrivalDay*24*60 + arr.Hour()*60 + arr.Minute()
	if arrTotal < depTotal {
		arrTotal += 24 * 60
	}

	duration := arrTotal - depTotal
	return &duration
}
