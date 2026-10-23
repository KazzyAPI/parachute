package config

import "time"

type Config struct {
	Command        Command
	JSON           bool
	Profile        string
	Relaxed        []string
	Relaxations    []Relaxation
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
	Policy         Policy
	API            API
	Hosts          []HostSpec
	Log            Log
	StartupMessage bool
}

type Relaxation struct {
	Setting string
	Value   string
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
	Explicit bool
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
	Report    bool
	LogStdout bool
}

type Engine struct {
	Host             string
	APIVersion       string
	TLSVerify        bool
	CertPath         string
	AllowInsecureTCP bool
}

type Policy struct {
	DelayDays        float64
	ProtectDatabases bool
	TagFollow        string
	Window           string
	WindowDuration   time.Duration
	ReminderDays     float64
}

type API struct {
	Port           int
	Update         bool
	Metrics        bool
	Status         bool
	PeriodicPolls  bool
	Token          string
	StatusToken    string
	TrustedProxies []string
}

type HostSpec struct {
	Name     string
	Endpoint string
}

type Log struct {
	Level   string
	Format  string
	NoColor bool
}
