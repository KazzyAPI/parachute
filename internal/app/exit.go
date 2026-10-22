package app

import (
	"errors"

	"github.com/KazzyAPI/parachute/internal/domain"
)

const (
	ExitSuccess        = 0
	ExitFailures       = 1
	ExitConfiguration  = 2
	ExitRollbackFailed = 3
	ExitUnreachable    = 4
)

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }

func (e *ExitError) Unwrap() error { return e.Err }

func configurationError(err error) error { return &ExitError{Code: ExitConfiguration, Err: err} }

func unreachableError(err error) error { return &ExitError{Code: ExitUnreachable, Err: err} }

func exitCodeOf(err error) int {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return ExitConfiguration
}

func reportExitCode(report domain.Report) int {
	code := ExitSuccess
	if !report.AllHostsReachable() {
		code = ExitFailures
	}
	for _, result := range report.Results() {
		switch result.State {
		case domain.StateRollbackFailed:
			code = max(code, ExitRollbackFailed)
		case domain.StateFailed, domain.StateRolledBack:
			code = max(code, ExitFailures)
		}
	}
	return code
}
