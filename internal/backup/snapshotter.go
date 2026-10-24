package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/policy"
	"github.com/KazzyAPI/parachute/internal/txn"
	"github.com/moby/moby/api/types/mount"
)

const (
	reasonSnapshotFailed    = "snapshot_failed"
	reasonInsufficientSpace = "insufficient_space"
	reasonDumpFailed        = "dump_failed"
	reasonSnapshotInvalid   = "snapshot_invalid"
)

type Settings struct {
	Mode      string
	Volume    string
	Retention int
	Helper    string
}

type Snapshotter struct {
	engine    Engine
	store     *DockerStore
	hostFS    HostFS
	labels    LabelReader
	databases *policy.Databases
	ids       *txn.IDGenerator
	clock     Clock
	logger    *slog.Logger
	settings  Settings
	ownBackup func(domain.Container) string
	stopped   []string
}

func NewSnapshotter(engine Engine, settings Settings, labels LabelReader, ids *txn.IDGenerator, clock Clock, logger *slog.Logger) *Snapshotter {
	hostFS := HostFS(LocalFS{})
	return &Snapshotter{
		engine: engine, store: NewDockerStore(engine, hostFS, settings.Volume), hostFS: hostFS,
		labels: labels, databases: policy.NewDatabases(), ids: ids, clock: clock, logger: logger, settings: settings,
		ownBackup: func(c domain.Container) string {
			if c.Own {
				return "none"
			}
			if value, ok := labels.Value(c, "backup"); ok {
				return value
			}
			return settings.Mode
		},
	}
}

func (s *Snapshotter) Estimate(ctx context.Context, plan txn.Plan) (int64, error) {
	root, err := s.store.Root(ctx)
	if err != nil {
		return 0, err
	}
	storePath := root.HostPath
	if storePath == "" {
		volume, err := s.engine.Volume(ctx, root.VolumeName)
		if err != nil {
			return 0, err
		}
		storePath = volume.Mountpoint
	}
	var total int64
	for _, target := range plan.Targets {
		container, spec, mode, err := s.loadTarget(ctx, plan.Host, target.Container)
		if err != nil {
			return 0, err
		}
		if mode == "none" {
			continue
		}
		outsiders, err := outsiderVolumes(ctx, s.engine, map[string]bool{container.ID: true})
		if err != nil {
			return 0, err
		}
		mounts, _, err := planMounts(ctx, s.engine, s.hostFS, s.labels, storePath, container, spec, outsiders)
		if err != nil {
			return 0, err
		}
		for _, item := range mounts {
			size, err := s.hostFS.Size(ctx, item.HostSource)
			if err != nil {
				return 0, fmt.Errorf("size of %s: %w", item.Destination, err)
			}
			total += size
		}
	}
	return total, nil
}

func (s *Snapshotter) Take(ctx context.Context, plan txn.Plan) (string, error) {
	s.stopped = nil
	id, err := s.ids.New("snap")
	if err != nil {
		return "", err
	}
	root, err := s.store.Root(ctx)
	if err != nil {
		return "", err
	}
	storePath := root.HostPath
	if storePath == "" {
		volume, err := s.engine.Volume(ctx, root.VolumeName)
		if err != nil {
			return "", err
		}
		storePath = volume.Mountpoint
	}
	estimate, err := s.Estimate(ctx, plan)
	if err != nil {
		return "", err
	}
	if estimate > 0 {
		free, err := s.hostFS.FreeBytes(ctx, storePath)
		if err != nil {
			return "", fmt.Errorf("free space: %w", err)
		}
		required := estimate + estimate/10 + 1<<30
		if free < required {
			return "", fmt.Errorf("%s: estimated %d free %d", reasonInsufficientSpace, estimate, free)
		}
	}
	snapshotDir := filepath.Join(storePath, id)
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		return "", err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(snapshotDir)
			s.restartStopped(ctx)
		}
	}()
	manifest := Manifest{
		Format: format, ID: id, Transaction: plan.ID, Host: plan.Host, Stack: plan.Stacks[0],
		Created: s.clock.Now().UTC(),
	}
	affected := map[string]bool{}
	for _, target := range plan.Targets {
		container, spec, _, err := s.loadTarget(ctx, plan.Host, target.Container)
		if err != nil {
			return "", err
		}
		affected[container.ID] = true
		manifest.Containers = append(manifest.Containers, spec)
	}
	outsiders, err := outsiderVolumes(ctx, s.engine, affected)
	if err != nil {
		return "", err
	}
	var toCopy []plannedMount
	for _, target := range plan.Targets {
		container, spec, mode, err := s.loadTarget(ctx, plan.Host, target.Container)
		if err != nil {
			return "", err
		}
		if err := s.runDump(ctx, plan, container, spec, mode, snapshotDir); err != nil {
			return "", err
		}
		mounts, skipped, err := planMounts(ctx, s.engine, s.hostFS, s.labels, storePath, container, spec, outsiders)
		if err != nil {
			return "", err
		}
		for _, item := range skipped {
			s.logger.Warn("system mount skipped", "container", item.Container, "destination", item.Destination, "source", item.Source)
		}
		manifest.Skipped = append(manifest.Skipped, skipped...)
		if mode == "none" {
			continue
		}
		if mode == "stop" {
			if err := s.engine.Stop(ctx, container.ID, "SIGTERM", 10*time.Second); err != nil {
				return "", err
			}
			s.stopped = append(s.stopped, container.ID)
		}
		for _, item := range mounts {
			if item.Consistency == "live" {
				s.logger.Warn("volume copied live", "container", item.Container, "destination", item.Destination)
			}
			toCopy = append(toCopy, item)
		}
	}
	for index, item := range toCopy {
		record, err := s.copyMount(ctx, plan, snapshotDir, item, index)
		if err != nil {
			return "", err
		}
		manifest.Mounts = append(manifest.Mounts, record)
	}
	raw, err := encodeManifest(manifest)
	if err != nil {
		return "", err
	}
	if _, _, err := s.store.WriteFile(ctx, root, id, "manifest.json", bytes.NewReader(raw)); err != nil {
		return "", err
	}
	if err := s.Validate(ctx, id); err != nil {
		return "", fmt.Errorf("%s: %w", reasonSnapshotFailed, err)
	}
	cleanup = false
	return id, nil
}

func (s *Snapshotter) Validate(ctx context.Context, snapshot string) error {
	root, err := s.store.Root(ctx)
	if err != nil {
		return err
	}
	return validateSnapshot(ctx, s.store, root, snapshot, "", "", "")
}

func (s *Snapshotter) Restore(ctx context.Context, snapshot string) error {
	root, err := s.store.Root(ctx)
	if err != nil {
		return err
	}
	if err := validateSnapshot(ctx, s.store, root, snapshot, "", "", ""); err != nil {
		return fmt.Errorf("%s: %w", reasonSnapshotInvalid, err)
	}
	data, err := s.store.ReadFile(ctx, root, snapshot, "manifest.json")
	if err != nil {
		return err
	}
	manifest, err := decodeManifest(data)
	if err != nil {
		return err
	}
	for _, record := range manifest.Mounts {
		archive, err := s.store.ReadFile(ctx, root, snapshot, record.File)
		if err != nil {
			return err
		}
		if err := validateArchive(archive); err != nil {
			return fmt.Errorf("%s: %w", reasonSnapshotInvalid, err)
		}
	}
	return nil
}

func (s *Snapshotter) Discard(ctx context.Context, snapshot string) error {
	root, err := s.store.Root(ctx)
	if err != nil {
		return err
	}
	return s.store.RemoveDir(ctx, root, snapshot)
}

func (s *Snapshotter) Retain(ctx context.Context, host, stack string, keep int) error {
	return nil
}

func (s *Snapshotter) restartStopped(ctx context.Context) {
	for _, id := range s.stopped {
		_ = s.engine.Start(ctx, id)
	}
	s.stopped = nil
}

func (s *Snapshotter) loadTarget(ctx context.Context, host, name string) (domain.Container, engine.Spec, string, error) {
	containers, err := s.engine.Containers(ctx)
	if err != nil {
		return domain.Container{}, engine.Spec{}, "", err
	}
	for _, container := range containers {
		if container.Name != name {
			continue
		}
		spec, err := s.engine.Inspect(ctx, container.ID)
		if err != nil {
			return domain.Container{}, engine.Spec{}, "", err
		}
		return container, spec, s.ownBackup(container), nil
	}
	return domain.Container{}, engine.Spec{}, "", fmt.Errorf("container %s not found", name)
}

func (s *Snapshotter) copyMount(ctx context.Context, plan txn.Plan, snapshotDir string, item plannedMount, index int) (MountRecord, error) {
	file := fmt.Sprintf("%s-%d.tar.gz", sanitize(item.Container), index)
	targetArchive := filepath.Join(snapshotDir, file)
	mounts := []mount.Mount{
		{Type: mount.TypeBind, Source: item.HostSource, Target: "/src", ReadOnly: true},
		{Type: mount.TypeBind, Source: snapshotDir, Target: "/snap", ReadOnly: false},
	}
	spec := engine.HelperSpec{
		Image:   s.settings.Helper,
		Command: []string{"sh", "-c", fmt.Sprintf("tar -C /src -cf - . | gzip -1 > /snap/%s", shellQuote(file))},
		Mounts:  mounts,
		Labels:  map[string]string{"parachute.helper": "true", "parachute.transaction": plan.ID},
		CapAdd:  []string{"CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID"},
	}
	id, err := s.engine.RunHelper(ctx, spec)
	if err != nil {
		return MountRecord{}, err
	}
	defer func() { _ = s.engine.Remove(ctx, id, false, true) }()
	code, err := s.engine.Wait(ctx, id)
	if err != nil || code != 0 {
		return MountRecord{}, fmt.Errorf("copy mount %s: exit %d", item.Destination, code)
	}
	data, err := os.ReadFile(targetArchive)
	if err != nil {
		return MountRecord{}, err
	}
	sum := sha256.Sum256(data)
	return MountRecord{
		Container: item.Container, Destination: item.Destination, Source: item.Source, File: file,
		SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data)), Consistency: item.Consistency,
	}, nil
}

func sanitize(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func (s *Snapshotter) runDump(ctx context.Context, plan txn.Plan, container domain.Container, spec engine.Spec, mode, snapshotDir string) error {
	if mode == "none" {
		return nil
	}
	override, ok := s.labels.Value(container, "backup.dump")
	if ok && override == "off" {
		return nil
	}
	var cmd []string
	var env []string
	if ok {
		cmd = []string{"sh", "-c", override}
	} else if s.dumpCommand(container, spec, &cmd, &env) {
	} else {
		return nil
	}
	result, err := s.engine.Exec(ctx, container.ID, cmd, env, 10*time.Minute, 64<<20)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		s.logger.Debug("dump stderr", "container", container.Name, "exit", result.ExitCode)
		return fmt.Errorf("%s: container %s exit %d", reasonDumpFailed, container.Name, result.ExitCode)
	}
	file := sanitize(container.Name) + ".dump"
	path := filepath.Join(snapshotDir, file)
	if err := os.WriteFile(path, result.Output, 0o600); err != nil {
		return err
	}
	sum := sha256.Sum256(result.Output)
	_ = sum
	return nil
}

func (s *Snapshotter) dumpCommand(container domain.Container, spec engine.Spec, cmd, env *[]string) bool {
	if !s.databases.Recognised(container.ImageRef) {
		return false
	}
	repo := strings.ToLower(container.ImageRef)
	switch {
	case strings.Contains(repo, "postgres"):
		user := "postgres"
		if spec.Config != nil {
			for _, item := range spec.Config.Env {
				if strings.HasPrefix(item, "POSTGRES_USER=") {
					user = strings.TrimPrefix(item, "POSTGRES_USER=")
				}
				if strings.HasPrefix(item, "POSTGRES_PASSWORD=") {
					*env = append(*env, "PGPASSWORD="+strings.TrimPrefix(item, "POSTGRES_PASSWORD="))
				}
			}
		}
		*cmd = []string{"pg_dumpall", "-U", user}
		return true
	case strings.Contains(repo, "mysql"), strings.Contains(repo, "mariadb"):
		if spec.Config != nil {
			for _, item := range spec.Config.Env {
				switch {
				case strings.HasPrefix(item, "MYSQL_ROOT_PASSWORD="):
					*env = append(*env, "MYSQL_PWD="+strings.TrimPrefix(item, "MYSQL_ROOT_PASSWORD="))
				case strings.HasPrefix(item, "MARIADB_ROOT_PASSWORD="):
					*env = append(*env, "MYSQL_PWD="+strings.TrimPrefix(item, "MARIADB_ROOT_PASSWORD="))
				}
			}
		}
		tool := "mysqldump"
		if strings.Contains(repo, "mariadb") {
			tool = "mariadb-dump"
		}
		*cmd = []string{tool, "--all-databases", "-uroot"}
		return true
	}
	return false
}

func validateSnapshot(ctx context.Context, store *DockerStore, root storeRoot, snapshot, host, stack, transaction string) error {
	info, err := store.Stat(ctx, root, snapshot, "manifest.json")
	if err != nil || !info.Regular || info.Symlink || info.Size > 16<<20 {
		return errors.New("invalid manifest")
	}
	data, err := store.ReadFile(ctx, root, snapshot, "manifest.json")
	if err != nil {
		return err
	}
	manifest, err := decodeManifest(data)
	if err != nil || manifest.Format != format || manifest.ID != snapshot {
		return errors.New("invalid manifest")
	}
	if host != "" && manifest.Host != host || stack != "" && manifest.Stack != stack || transaction != "" && manifest.Transaction != transaction {
		return errors.New("manifest mismatch")
	}
	for _, record := range append(filesFromMounts(manifest.Mounts), filesFromDumps(manifest.Dumps)...) {
		if !safeFileName(record) {
			return errors.New("invalid file name")
		}
		info, err := store.Stat(ctx, root, snapshot, record)
		if err != nil || !info.Regular || info.Symlink {
			return errors.New("invalid archive")
		}
		bytes, err := store.ReadFile(ctx, root, snapshot, record)
		if err != nil {
			return err
		}
		if int64(len(bytes)) != info.Size {
			return errors.New("size mismatch")
		}
	}
	return nil
}

func filesFromMounts(records []MountRecord) []string {
	var names []string
	for _, record := range records {
		names = append(names, record.File)
	}
	return names
}

func filesFromDumps(records []DumpRecord) []string {
	var names []string
	for _, record := range records {
		names = append(names, record.File)
	}
	return names
}

func safeFileName(name string) bool {
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func validateArchive(data []byte) error {
	reader := io.NewSectionReader(bytes.NewReader(data), 0, int64(len(data)))
	return inspectTar(reader)
}
