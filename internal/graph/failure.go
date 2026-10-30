package graph

import (
	"errors"
	"fmt"
	"strings"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/recreate"
	"github.com/KazzyAPI/parachute/internal/txn"
	"github.com/KazzyAPI/parachute/internal/verify"
)

func knownReasons() []string {
	return []string{"insufficient_space", "dump_failed", "snapshot_failed", "pull_failed", "config_unreproducible", "image_mismatch"}
}

func classify(err error, fallback string) (domain.State, string, string) {
	var failure recreate.Failure
	if errors.As(err, &failure) {
		state := domain.State(failure.State)
		if state == "" {
			state = domain.StateRolledBack
		}
		return state, failure.Reason, failure.Detail
	}
	for _, reason := range knownReasons() {
		if strings.HasPrefix(err.Error(), reason) {
			return domain.StateRolledBack, reason, err.Error()
		}
	}
	return domain.StateRolledBack, fallback, err.Error()
}

func snapshotFailure(err error) error {
	for _, reason := range knownReasons() {
		if strings.HasPrefix(err.Error(), reason) {
			return err
		}
	}
	reason := "snapshot_failed"
	var writeErr *engine.WriteError
	if errors.As(err, &writeErr) {
		reason = "engine_error"
	}
	return recreate.Failure{State: string(domain.StateFailed), Reason: reason, Detail: err.Error()}
}

func pullFailure(err error) error {
	var failure recreate.Failure
	if errors.As(err, &failure) {
		return err
	}
	return recreate.Failure{State: string(domain.StateFailed), Reason: "pull_failed", Detail: err.Error()}
}

func timeoutFailure() error {
	return recreate.Failure{State: string(domain.StateRolledBack), Reason: "transaction_timeout", Detail: "transaction ceiling reached"}
}

func verdictDetail(verdict txn.Verdict) string {
	detail := fmt.Sprintf("%s: %s", verdict.Check, verdict.Output)
	if verdict.Reason == verify.ReasonTimeout {
		detail += fmt.Sprintf("; deadline %ds expired; set the label parachute.verify.timeout for a slow-starting container", int(verdict.Deadline.Seconds()))
	}
	return detail
}
