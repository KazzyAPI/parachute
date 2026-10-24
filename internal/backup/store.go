package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type DockerStore struct {
	engine Engine
	hostFS HostFS
	volume string
}

func NewDockerStore(engine Engine, hostFS HostFS, volume string) *DockerStore {
	return &DockerStore{engine: engine, hostFS: hostFS, volume: volume}
}

func (s *DockerStore) Root(ctx context.Context) (storeRoot, error) {
	if strings.HasPrefix(s.volume, "/") {
		return storeRoot{HostPath: s.volume}, nil
	}
	if err := s.engine.EnsureVolume(ctx, s.volume); err != nil {
		return storeRoot{}, err
	}
	volume, err := s.engine.Volume(ctx, s.volume)
	if err != nil {
		return storeRoot{}, err
	}
	return storeRoot{VolumeName: s.volume, HostPath: volume.Mountpoint}, nil
}

func (s *DockerStore) snapshotPath(root storeRoot, snapshot string) string {
	if root.HostPath != "" {
		return filepath.Join(root.HostPath, snapshot)
	}
	return snapshot
}

func (s *DockerStore) WriteFile(ctx context.Context, root storeRoot, snapshot, name string, data io.Reader) (int64, string, error) {
	if root.HostPath != "" {
		dir := filepath.Join(root.HostPath, snapshot)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return 0, "", err
		}
		path := filepath.Join(dir, name)
		file, err := os.Create(path)
		if err != nil {
			return 0, "", err
		}
		hasher := sha256.New()
		written, err := io.Copy(io.MultiWriter(file, hasher), data)
		closeErr := file.Close()
		if err != nil {
			return 0, "", err
		}
		if closeErr != nil {
			return 0, "", closeErr
		}
		return written, hex.EncodeToString(hasher.Sum(nil)), nil
	}
	helper, err := s.runStoreHelper(ctx, root, snapshot, []string{"sh", "-c", fmt.Sprintf("cat > /snap/%s", shellQuote(name))}, data)
	if err != nil {
		return 0, "", err
	}
	info, err := s.Stat(ctx, root, snapshot, name)
	if err != nil {
		_ = s.engine.Remove(ctx, helper, false, true)
		return 0, "", err
	}
	path := filepath.Join(s.snapshotPath(root, snapshot), name)
	sum, err := s.hashHostFile(ctx, path)
	_ = s.engine.Remove(ctx, helper, false, true)
	return info.Size, sum, err
}

func (s *DockerStore) ReadFile(ctx context.Context, root storeRoot, snapshot, name string) ([]byte, error) {
	if root.HostPath != "" {
		return os.ReadFile(filepath.Join(root.HostPath, snapshot, name))
	}
	path := filepath.Join(s.snapshotPath(root, snapshot), name)
	return s.readHostFile(ctx, path)
}

func (s *DockerStore) RemoveDir(ctx context.Context, root storeRoot, snapshot string) error {
	target := filepath.Join(s.hostRoot(root), snapshot)
	if target == "" || target == snapshot {
		volume, err := s.engine.Volume(ctx, root.VolumeName)
		if err != nil {
			return err
		}
		target = filepath.Join(volume.Mountpoint, snapshot)
	}
	return os.RemoveAll(target)
}

func (s *DockerStore) ListDirs(ctx context.Context, root storeRoot) ([]string, error) {
	base := root.HostPath
	if base == "" {
		volume, err := s.engine.Volume(ctx, root.VolumeName)
		if err != nil {
			return nil, err
		}
		base = volume.Mountpoint
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

func (s *DockerStore) Stat(ctx context.Context, root storeRoot, snapshot, name string) (fileInfo, error) {
	path := filepath.Join(s.hostRoot(root), snapshot, name)
	stat, err := os.Lstat(path)
	if err != nil {
		return fileInfo{}, err
	}
	mode := stat.Mode()
	return fileInfo{Size: stat.Size(), Regular: mode.IsRegular(), Symlink: mode&os.ModeSymlink != 0}, nil
}

func (s *DockerStore) hostRoot(root storeRoot) string {
	if root.HostPath != "" {
		return root.HostPath
	}
	volume, err := s.engine.Volume(context.Background(), root.VolumeName)
	if err != nil {
		return ""
	}
	return volume.Mountpoint
}

func (s *DockerStore) runStoreHelper(ctx context.Context, root storeRoot, snapshot string, cmd []string, stdin io.Reader) (string, error) {
	_ = ctx
	_ = root
	_ = snapshot
	_ = cmd
	_ = stdin
	return "", fmt.Errorf("not implemented")
}

func (s *DockerStore) hashHostFile(ctx context.Context, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (s *DockerStore) readHostFile(ctx context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
