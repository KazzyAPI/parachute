package handover

import (
	"strings"
	"testing"
)

func TestClassifyOldInstance(t *testing.T) {
	marker := validMarker()
	admission, err := Classify(marker, true, marker.Container, "", nil)
	if err != nil || admission.Role != RoleOld || !admission.HasMarker {
		t.Fatalf("admission %+v err=%v", admission, err)
	}
}

func TestClassifyNewInstance(t *testing.T) {
	marker := validMarker()
	admission, err := Classify(marker, true, strings.Repeat("b", 64), marker.Transaction, []string{marker.Digest})
	if err != nil || admission.Role != RoleNew {
		t.Fatalf("admission %+v err=%v", admission, err)
	}
}

func TestClassifyRejectsUnrelatedInstance(t *testing.T) {
	marker := validMarker()
	_, err := Classify(marker, true, strings.Repeat("c", 64), "", nil)
	if err == nil || !strings.Contains(err.Error(), "handover.json") {
		t.Fatalf("err=%v", err)
	}
}

func TestOldContainerActive(t *testing.T) {
	if OldContainerActive(false, "") || !OldContainerActive(true, "running") || !OldContainerActive(true, "restarting") || OldContainerActive(true, "exited") {
		t.Fatal("active check wrong")
	}
}

func TestRenamedNameUsesTransactionSuffix(t *testing.T) {
	got := RenamedName("parachute", "txn-20260101T000000Z-abc123")
	if got != "parachute-parachute-abc123" {
		t.Fatalf("got %q", got)
	}
}
