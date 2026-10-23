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
	times   []time.Time
	pending int
	warned  bool
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

type failureThrottle struct {
	clock   Clock
	logger  *slog.Logger
	lock    sync.Mutex
	entries map[netip.Prefix]*failures
}

func newFailureThrottle(clock Clock, logger *slog.Logger) *failureThrottle {
	return &failureThrottle{clock: clock, logger: logger, entries: map[netip.Prefix]*failures{}}
}

func sourceKey(source netip.Addr) netip.Prefix {
	if source.Is6() {
		prefix, _ := source.Prefix(ipv6Prefix)
		return prefix
	}
	return netip.PrefixFrom(source, source.BitLen())
}

func (t *failureThrottle) reserve(source netip.Addr) (int, bool) {
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
	if len(entry.times)+entry.pending < failureLimit {
		entry.pending++
		entry.warned = false
		return 0, false
	}
	retryAfter := 1
	if len(entry.times) > 0 {
		retryAfter = int(math.Ceil(entry.times[0].Add(failureWindow).Sub(now).Seconds()))
	}
	if !entry.warned {
		entry.warned = true
		t.logger.Warn("too many failed authentications, requests are throttled", "source", source.String(), "retry_after", retryAfter)
	}
	return min(max(retryAfter, 1), 60), true
}

func (t *failureThrottle) release(source netip.Addr, failed bool) {
	t.lock.Lock()
	defer t.lock.Unlock()
	key := sourceKey(source)
	entry := t.entries[key]
	if entry == nil {
		return
	}
	entry.pending--
	if failed {
		entry.times = append(entry.times, t.clock.Now())
	}
	if len(entry.times) == 0 && entry.pending == 0 {
		delete(t.entries, key)
	}
}

func (t *failureThrottle) evictOldest() {
	var oldest netip.Prefix
	var oldestTime time.Time
	found := false
	for key, entry := range t.entries {
		if newest := entry.newest(); entry.pending == 0 && (!found || newest.Before(oldestTime)) {
			oldest, oldestTime, found = key, newest, true
		}
	}
	if found {
		delete(t.entries, oldest)
	}
}
