package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule holds the set of allowed values for each cron field.
type Schedule struct {
	minutes  [60]bool
	hours    [24]bool
	days     [32]bool
	months   [13]bool
	weekdays [7]bool
}

// Parse parses a 5-field cron expression (minute hour day month weekday).
func Parse(expr string) (*Schedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d", len(fields))
	}
	s := &Schedule{}
	parsers := []struct {
		field string
		min   int
		max   int
		set   func(int)
	}{
		{fields[0], 0, 59, func(v int) { s.minutes[v] = true }},
		{fields[1], 0, 23, func(v int) { s.hours[v] = true }},
		{fields[2], 1, 31, func(v int) { s.days[v] = true }},
		{fields[3], 1, 12, func(v int) { s.months[v] = true }},
		{fields[4], 0, 6, func(v int) { s.weekdays[v] = true }},
	}
	for _, p := range parsers {
		vals, err := parseField(p.field, p.min, p.max)
		if err != nil {
			return nil, err
		}
		for _, v := range vals {
			p.set(v)
		}
	}
	return s, nil
}

func parseField(field string, min, max int) ([]int, error) {
	var out []int
	for _, part := range strings.Split(field, ",") {
		vals, err := parsePart(part, min, max)
		if err != nil {
			return nil, err
		}
		out = append(out, vals...)
	}
	return out, nil
}

func parsePart(part string, min, max int) ([]int, error) {
	// */step
	if strings.HasPrefix(part, "*/") {
		step, err := strconv.Atoi(part[2:])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("cron: invalid step in %q", part)
		}
		var out []int
		for i := min; i <= max; i += step {
			out = append(out, i)
		}
		return out, nil
	}
	// *
	if part == "*" {
		var out []int
		for i := min; i <= max; i++ {
			out = append(out, i)
		}
		return out, nil
	}
	// a-b or a-b/step
	if idx := strings.Index(part, "-"); idx != -1 {
		rangeStep := strings.SplitN(part, "/", 2)
		bounds := strings.SplitN(rangeStep[0], "-", 2)
		a, err1 := strconv.Atoi(bounds[0])
		b, err2 := strconv.Atoi(bounds[1])
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("cron: invalid range %q", part)
		}
		step := 1
		if len(rangeStep) == 2 {
			s, err := strconv.Atoi(rangeStep[1])
			if err != nil || s <= 0 {
				return nil, fmt.Errorf("cron: invalid step in range %q", part)
			}
			step = s
		}
		var out []int
		for i := a; i <= b; i += step {
			out = append(out, i)
		}
		return out, nil
	}
	// literal
	v, err := strconv.Atoi(part)
	if err != nil {
		return nil, fmt.Errorf("cron: invalid value %q", part)
	}
	if v < min || v > max {
		return nil, fmt.Errorf("cron: value %d out of range [%d-%d]", v, min, max)
	}
	return []int{v}, nil
}

// Next returns the next time after t that matches the schedule.
func (s *Schedule) Next(t time.Time) time.Time {
	// Advance to the start of the next minute.
	t = t.Add(time.Minute - time.Duration(t.Second())*time.Second - time.Duration(t.Nanosecond()))
	deadline := t.Add(4 * 365 * 24 * time.Hour)
	for t.Before(deadline) {
		if !s.months[int(t.Month())] {
			t = t.AddDate(0, 1, -t.Day()+1)
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
			continue
		}
		if !s.days[t.Day()] || !s.weekdays[int(t.Weekday())] {
			t = t.AddDate(0, 0, 1)
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
			continue
		}
		if !s.hours[t.Hour()] {
			t = t.Add(time.Hour)
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
			continue
		}
		if !s.minutes[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}
	return time.Time{}
}
