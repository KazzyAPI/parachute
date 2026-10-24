package config

const (
	ProfileSafe       = "safe"
	ProfileConvenient = "convenient"
)

type Registry struct {
	settings []Setting
	byFlag   map[string]Setting
	byEnv    map[string]Setting
}

func NewRegistry(settings []Setting) *Registry {
	registry := &Registry{settings: settings, byFlag: map[string]Setting{}, byEnv: map[string]Setting{}}
	for _, setting := range settings {
		if setting.Name != "" {
			registry.byFlag[setting.Name] = setting
		}
		registry.byEnv[setting.EnvName()] = setting
	}
	return registry
}

func (r *Registry) Settings() []Setting { return r.settings }

func (r *Registry) ByFlag(name string) (Setting, bool) {
	setting, ok := r.byFlag[name]
	return setting, ok
}

func (r *Registry) ByEnv(name string) (Setting, bool) {
	setting, ok := r.byEnv[name]
	return setting, ok
}

func (r *Registry) FlagNames() []string {
	names := make([]string, 0, len(r.byFlag))
	for name := range r.byFlag {
		names = append(names, "--"+name)
	}
	return names
}

func (r *Registry) EnvNames() []string {
	names := make([]string, 0, len(r.byEnv))
	for name := range r.byEnv {
		names = append(names, name)
	}
	return names
}

func convenient(value string) *string { return &value }

func notInThisMilestone(flag string) string {
	return flag + " is not implemented in this milestone"
}

func DefaultSettings() []Setting {
	return []Setting{
		{Name: "name", Kind: KindList, Repeatable: true},
		{Name: "label-enable", Kind: KindBool, Default: "false"},
		{Name: "disable-containers", Kind: KindList},
		{Name: "monitor-only", Kind: KindBool, Default: "false"},
		{Name: "label-take-precedence", Kind: KindBool, Default: "false"},
		{Name: "include-restarting", Kind: KindBool, Default: "false"},
		{Name: "include-stopped", Kind: KindBool, Default: "false"},
		{Name: "revive-stopped", Kind: KindBool, Default: "false"},
		{Name: "scope", Kind: KindString},
		{Name: "interval", Kind: KindPositiveInt, Default: "86400"},
		{Name: "schedule", Kind: KindString},
		{Env: "TZ", Kind: KindString, Default: "UTC"},
		{Name: "run-once", Kind: KindBool, Default: "false"},
		{Name: "no-startup-message", Kind: KindBool, Default: "false"},
		{Name: "warn-on-head-failure", Kind: KindEnum, Default: "auto", Choices: []string{"always", "auto", "never"}},
		{Name: "no-pull", Kind: KindBool, Default: "false"},
		{Name: "stop-timeout", Kind: KindDuration, Default: "10s"},
		{Name: "cleanup", Kind: KindBool, Default: "false"},
		{Name: "remove-volumes", Kind: KindBool, Default: "false"},
		{Name: "no-restart", Kind: KindBool, Default: "false"},
		{Name: "rolling-restart", Kind: KindBool, Default: "false"},
		{Name: "transaction-timeout", Kind: KindDuration, Default: "30m"},
		{Name: "enable-lifecycle-hooks", Kind: KindBool, Default: "false", Unimplemented: notInThisMilestone("--enable-lifecycle-hooks")},
		{Name: "backup", Kind: KindEnum, Default: "stop", Choices: []string{"stop", "live", "none"}},
		{Name: "backup-volume", Kind: KindString, Default: "parachute-snapshots"},
		{Name: "backup-retention", Kind: KindNonNegativeInt, Default: "3"},
		{Name: "helper-image", Kind: KindString},
		{Name: "verify-timeout", Kind: KindDuration, Default: "120s"},
		{Name: "verify-stable", Kind: KindDuration, Default: "30s"},
		{Name: "delay-days", Kind: KindDecimal, Default: "2", ConvenientDefault: convenient("0")},
		{Name: "protect-databases", Kind: KindBool, Default: "true", ConvenientDefault: convenient("false")},
		{Name: "tag-follow", Kind: KindEnum, Default: "none", Choices: []string{"none", "patch", "minor"}},
		{Name: "maintenance-window", Kind: KindString},
		{Name: "maintenance-window-duration", Kind: KindDuration, Default: "2h"},
		{Name: "notification-url", Kind: KindList, FromFile: true},
		{Name: "notification-report", Kind: KindBool, Default: "true"},
		{Name: "notifications-level", Kind: KindEnum, Default: "info", Choices: []string{"debug", "info", "warn", "error"}},
		{Name: "notifications-hostname", Kind: KindString},
		{Name: "notification-title-tag", Kind: KindString},
		{Name: "notification-skip-title", Kind: KindBool, Default: "false"},
		{Name: "notifications-delay", Kind: KindNonNegativeInt, Default: "0"},
		{Name: "notification-template", Kind: KindString, FromFile: true},
		{Name: "notification-log-stdout", Kind: KindBool, Default: "false"},
		{Name: "approval-reminder-days", Kind: KindPositiveDecimal, Default: "7"},
		{Name: "http-api-port", Kind: KindPositiveInt, Default: "8080"},
		{Name: "http-api-update", Kind: KindBool, Default: "false"},
		{Name: "http-api-metrics", Kind: KindBool, Default: "false"},
		{Name: "http-api-status", Kind: KindBool, Default: "false"},
		{Name: "http-api-token", Kind: KindString, FromFile: true},
		{Name: "http-api-status-token", Kind: KindString, FromFile: true},
		{Name: "http-api-periodic-polls", Kind: KindBool, Default: "false"},
		{Name: "trusted-proxies", Kind: KindList},
		{Env: "DOCKER_CONFIG", Kind: KindString, Default: "/"},
		{Name: "host", Env: "DOCKER_HOST", Kind: KindString, Default: "unix:///var/run/docker.sock"},
		{Name: "api-version", Env: "DOCKER_API_VERSION", Kind: KindString},
		{Name: "tlsverify", Env: "DOCKER_TLS_VERIFY", Kind: KindBool, Default: "false", EnvPresence: true},
		{Env: "DOCKER_CERT_PATH", Kind: KindString, Default: "/etc/ssl/docker"},
		{Name: "allow-insecure-tcp", Kind: KindBool, Default: "false"},
		{Name: "hosts", Kind: KindList},
		{Name: "restore-verify-schedule", Kind: KindString, Unimplemented: notInThisMilestone("--restore-verify-schedule")},
		{Name: "armed", Kind: KindBool, Default: "false", ConvenientDefault: convenient("true")},
		{Name: "profile", Kind: KindEnum, Default: ProfileSafe, Choices: []string{ProfileSafe, ProfileConvenient}},
		{Name: "porcelain", Kind: KindEnum, Choices: []string{"v1"}},
		{Name: "log-level", Kind: KindEnum, Default: "info", Choices: []string{"error", "warn", "info", "debug", "trace"}},
		{Name: "debug", Kind: KindBool, Default: "false"},
		{Name: "trace", Kind: KindBool, Default: "false"},
		{Name: "log-format", Kind: KindEnum, Default: "auto", Choices: []string{"auto", "logfmt", "pretty", "json"}},
		{Name: "no-color", Env: "NO_COLOR", Kind: KindBool, Default: "false", EnvPresence: true},
		{Name: "state-dir", Kind: KindString, Default: "/var/lib/parachute"},
		{Name: "json", Kind: KindBool, Default: "false"},
		{Name: "health-check", Kind: KindBool, Default: "false"},
	}
}
