package config

type Command string

const (
	CommandDaemon        Command = ""
	CommandHealthcheck   Command = "healthcheck"
	CommandStatus        Command = "status"
	CommandRestoreVerify Command = "restore-verify"
)
