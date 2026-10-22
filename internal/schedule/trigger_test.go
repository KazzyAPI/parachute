package schedule

import (
	"testing"
	"time"
)

func TestCronTriggerUsesTheConfiguredZone(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	trigger, err := NewCronTrigger("0 0 4 * * *", berlin)
	if err != nil {
		t.Fatal(err)
	}
	next := trigger.First(time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)).In(berlin)
	if want := time.Date(2026, 10, 23, 4, 0, 0, 0, berlin); !next.Equal(want) {
		t.Errorf("next %s, want %s", next, want)
	}
	if after := trigger.Next(next); !after.Equal(next.Add(24 * time.Hour)) {
		t.Errorf("following %s", after)
	}
}

func TestCronTriggerRejectsFiveFields(t *testing.T) {
	if _, err := NewCronTrigger("0 4 * * *", time.UTC); err == nil {
		t.Error("five-field expression accepted")
	}
}

func TestIntervalTriggerStartsImmediately(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	trigger := NewIntervalTrigger(time.Minute)
	if !trigger.First(now).Equal(now) || !trigger.Next(now).Equal(now.Add(time.Minute)) {
		t.Error("interval trigger does not start now and repeat every minute")
	}
}
