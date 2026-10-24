package backup

import (
	"encoding/json"
	"time"

	"github.com/KazzyAPI/parachute/internal/engine"
)

const format = "parachute.snapshot/v1"

type Manifest struct {
	Format      string          `json:"format"`
	ID          string          `json:"id"`
	Transaction string          `json:"transaction"`
	Host        string          `json:"host"`
	Stack       string          `json:"stack"`
	Created     time.Time       `json:"created"`
	Containers  []engine.Spec   `json:"containers"`
	Mounts      []MountRecord   `json:"mounts"`
	Dumps       []DumpRecord    `json:"dumps"`
	Skipped     []SkippedRecord `json:"skipped"`
}

type MountRecord struct {
	Container   string `json:"container"`
	Destination string `json:"destination"`
	Source      string `json:"source"`
	File        string `json:"file"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Consistency string `json:"consistency"`
}

type DumpRecord struct {
	Container string `json:"container"`
	File      string `json:"file"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
}

type SkippedRecord struct {
	Container   string `json:"container"`
	Destination string `json:"destination"`
	Source      string `json:"source"`
}

func encodeManifest(m Manifest) ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

func decodeManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}
