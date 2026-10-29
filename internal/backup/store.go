package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/moby/moby/api/types/mount"
)

const readLimit = 11 << 30

type DockerStore struct {
	engine Engine
	run    *runner
	volume string
}

func NewDockerStore(engine Engine, run *runner, volume string) *DockerStore {
	return &DockerStore{engine: engine, run: run, volume: volume}
}

func (r storeRoot) mount(readOnly bool) mount.Mount {
	if r.HostPath != "" {
		return mount.Mount{Type: mount.TypeBind, Source: r.HostPath, Target: "/snap", ReadOnly: readOnly, BindOptions: &mount.BindOptions{CreateMountpoint: true}}
	}
	return mount.Mount{Type: mount.TypeVolume, Source: r.VolumeName, Target: "/snap", ReadOnly: readOnly}
}

func (s *DockerStore) Root(ctx context.Context) (storeRoot, error) {
	if strings.HasPrefix(s.volume, "/") {
		return storeRoot{HostPath: s.volume}, nil
	}
	if err := s.engine.EnsureVolume(ctx, s.volume); err != nil {
		return storeRoot{}, err
	}
	return storeRoot{VolumeName: s.volume}, nil
}

func (s *DockerStore) exec(ctx context.Context, root storeRoot, readOnly bool, script string, args ...string) ([]byte, error) {
	return s.run.run(ctx, []mount.Mount{root.mount(readOnly)}, smallOutput, script, args...)
}

func entryPath(snapshot, name string) error {
	if !safeFileName(snapshot) || name != "" && !safeFileName(name) {
		return fmt.Errorf("invalid snapshot path %q/%q", snapshot, name)
	}
	return nil
}

func (s *DockerStore) MkDir(ctx context.Context, root storeRoot, snapshot string) error {
	if err := entryPath(snapshot, ""); err != nil {
		return err
	}
	_, err := s.exec(ctx, root, false, `mkdir -p "/snap/$1"`, snapshot)
	return err
}

func (s *DockerStore) WriteFile(ctx context.Context, root storeRoot, snapshot, name string, data io.Reader) (int64, string, error) {
	if err := entryPath(snapshot, name); err != nil {
		return 0, "", err
	}
	var buf bytes.Buffer
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(&buf, hasher), data)
	if err != nil {
		return 0, "", err
	}
	helper, err := s.engine.CreateHelper(ctx, s.run.spec([]mount.Mount{root.mount(false)}, []string{"true"}))
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = s.engine.Remove(context.WithoutCancel(ctx), helper, false, true) }()
	if err := s.engine.WriteFile(ctx, helper, "/snap/"+snapshot, name, buf.Bytes()); err != nil {
		return 0, "", err
	}
	return written, hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s *DockerStore) ReadFile(ctx context.Context, root storeRoot, snapshot, name string) ([]byte, error) {
	if err := entryPath(snapshot, name); err != nil {
		return nil, err
	}
	encoded, err := s.run.run(ctx, []mount.Mount{root.mount(true)}, readLimit, `base64 "/snap/$1/$2"`, snapshot, name)
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	return data, nil
}

func (s *DockerStore) Digest(ctx context.Context, root storeRoot, snapshot, name string) (int64, string, error) {
	if err := entryPath(snapshot, name); err != nil {
		return 0, "", err
	}
	out, err := s.exec(ctx, root, true, `p="/snap/$1/$2"; echo "$(wc -c < "$p") $(sha256sum "$p" | cut -d' ' -f1)"`, snapshot, name)
	if err != nil {
		return 0, "", err
	}
	var size int64
	var sum string
	if _, err := fmt.Sscan(string(out), &size, &sum); err != nil {
		return 0, "", fmt.Errorf("digest %s: %w", name, err)
	}
	return size, sum, nil
}

func (s *DockerStore) RemoveDir(ctx context.Context, root storeRoot, snapshot string) error {
	if err := entryPath(snapshot, ""); err != nil {
		return err
	}
	_, err := s.exec(ctx, root, false, `rm -rf -- "/snap/$1"`, snapshot)
	return err
}

func (s *DockerStore) ListDirs(ctx context.Context, root storeRoot) ([]string, error) {
	out, err := s.exec(ctx, root, true, `cd /snap && for d in *; do if [ -d "$d" ] && [ ! -L "$d" ]; then echo "$d"; fi; done`)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

func (s *DockerStore) Stat(ctx context.Context, root storeRoot, snapshot, name string) (fileInfo, error) {
	if err := entryPath(snapshot, name); err != nil {
		return fileInfo{}, err
	}
	out, err := s.exec(ctx, root, true, `p="/snap/$1/$2"
if [ -L "$p" ]; then echo link 0
elif [ -f "$p" ]; then echo file $(wc -c < "$p")
elif [ -e "$p" ]; then echo other 0
else exit 3; fi`, snapshot, name)
	if err != nil {
		return fileInfo{}, err
	}
	var kind string
	var size int64
	if _, err := fmt.Sscan(string(out), &kind, &size); err != nil {
		return fileInfo{}, err
	}
	return fileInfo{Size: size, Regular: kind == "file", Symlink: kind == "link"}, nil
}

func (s *DockerStore) FreeBytes(ctx context.Context, root storeRoot) (int64, error) {
	out, err := s.exec(ctx, root, true, `df -P -k /snap | tail -n 1 | awk '{print $4}'`)
	if err != nil {
		return 0, err
	}
	var kib int64
	if _, err := fmt.Sscan(string(out), &kib); err != nil {
		return 0, fmt.Errorf("parse free space: %w", err)
	}
	return kib * 1024, nil
}
