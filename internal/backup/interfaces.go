package backup

import (
	"context"
	"io"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/moby/moby/api/types/mount"
)

type Clock interface {
	Now() time.Time
}

type LabelReader interface {
	Value(container domain.Container, name string) (string, bool)
}

type DatabaseRecognizer interface {
	Recognised(reference string) bool
}

type Engine interface {
	Inspect(ctx context.Context, id string) (engine.Spec, error)
	Stop(ctx context.Context, id, signal string, timeout time.Duration) error
	Start(ctx context.Context, id string) error
	Exec(ctx context.Context, id string, cmd, env []string, limit time.Duration, maxOutput int64) (engine.ExecResult, error)
	EnsureVolume(ctx context.Context, name string) error
	Volume(ctx context.Context, name string) (engine.Volume, error)
	RunHelper(ctx context.Context, spec engine.HelperSpec) (string, error)
	Wait(ctx context.Context, id string) (int64, error)
	Remove(ctx context.Context, id string, anonymousVolumes, force bool) error
	Containers(ctx context.Context) ([]domain.Container, error)
	Helpers(ctx context.Context, filters map[string][]string) ([]string, error)
}

type HostFS interface {
	Resolve(ctx context.Context, path string) (string, error)
	Size(ctx context.Context, path string) (int64, error)
	FreeBytes(ctx context.Context, path string) (int64, error)
}

type Store interface {
	Root(ctx context.Context) (storeRoot, error)
	WriteFile(ctx context.Context, root storeRoot, snapshot, name string, data io.Reader) (int64, string, error)
	ReadFile(ctx context.Context, root storeRoot, snapshot, name string) ([]byte, error)
	RemoveDir(ctx context.Context, root storeRoot, snapshot string) error
	ListDirs(ctx context.Context, root storeRoot) ([]string, error)
	Stat(ctx context.Context, root storeRoot, snapshot, name string) (fileInfo, error)
}

type storeRoot struct {
	VolumeName string
	HostPath   string
}

type fileInfo struct {
	Size    int64
	Regular bool
	Symlink bool
}

type plannedMount struct {
	Container   string
	Destination string
	Source      string
	Kind        mount.Type
	VolumeName  string
	Consistency string
	HostSource  string
}
