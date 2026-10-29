package graph

import (
	"errors"
	"fmt"
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/recreate"
)

func TestDMN_005_snapshot_engine_write_failure_is_engine_error(t *testing.T) {
	err := snapshotFailure(fmt.Errorf("snapshot: %w", &engine.WriteError{Operation: "stop container", Err: errors.New("boom")}))
	var failure recreate.Failure
	if !errors.As(err, &failure) || failure.Reason != "engine_error" || failure.State != string(domain.StateFailed) {
		t.Fatalf("failure %+v", err)
	}
}

func TestUPD_014_other_snapshot_failure_keeps_snapshot_failed(t *testing.T) {
	var failure recreate.Failure
	if !errors.As(snapshotFailure(errors.New("disk")), &failure) || failure.Reason != "snapshot_failed" {
		t.Fatalf("failure %+v", failure)
	}
}
