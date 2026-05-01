package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronSpec represents a parsed cron expression with 5 fields
type CronSpec struct {
	Minutes []int // minute (0-59)
	Hours   []int // hour (0-23)
	Days    []int // day of month (1-31)
	Months  []int // month (1-12)
	Dow     []int // day of week (0-6)
}

// ParseCron parses a standard 5-field cron expression
// Fields: minute, hour, day-of-month, month, day-of-week
// Special: @every N{unit} (e.g., @every 30m)
func ParseCron(expr string) (*CronSpec, error) {
	// Handle @every syntax
	if strings.HasPrefix(expr, "@every ") {
		spec, err := parseEvery(expr[7:])
		if err != nil {
			return nil, err
		}
		return spec, nil
	}

	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: need 5 fields, got %d", len(fields))
	}

	spec := &CronSpec{}

	parsed, err := parseCronField(fields[0], 0)
	if err != nil {
		return nil, err
	}
	spec.Minutes = parsed

	parsed, err = parseCronField(fields[1], 1)
	if err != nil {
		return nil, err
	}
	spec.Hours = parsed

	parsed, err = parseCronField(fields[2], 2)
	if err != nil {
		return nil, err
	}
	spec.Days = parsed

	parsed, err = parseCronField(fields[3], 3)
	if err != nil {
		return nil, err
	}
	spec.Months = parsed

	parsed, err = parseCronField(fields[4], 4)
	if err != nil {
		return nil, err
	}
	spec.Dow = parsed

	return spec, nil
}

// parseEvery parses @every N duration
func parseEvery(expr string) (*CronSpec, error) {
	expr = strings.ToLower(strings.TrimSpace(expr))

	var duration time.Duration
	var num int
	var unit string

	_, err := fmt.Sscanf(expr, "%d%s", &num, &unit)
	if err != nil {
		return nil, fmt.Errorf("@every parse: %w", err)
	}

	switch unit {
	case "s", "sec", "seconds":
		duration = time.Duration(num) * time.Second
	case "m", "min", "minutes":
		duration = time.Duration(num) * time.Minute
	case "h", "hour", "hours":
		duration = time.Duration(num) * time.Hour
	case "d", "day", "days":
		duration = time.Duration(num) * 24 * time.Hour
	default:
		return nil, fmt.Errorf("unknown unit: %s", unit)
	}

	// Convert to cron-like representation (every N minutes = */N)
	if duration >= time.Hour {
		return &CronSpec{
			Minutes: []int{0},
			Hours:   []int{0},
			Days:    []int{1},
			Months:  []int{0},
			Dow:     []int{num},
		}, nil
	}

	minutes := int(duration / time.Minute)
	return &CronSpec{
		Minutes: []int{0},
		Hours:   []int{0},
		Days:    []int{0},
		Months:  []int{0},
		Dow:     []int{minutes},
	}, nil
}

// parseCronField parses a single cron field
func parseCronField(field string, pos int) ([]int, error) {
	// Handle lists
	if strings.Contains(field, ",") {
		var result []int
		for _, f := range strings.Split(field, ",") {
			parsed, err := parseCronField(f, pos)
			if err != nil {
				return nil, err
			}
			result = append(result, parsed...)
		}
		return result, nil
	}

	// Handle ranges
	if strings.Contains(field, "-") {
		parts := strings.Split(field, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range: %s", field)
		}
		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, err
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		var result []int
		for i := start; i <= end; i++ {
			result = append(result, i)
		}
		return result, nil
	}

	// Handle */N (step)
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil {
			return nil, err
		}

		max := getFieldMax(pos)
		var result []int
		for i := 0; i <= max; i += step {
			result = append(result, i)
		}
		return result, nil
	}

	// Single value
	val, err := strconv.Atoi(field)
	if err != nil {
		return nil, err
	}

	return []int{val}, nil
}

// getFieldMax returns the max value for a field position
func getFieldMax(pos int) int {
	switch pos {
	case 0:
		return 59 // minute
	case 1:
		return 23 // hour
	case 2:
		return 31 // day
	case 3:
		return 12 // month
	case 4:
		return 6 // day of week
	default:
		return 0
	}
}

// NextCronTime calculates the next time a cron expression should fire
func NextCronTime(expr string, from time.Time) (time.Time, error) {
	fields, err := ParseCron(expr)
	if err != nil {
		return time.Time{}, err
	}

	// Simple next-run calculation
	for h := 0; h < 24*365; h++ {
		check := from.Add(time.Duration(h) * time.Hour)

		// Check hour
		if !contains(fields.Hours, check.Hour()) {
			continue
		}

		// Check minute
		if !contains(fields.Minutes, check.Minute()) {
			continue
		}

		// Check day of month
		if !contains(fields.Days, check.Day()) {
			continue
		}

		// Check month
		if !contains(fields.Months, int(check.Month())) {
			continue
		}

		// Check day of week
		if !contains(fields.Dow, int(check.Weekday())) {
			continue
		}

		return check, nil
	}

	return time.Time{}, fmt.Errorf("no next run found")
}

func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}
