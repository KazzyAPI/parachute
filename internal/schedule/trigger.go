package schedule

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

type Trigger interface {
	First(now time.Time) time.Time
	Next(previous time.Time) time.Time
	Describe() string
}

type IntervalTrigger struct {
	interval time.Duration
}

func NewIntervalTrigger(interval time.Duration) *IntervalTrigger {
	return &IntervalTrigger{interval: interval}
}

func (t *IntervalTrigger) First(now time.Time) time.Time { return now }

func (t *IntervalTrigger) Next(previous time.Time) time.Time { return previous.Add(t.interval) }

func (t *IntervalTrigger) Describe() string {
	return fmt.Sprintf("every %d seconds", int(t.interval.Seconds()))
}

type CronTrigger struct {
	expression string
	schedule   cron.Schedule
	location   *time.Location
}

func NewCronTrigger(expression string, location *time.Location) (*CronTrigger, error) {
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", expression, err)
	}
	return &CronTrigger{expression: expression, schedule: schedule, location: location}, nil
}

func (t *CronTrigger) First(now time.Time) time.Time {
	return t.schedule.Next(now.In(t.location).Add(-time.Nanosecond))
}

func (t *CronTrigger) Next(previous time.Time) time.Time {
	return t.schedule.Next(previous.In(t.location))
}

func (t *CronTrigger) Describe() string {
	return fmt.Sprintf("cron %q (%s)", t.expression, t.location)
}
