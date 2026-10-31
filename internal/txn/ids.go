package txn

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"time"
)

var (
	idPattern              = regexp.MustCompile(`^(txn|snap)-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}$`)
	restoreVerifyIDPattern = regexp.MustCompile(`^rv-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}$`)
)

type IDGenerator struct {
	now func() time.Time
}

func NewIDGenerator(now func() time.Time) *IDGenerator {
	return &IDGenerator{now: now}
}

func (g *IDGenerator) New(prefix string) (string, error) {
	var suffix [3]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return fmt.Sprintf("%s-%s-%s", prefix, g.now().UTC().Format("20060102T150405Z"), hex.EncodeToString(suffix[:])), nil
}

func ValidID(id string) bool { return idPattern.MatchString(id) }

func ValidRestoreVerifyID(id string) bool { return restoreVerifyIDPattern.MatchString(id) }

func RunSuffix(runID string) string {
	if len(runID) < 6 {
		return runID
	}
	return runID[len(runID)-6:]
}
