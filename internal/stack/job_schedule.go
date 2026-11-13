package stack

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// A job is a service that runs to completion each time a job is queued for it,
// one job at a time on the machine. It is declared with x-bay.job; up and start
// leave it alone, and it may declare a cron schedule and a time limit.

const (
	// DefaultJobTimeout is the limit on one run when x-bay.timeout is unset.
	DefaultJobTimeout = time.Hour
	// MaxJobTimeout is the longest limit a job may declare.
	MaxJobTimeout = 24 * time.Hour
)

func decodeJob(s *Service, b *serviceSettings) error {
	if !b.Job {
		if b.Schedule != "" || b.Timeout != "" {
			return errors.New("x-bay.schedule and x-bay.timeout apply to a job; set x-bay.job: true")
		}
		return nil
	}
	if len(s.Domains) > 0 {
		return errors.New("a job serves no domain: it runs to completion and is not routed to")
	}
	s.Job = true
	s.JobTimeout = DefaultJobTimeout
	if b.Timeout != "" {
		d, err := time.ParseDuration(b.Timeout)
		if err != nil || d <= 0 || d > MaxJobTimeout {
			return fmt.Errorf("x-bay.timeout %q must be a duration from 1s to %s, such as 30m", b.Timeout, MaxJobTimeout)
		}
		s.JobTimeout = d
	}
	if b.Schedule != "" {
		if _, err := ParseSchedule(b.Schedule); err != nil {
			return fmt.Errorf("x-bay.schedule %q: %w", b.Schedule, err)
		}
		s.Schedule = b.Schedule
	}
	return nil
}

// Schedule is a cron schedule: minute, hour, day of month, month and day of
// week, read in the machine's local time.
type Schedule struct {
	minute, hour, dom, month, dow [64]bool
	// domAny and dowAny record a "*" day field: cron runs on a day when
	// either day field matches, unless one of them is "*".
	domAny, dowAny bool
}

var scheduleAliases = map[string]string{
	"@hourly":   "0 * * * *",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@weekly":   "0 0 * * 0",
	"@monthly":  "0 0 1 * *",
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
}

// ParseSchedule reads five cron fields, each "*", a number, a range a-b, a
// list a,b and a step */n or a-b/n, or one of @hourly, @daily, @weekly,
// @monthly and @yearly.
func ParseSchedule(spec string) (Schedule, error) {
	if alias, ok := scheduleAliases[strings.TrimSpace(spec)]; ok {
		spec = alias
	}
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return Schedule{}, errors.New("needs five fields: minute hour day-of-month month day-of-week, or @hourly, @daily, @weekly, @monthly")
	}
	var sc Schedule
	parts := []struct {
		set      *[64]bool
		min, max int
		name     string
	}{
		{&sc.minute, 0, 59, "minute"}, {&sc.hour, 0, 23, "hour"}, {&sc.dom, 1, 31, "day of month"},
		{&sc.month, 1, 12, "month"}, {&sc.dow, 0, 7, "day of week"},
	}
	for i, p := range parts {
		if err := parseField(fields[i], p.min, p.max, p.set); err != nil {
			return Schedule{}, fmt.Errorf("%s: %w", p.name, err)
		}
	}
	if sc.dow[7] {
		sc.dow[0] = true
	}
	sc.domAny, sc.dowAny = fields[2] == "*", fields[4] == "*"
	return sc, nil
}

func parseField(field string, min, max int, set *[64]bool) error {
	for _, item := range strings.Split(field, ",") {
		rng, stepText, hasStep := strings.Cut(item, "/")
		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepText)
			if err != nil || n < 1 {
				return fmt.Errorf("step %q is not a positive number", stepText)
			}
			step = n
		}
		lo, hi := min, max
		if rng != "*" {
			a, b, isRange := strings.Cut(rng, "-")
			var err error
			if lo, err = strconv.Atoi(a); err != nil {
				return fmt.Errorf("%q is not a number", a)
			}
			hi = lo
			if isRange {
				if hi, err = strconv.Atoi(b); err != nil {
					return fmt.Errorf("%q is not a number", b)
				}
			} else if hasStep {
				hi = max
			}
		}
		if lo < min || hi > max || lo > hi {
			return fmt.Errorf("%q is outside %d-%d", item, min, max)
		}
		for v := lo; v <= hi; v += step {
			set[v] = true
		}
	}
	return nil
}

// Matches reports whether the schedule runs in the minute holding t.
func (sc Schedule) Matches(t time.Time) bool {
	if !sc.minute[t.Minute()] || !sc.hour[t.Hour()] || !sc.month[int(t.Month())] {
		return false
	}
	dom, dow := sc.dom[t.Day()], sc.dow[int(t.Weekday())]
	switch {
	case sc.domAny && sc.dowAny:
		return true
	case sc.domAny:
		return dow
	case sc.dowAny:
		return dom
	default:
		return dom || dow
	}
}

// Next returns the first minute after t the schedule runs in, within a year.
func (sc Schedule) Next(t time.Time) (time.Time, bool) {
	next := t.Truncate(time.Minute).Add(time.Minute)
	for end := next.AddDate(1, 0, 1); next.Before(end); next = next.Add(time.Minute) {
		if sc.Matches(next) {
			return next, true
		}
	}
	return time.Time{}, false
}
