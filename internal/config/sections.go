package config

func (l *Loader) engine(b *binder) Engine {
	return Engine{
		Host: b.raw("host"), APIVersion: b.raw("api-version"), TLSVerify: b.boolean("tlsverify"),
		CertPath: b.raw("DOCKER_CERT_PATH"), AllowInsecureTCP: b.boolean("allow-insecure-tcp"),
		HostSet: b.explicit("host"),
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

func (l *Loader) update(b *binder) Update {
	return Update{
		StopTimeout:        b.duration("stop-timeout"),
		TransactionTimeout: b.duration("transaction-timeout"),
		Cleanup:            b.boolean("cleanup"),
		RemoveVolumes:      b.boolean("remove-volumes"),
		RollingRestart:     b.boolean("rolling-restart"),
		ReviveStopped:      b.boolean("revive-stopped"),
	}
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
