package config

import "time"

type Config struct {
	Command        Command
	JSON           bool
	Profile        string
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
	Backup         Backup
	Update         Update
	StartupMessage bool
	Verify         Verify
	RestoreVerify  RestoreVerify
	NoRestart      bool
	Hooks          Hooks
}

type Hooks struct {
	Enabled bool
}

type Verify struct {
	Timeout time.Duration
	Stable  time.Duration
}

type RestoreVerify struct {
	Stack string
	Host  string
}

type Backup struct {
	Mode      string
	Volume    string
	Retention int
	Helper    string
}

type Update struct {
	StopTimeout        time.Duration
	TransactionTimeout time.Duration
	Cleanup            bool
	RemoveVolumes      bool
	RollingRestart     bool
	ReviveStopped      bool
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
	HostSet          bool
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
