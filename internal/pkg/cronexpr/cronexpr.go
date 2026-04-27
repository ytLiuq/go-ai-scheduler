package cronexpr

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const searchLimitMinutes = 5 * 366 * 24 * 60

type field struct {
	values map[int]struct{}
	any    bool
}

// Validate returns an error if expr is not a supported 5-field cron expression.
func Validate(expr string) error {
	_, err := parse(expr)
	return err
}

// NextAfter returns the first schedule strictly after now.
func NextAfter(now time.Time, expr string) (time.Time, error) {
	schedule, err := parse(expr)
	if err != nil {
		return time.Time{}, err
	}

	candidate := now.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < searchLimitMinutes; i++ {
		if schedule.matches(candidate) {
			return candidate, nil
		}
		candidate = candidate.Add(time.Minute)
	}

	return time.Time{}, fmt.Errorf("no matching time found for cron expression %q", expr)
}

type schedule struct {
	minute field
	hour   field
	day    field
	month  field
	week   field
}

func (s schedule) matches(t time.Time) bool {
	if !s.minute.contains(t.Minute()) || !s.hour.contains(t.Hour()) || !s.month.contains(int(t.Month())) {
		return false
	}

	dayMatch := s.day.contains(t.Day())
	weekMatch := s.week.contains(int(t.Weekday()))

	switch {
	case s.day.any && s.week.any:
		return true
	case s.day.any:
		return weekMatch
	case s.week.any:
		return dayMatch
	default:
		return dayMatch || weekMatch
	}
}

func (f field) contains(value int) bool {
	if f.any {
		return true
	}
	_, ok := f.values[value]
	return ok
}

func parse(expr string) (*schedule, error) {
	parts := strings.Fields(strings.TrimSpace(expr))
	if len(parts) != 5 {
		return nil, fmt.Errorf("cron expression must contain 5 fields")
	}

	minute, err := parseField(parts[0], 0, 59, false)
	if err != nil {
		return nil, fmt.Errorf("parse minute field: %w", err)
	}
	hour, err := parseField(parts[1], 0, 23, false)
	if err != nil {
		return nil, fmt.Errorf("parse hour field: %w", err)
	}
	day, err := parseField(parts[2], 1, 31, false)
	if err != nil {
		return nil, fmt.Errorf("parse day-of-month field: %w", err)
	}
	month, err := parseField(parts[3], 1, 12, false)
	if err != nil {
		return nil, fmt.Errorf("parse month field: %w", err)
	}
	week, err := parseField(parts[4], 0, 6, true)
	if err != nil {
		return nil, fmt.Errorf("parse day-of-week field: %w", err)
	}

	return &schedule{
		minute: minute,
		hour:   hour,
		day:    day,
		month:  month,
		week:   week,
	}, nil
}

func parseField(token string, min int, max int, allowSevenSunday bool) (field, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return field{}, fmt.Errorf("empty field")
	}
	if token == "*" {
		return field{any: true}, nil
	}

	result := field{values: make(map[int]struct{})}
	for _, part := range strings.Split(token, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return field{}, fmt.Errorf("empty list segment")
		}
		if err := addSegment(result.values, part, min, max, allowSevenSunday); err != nil {
			return field{}, err
		}
	}
	return result, nil
}

func addSegment(values map[int]struct{}, segment string, min int, max int, allowSevenSunday bool) error {
	step := 1
	base := segment
	if strings.Contains(segment, "/") {
		parts := strings.Split(segment, "/")
		if len(parts) != 2 {
			return fmt.Errorf("invalid step segment %q", segment)
		}
		base = parts[0]
		parsedStep, err := strconv.Atoi(parts[1])
		if err != nil || parsedStep <= 0 {
			return fmt.Errorf("invalid step value in %q", segment)
		}
		step = parsedStep
	}

	start, end, err := parseRange(base, min, max, allowSevenSunday)
	if err != nil {
		return err
	}
	for value := start; value <= end; value += step {
		normalized := normalizeValue(value, allowSevenSunday)
		if normalized < min || normalized > max {
			return fmt.Errorf("value %d out of range", value)
		}
		values[normalized] = struct{}{}
	}
	return nil
}

func parseRange(base string, min int, max int, allowSevenSunday bool) (int, int, error) {
	if base == "*" {
		return min, max, nil
	}

	if strings.Contains(base, "-") {
		parts := strings.Split(base, "-")
		if len(parts) != 2 {
			return 0, 0, fmt.Errorf("invalid range %q", base)
		}
		start, err := parseValue(parts[0], min, max, allowSevenSunday)
		if err != nil {
			return 0, 0, err
		}
		end, err := parseValue(parts[1], min, max, allowSevenSunday)
		if err != nil {
			return 0, 0, err
		}
		if start > end {
			return 0, 0, fmt.Errorf("range start greater than end in %q", base)
		}
		return start, end, nil
	}

	value, err := parseValue(base, min, max, allowSevenSunday)
	if err != nil {
		return 0, 0, err
	}
	return value, value, nil
}

func parseValue(text string, min int, max int, allowSevenSunday bool) (int, error) {
	value, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q", text)
	}
	normalized := normalizeValue(value, allowSevenSunday)
	if normalized < min || normalized > max {
		return 0, fmt.Errorf("value %d out of range [%d,%d]", value, min, max)
	}
	return value, nil
}

func normalizeValue(value int, allowSevenSunday bool) int {
	if allowSevenSunday && value == 7 {
		return 0
	}
	return value
}
