package api

import (
	"log/slog"
	"math"
	"net/netip"
	"sync"
	"time"
)

const (
	failureWindow  = 60 * time.Second
	failureLimit   = 10
	trackedSources = 10000
	ipv6Prefix     = 64
)

type failures struct {
	times  []time.Time
	warned bool
}

func (f *failures) prune(now time.Time) {
	kept := f.times[:0]
	for _, failed := range f.times {
		if now.Sub(failed) < failureWindow {
			kept = append(kept, failed)
		}
	}
	f.times = kept
}

func (f *failures) newest() time.Time {
	if len(f.times) == 0 {
		return time.Time{}
	}
	return f.times[len(f.times)-1]
}

type Throttle struct {
	clock   Clock
	logger  *slog.Logger
	lock    sync.Mutex
	entries map[netip.Prefix]*failures
}

func NewThrottle(clock Clock, logger *slog.Logger) *Throttle {
	return &Throttle{clock: clock, logger: logger, entries: map[netip.Prefix]*failures{}}
}

func sourceKey(source netip.Addr) netip.Prefix {
	if source.Is6() {
		prefix, _ := source.Prefix(ipv6Prefix)
		return prefix
	}
	return netip.PrefixFrom(source, source.BitLen())
}

func (t *Throttle) Check(source netip.Addr) (int, bool) {
	t.lock.Lock()
	defer t.lock.Unlock()
	entry := t.entries[sourceKey(source)]
	if entry == nil {
		return 0, false
	}
	now := t.clock.Now()
	entry.prune(now)
	if len(entry.times) < failureLimit {
		entry.warned = false
		return 0, false
	}
	retryAfter := int(math.Ceil(entry.times[0].Add(failureWindow).Sub(now).Seconds()))
	if !entry.warned {
		entry.warned = true
		t.logger.Warn("too many failed authentications, requests are throttled", "source", source.String(), "retry_after", retryAfter)
	}
	return min(max(retryAfter, 1), 60), true
}

func (t *Throttle) Fail(source netip.Addr) {
	t.lock.Lock()
	defer t.lock.Unlock()
	key := sourceKey(source)
	entry := t.entries[key]
	if entry == nil {
		if len(t.entries) >= trackedSources {
			t.evictOldest()
		}
		entry = &failures{}
		t.entries[key] = entry
	}
	now := t.clock.Now()
	entry.prune(now)
	entry.times = append(entry.times, now)
}

func (t *Throttle) evictOldest() {
	var oldest netip.Prefix
	var oldestTime time.Time
	first := true
	for key, entry := range t.entries {
		if newest := entry.newest(); first || newest.Before(oldestTime) {
			oldest, oldestTime, first = key, newest, false
		}
	}
	delete(t.entries, oldest)
}
