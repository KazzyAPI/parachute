package engine

import "fmt"

type WriteError struct {
	Operation string
	Err       error
}

func (e *WriteError) Error() string {
	return fmt.Sprintf("%s: %v", e.Operation, e.Err)
}

func (e *WriteError) Unwrap() error { return e.Err }

func writeError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &WriteError{Operation: operation, Err: err}
}
