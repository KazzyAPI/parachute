package config

import (
	"errors"
	"fmt"
	"time"
)

type System interface {
	Hostname() (string, error)
	TempDir() string
}

type Loader struct {
	registry  *Registry
	arguments *ArgumentParser
	env       *EnvironmentReader
	resolver  *Resolver
	system    System
}

func NewLoader(registry *Registry, arguments *ArgumentParser, env *EnvironmentReader, resolver *Resolver, system System) *Loader {
	return &Loader{registry: registry, arguments: arguments, env: env, resolver: resolver, system: system}
}

func (l *Loader) Load(args, environ []string) (Config, error) {
	parsed, err := l.arguments.Parse(args)
	if err != nil {
		return Config{}, err
	}
	env, err := l.env.Read(environ)
	if err != nil {
		return Config{}, err
	}
	values, err := l.resolver.Resolve(parsed, env)
	if err != nil {
		return Config{}, err
	}
	b := &binder{registry: l.registry, values: values}
	cfg, err := l.bind(b, parsed)
	if err != nil {
		return Config{}, err
	}
	if cfg.Command != "healthcheck" {
		if err := b.unimplemented(); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func (l *Loader) bind(b *binder, parsed Arguments) (Config, error) {
	cfg := Config{
		Command:        parsed.Command,
		JSON:           b.boolean("json"),
		Profile:        b.raw("profile"),
		Relaxed:        b.relaxed(),
		Armed:          b.boolean("armed"),
		RunOnce:        b.boolean("run-once"),
		Porcelain:      b.raw("porcelain"),
		StateDir:       b.raw("state-dir"),
		RuntimeDir:     l.system.TempDir(),
		StartupMessage: !b.boolean("no-startup-message"),
		Selection:      l.selection(b, parsed),
		Detection:      Detection{WarnOnHeadFailure: b.raw("warn-on-head-failure"), DockerConfigDir: b.raw("DOCKER_CONFIG")},
		Engine:         Engine{Host: b.raw("host"), APIVersion: b.raw("api-version"), TLSVerify: b.boolean("tlsverify"), CertPath: b.raw("DOCKER_CERT_PATH")},
		Log:            l.log(b),
	}
	if b.boolean("health-check") {
		cfg.Command = "healthcheck"
	}
	if b.boolean("revive-stopped") && !cfg.Selection.IncludeStopped {
		return Config{}, errors.New("--revive-stopped requires --include-stopped")
	}
	schedule, err := l.schedule(b)
	if err != nil {
		return Config{}, err
	}
	cfg.Schedule = schedule
	notify, err := l.notify(b)
	if err != nil {
		return Config{}, err
	}
	cfg.Notify = notify
	return cfg, nil
}

func (l *Loader) selection(b *binder, parsed Arguments) Selection {
	return Selection{
		Names:               append(parsed.Positionals, b.list("name")...),
		LabelEnable:         b.boolean("label-enable"),
		Disabled:            b.list("disable-containers"),
		MonitorOnly:         b.boolean("monitor-only"),
		LabelTakePrecedence: b.boolean("label-take-precedence"),
		IncludeRestarting:   b.boolean("include-restarting"),
		IncludeStopped:      b.boolean("include-stopped"),
		Scope:               b.raw("scope"),
		ScopeSet:            b.explicit("scope"),
		NoPull:              b.boolean("no-pull"),
	}
}

func (l *Loader) schedule(b *binder) (Schedule, error) {
	if b.explicit("interval") && b.explicit("schedule") {
		return Schedule{}, errors.New("--interval and --schedule cannot be used together")
	}
	location, err := time.LoadLocation(b.raw("TZ"))
	if err != nil {
		return Schedule{}, fmt.Errorf("invalid value %q for TZ: %w", b.raw("TZ"), err)
	}
	return Schedule{Interval: b.seconds("interval"), Cron: b.raw("schedule"), Location: location}, nil
}

func (l *Loader) notify(b *binder) (Notify, error) {
	hostname := b.raw("notifications-hostname")
	if hostname == "" {
		osHostname, err := l.system.Hostname()
		if err != nil {
			return Notify{}, fmt.Errorf("cannot determine the hostname for notifications: %w", err)
		}
		hostname = osHostname
	}
	return Notify{
		URLs:      b.list("notification-url"),
		Level:     b.raw("notifications-level"),
		Hostname:  hostname,
		TitleTag:  b.raw("notification-title-tag"),
		SkipTitle: b.boolean("notification-skip-title"),
		Delay:     b.seconds("notifications-delay"),
		Template:  b.raw("notification-template"),
	}, nil
}

func (l *Loader) log(b *binder) Log {
	level := b.raw("log-level")
	if b.boolean("debug") {
		level = "debug"
	}
	if b.boolean("trace") {
		level = "trace"
	}
	return Log{Level: level, Format: b.raw("log-format"), NoColor: b.boolean("no-color")}
}
