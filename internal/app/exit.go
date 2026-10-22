package app

import (
	"errors"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }

func (e *ExitError) Unwrap() error { return e.Err }

func configurationError(err error) error { return &ExitError{Code: domain.ExitConfiguration, Err: err} }

func unreachableError(err error) error { return &ExitError{Code: domain.ExitUnreachable, Err: err} }

func exitCodeOf(err error) int {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return domain.ExitFailures
}
