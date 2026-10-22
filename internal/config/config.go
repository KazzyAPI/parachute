package config

import "time"

type Config struct {
	Command        string
	JSON           bool
	Profile        string
	Relaxed        []string
	Armed          bool
	RunOnce        bool
	Porcelain      string
	StateDir       string
	RuntimeDir     string
	Selection      Selection
	Schedule       Schedule
	Detection      Detection
	Notify         Notify
	Engine         Engine
	Log            Log
	StartupMessage bool
}

type Selection struct {
	Names               []string
	LabelEnable         bool
	Disabled            []string
	MonitorOnly         bool
	LabelTakePrecedence bool
	IncludeRestarting   bool
	IncludeStopped      bool
	Scope               string
	ScopeSet            bool
	NoPull              bool
}

type Schedule struct {
	Interval time.Duration
	Cron     string
	Location *time.Location
}

type Detection struct {
	WarnOnHeadFailure string
	DockerConfigDir   string
}

type Notify struct {
	URLs      []string
	Level     string
	Hostname  string
	TitleTag  string
	SkipTitle bool
	Delay     time.Duration
	Template  string
}

type Engine struct {
	Host       string
	APIVersion string
	TLSVerify  bool
	CertPath   string
}

type Log struct {
	Level   string
	Format  string
	NoColor bool
}
