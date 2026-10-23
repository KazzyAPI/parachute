package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/schedule"
)

func (l *Loader) engine(b *binder) Engine {
	return Engine{
		Host: b.raw("host"), APIVersion: b.raw("api-version"), TLSVerify: b.boolean("tlsverify"),
		CertPath: b.raw("DOCKER_CERT_PATH"), AllowInsecureTCP: b.boolean("allow-insecure-tcp"),
	}
}

func (l *Loader) policy(b *binder) Policy {
	return Policy{
		DelayDays:        b.decimal("delay-days"),
		ProtectDatabases: b.boolean("protect-databases"),
		TagFollow:        b.raw("tag-follow"),
		Window:           b.raw("maintenance-window"),
		WindowDuration:   b.duration("maintenance-window-duration"),
		ReminderDays:     b.decimal("approval-reminder-days"),
	}
}

func (l *Loader) window(expression string, location *time.Location) error {
	if expression == "" {
		return nil
	}
	if _, err := schedule.NewCronTrigger(expression, location); err != nil {
		return fmt.Errorf("invalid --maintenance-window %q: %w", expression, errors.Unwrap(err))
	}
	return nil
}

func (l *Loader) api(b *binder) API {
	return API{
		Port:           b.integer("http-api-port"),
		Update:         b.boolean("http-api-update"),
		Metrics:        b.boolean("http-api-metrics"),
		Status:         b.boolean("http-api-status"),
		PeriodicPolls:  b.boolean("http-api-periodic-polls"),
		Token:          b.raw("http-api-token"),
		StatusToken:    b.raw("http-api-status-token"),
		TrustedProxies: b.list("trusted-proxies"),
	}
}

func (l *Loader) hosts(b *binder) []HostSpec {
	var hosts []HostSpec
	for _, entry := range b.list("hosts") {
		name, endpoint, _ := strings.Cut(entry, "=")
		hosts = append(hosts, HostSpec{Name: name, Endpoint: endpoint})
	}
	return hosts
}
