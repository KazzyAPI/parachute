package handover

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const PollInterval = time.Second

type Role int

const (
	RoleNone Role = iota
	RoleOld
	RoleNew
)

type Admission struct {
	Role      Role
	Marker    Marker
	HasMarker bool
}

func Classify(marker Marker, hasMarker bool, ownID, transactionLabel string, digests []string) (Admission, error) {
	if !hasMarker {
		return Admission{Role: RoleNone}, nil
	}
	if marker.Container == ownID {
		return Admission{Role: RoleOld, Marker: marker, HasMarker: true}, nil
	}
	if transactionLabel == marker.Transaction && DigestMatches(digests, marker.Digest) {
		return Admission{Role: RoleNew, Marker: marker, HasMarker: true}, nil
	}
	return Admission{}, fmt.Errorf("handover.json")
}

func DigestMatches(digests []string, digest string) bool {
	return slices.Contains(digests, digest)
}

func OldContainerActive(exists bool, state string) bool {
	if !exists {
		return false
	}
	return state == "running" || state == "restarting"
}

func RenamedSuffix(transaction string) string {
	if len(transaction) < 6 {
		return transaction
	}
	return transaction[len(transaction)-6:]
}

func RenamedName(originalName, transaction string) string {
	name := strings.TrimPrefix(originalName, "/")
	return name + "-parachute-" + RenamedSuffix(transaction)
}

func InvalidMarkerError(err error) error {
	if err == nil {
		return fmt.Errorf("handover.json")
	}
	return fmt.Errorf("handover.json: %w", err)
}
