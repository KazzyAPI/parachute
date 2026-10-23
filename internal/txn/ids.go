package txn

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

type IDGenerator struct {
	now func() time.Time
}

func NewIDGenerator(now func() time.Time) *IDGenerator {
	return &IDGenerator{now: now}
}

func (g *IDGenerator) Transaction() string {
	stamp := g.now().UTC().Format("20060102T%H%M%SZ")
	var suffix [3]byte
	_, _ = rand.Read(suffix[:])
	return fmt.Sprintf("txn-%s-%s", stamp, hex.EncodeToString(suffix[:]))
}

func (g *IDGenerator) Snapshot() string {
	stamp := g.now().UTC().Format("20060102T%H%M%SZ")
	var suffix [3]byte
	_, _ = rand.Read(suffix[:])
	return fmt.Sprintf("snap-%s-%s", stamp, hex.EncodeToString(suffix[:]))
}
