package app

import (
	"fmt"
	"io"
	"slices"
)

const usageText = `Parachute updates Docker containers safely: it snapshots data, verifies the new
version and rolls back when an update fails.

Usage:
  parachute [settings]              run as a daemon (reports only until armed)
  parachute --run-once              run one cycle and exit
  parachute status [--json]         show what Parachute sees and has done
  parachute healthcheck             exit 0 when the daemon is healthy
  parachute restore-verify          prove the latest snapshot restores
  parachute version                 print the version

Common settings (each also reads PARACHUTE_<NAME>):
  --armed                  apply updates; without it Parachute only reports
  --schedule <cron>        6-field cron expression (default: once a day)
  --interval <seconds>     period between cycles
  --notification-url <u>   Discord, Slack, Telegram, email, ntfy and more
  --backup stop|live|none  how data is snapshotted before an update
  --state-dir <dir>        where state is kept (mount a volume at /var/lib/parachute)

Every setting and label: https://github.com/KazzyAPI/parachute/blob/main/docs/specs/16-configuration.md
`

func informational(args []string) (func(io.Writer, string) error, bool) {
	if len(args) == 0 {
		return nil, false
	}
	switch {
	case slices.Contains([]string{"--help", "-h", "help"}, args[0]):
		return func(out io.Writer, _ string) error { _, err := io.WriteString(out, usageText); return err }, true
	case slices.Contains([]string{"--version", "-v", "version"}, args[0]):
		return func(out io.Writer, version string) error {
			_, err := fmt.Fprintf(out, "parachute %s\n", version)
			return err
		}, true
	}
	return nil, false
}
