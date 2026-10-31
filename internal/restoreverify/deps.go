package restoreverify

import (
	"context"
	"time"

	"github.com/KazzyAPI/parachute/internal/backup"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/osys"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type Clock interface {
	Now() time.Time
}

type LockFile = osys.LockFile

type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteAtomic(path string, data []byte) error
	OpenLockFile(path string) (LockFile, error)
}

type Engine interface {
	Inspect(ctx context.Context, id string) (engine.Spec, error)
	Create(ctx context.Context, spec engine.Spec) (string, error)
	Start(ctx context.Context, id string) error
	Remove(ctx context.Context, id string, anonymousVolumes, force bool) error
	RemoveVolume(ctx context.Context, name string) error
	RemoveNetwork(ctx context.Context, name string) error
	CreateInternalNetwork(ctx context.Context, name string, labels map[string]string) error
	CreateLabeledVolume(ctx context.Context, name string, labels map[string]string) error
	EnsureImage(ctx context.Context, reference string) error
	Helpers(ctx context.Context, filters map[string][]string) ([]string, error)
	ListVolumeNames(ctx context.Context, filters map[string][]string) ([]string, error)
	ListNetworkNames(ctx context.Context, filters map[string][]string) ([]string, error)
	VolumeLabels(ctx context.Context, name string) (map[string]string, error)
	NetworkLabels(ctx context.Context, name string) (map[string]string, error)
}

type Snapshotter interface {
	ValidateTransaction(ctx context.Context, snapshot, host, stack, transaction string) error
	Manifest(ctx context.Context, snapshot string) (backup.Manifest, error)
	RestoreToVolume(ctx context.Context, snapshot, archive, volume string) error
	BeginRestoreVerify(runID string) func()
}

type Verifier interface {
	Verify(ctx context.Context, plan txn.Plan, containerID string) (txn.Verdict, error)
}

type Options struct {
	StateDir  string
	Host      string
	Stack     string
	Retention int
	Verify    VerifySettings
}

type VerifySettings struct {
	Timeout time.Duration
	Stable  time.Duration
}

type Target struct {
	Host     string
	Stack    string
	Snapshot state.SnapshotRecord
}
