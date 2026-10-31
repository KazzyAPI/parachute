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
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/policy"
	"github.com/KazzyAPI/parachute/internal/redact"
	"github.com/KazzyAPI/parachute/internal/txn"
	"github.com/moby/moby/api/types/mount"
)

const (
	reasonSnapshotFailed    = "snapshot_failed"
	reasonInsufficientSpace = "insufficient_space"
	reasonDumpFailed        = "dump_failed"
	dumpLimit               = 64 << 20
	reasonSnapshotInvalid   = "snapshot_invalid"
)

type Settings struct {
	Mode      string
	Volume    string
	Retention int
	Helper    string
}

const NoSnapshot = "none"

type Snapshotter struct {
	engine    Engine
	run       *runner
	store     *DockerStore
	hostFS    HostFS
	labels    LabelReader
	databases *policy.Databases
	ids       *txn.IDGenerator
	clock     Clock
	logger    *slog.Logger
	settings  Settings
	catalog   SnapshotCatalog
	ownBackup func(domain.Container) string
	stopped   []string
}

func NewSnapshotter(engine Engine, settings Settings, labels LabelReader, ids *txn.IDGenerator, clock Clock, logger *slog.Logger) *Snapshotter {
	run := &runner{engine: engine, image: settings.Helper}
	return &Snapshotter{
		engine: engine, run: run, store: NewDockerStore(engine, run, settings.Volume), hostFS: HelperFS{r: run},
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
	s.run.txn = plan.ID
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
		mounts, _, err := planMounts(ctx, s.engine, s.hostFS, s.labels, s.settings.Volume, container, spec, outsiders)
		if err != nil {
			return 0, err
		}
		for _, item := range mounts {
			size, err := s.hostFS.Size(ctx, item.SrcMount)
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
	if all, err := s.allUnbacked(ctx, plan); err != nil {
		return "", err
	} else if all {
		return NoSnapshot, nil
	}
	id, err := s.ids.New("snap")
	if err != nil {
		return "", err
	}
	root, err := s.store.Root(ctx)
	if err != nil {
		return "", err
	}
	estimate, err := s.Estimate(ctx, plan)
	if err != nil {
		return "", err
	}
	if estimate > 0 {
		free, err := s.store.FreeBytes(ctx, root)
		if err != nil {
			return "", fmt.Errorf("free space: %w", err)
		}
		required := estimate + estimate/10 + 1<<30
		if free < required {
			return "", fmt.Errorf("%s: estimated %d free %d", reasonInsufficientSpace, estimate, free)
		}
	}
	if err := s.store.MkDir(ctx, root, id); err != nil {
		return "", err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = s.store.RemoveDir(context.WithoutCancel(ctx), root, id)
			s.restartStopped(context.WithoutCancel(ctx))
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
		dump, err := s.runDump(ctx, plan, root, id, container, spec, mode)
		if err != nil {
			return "", err
		}
		if dump.File != "" {
			manifest.Dumps = append(manifest.Dumps, dump)
		}
		mounts, skipped, err := planMounts(ctx, s.engine, s.hostFS, s.labels, s.settings.Volume, container, spec, outsiders)
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
		if mode == "stop" && len(mounts) > 0 {
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
		record, err := s.copyMount(ctx, plan, root, id, item, index)
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

func (s *Snapshotter) allUnbacked(ctx context.Context, plan txn.Plan) (bool, error) {
	for _, target := range plan.Targets {
		_, _, mode, err := s.loadTarget(ctx, plan.Host, target.Container)
		if err != nil {
			return false, err
		}
		if mode != "none" {
			return false, nil
		}
	}
	return len(plan.Targets) > 0, nil
}

func (s *Snapshotter) Validate(ctx context.Context, snapshot string) error {
	root, err := s.store.Root(ctx)
	if err != nil {
		return err
	}
	return validateSnapshot(ctx, s.store, root, snapshot, "", "", "")
}

func (s *Snapshotter) Discard(ctx context.Context, snapshot string) error {
	root, err := s.store.Root(ctx)
	if err != nil {
		return err
	}
	return s.store.RemoveDir(ctx, root, snapshot)
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

func (s *Snapshotter) copyMount(ctx context.Context, plan txn.Plan, root storeRoot, snapshot string, item plannedMount, index int) (MountRecord, error) {
	file := fmt.Sprintf("%s-%d.tar.gz", sanitize(item.Container), index)
	if ceiling := txn.CeilingFrom(ctx); ceiling != nil {
		ceiling.Pause()
		defer ceiling.Resume()
	}
	s.run.txn = plan.ID
	_, err := s.run.run(ctx, []mount.Mount{item.SrcMount, root.mount(false)}, smallOutput,
		`tar -C /src -cf "/snap/$1/$2.part" . && gzip -1 < "/snap/$1/$2.part" > "/snap/$1/$2" && rm "/snap/$1/$2.part"`, snapshot, file)
	if err != nil {
		return MountRecord{}, fmt.Errorf("copy mount %s: %w", item.Destination, err)
	}
	size, sum, err := s.store.Digest(ctx, root, snapshot, file)
	if err != nil {
		return MountRecord{}, err
	}
	return MountRecord{
		Container: item.Container, Destination: item.Destination, Source: item.Source, File: file,
		SHA256: sum, Size: size, Consistency: item.Consistency,
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

func (s *Snapshotter) runDump(ctx context.Context, plan txn.Plan, root storeRoot, snapshot string, container domain.Container, spec engine.Spec, mode string) (DumpRecord, error) {
	if mode == "none" {
		return DumpRecord{}, nil
	}
	override, ok := s.labels.Value(container, "backup.dump")
	if ok && override == "off" {
		return DumpRecord{}, nil
	}
	var cmd []string
	var env []string
	switch {
	case ok:
		cmd = []string{"sh", "-c", override}
	case s.dumpCommand(container, spec, &cmd, &env):
	default:
		return DumpRecord{}, nil
	}
	result, err := s.engine.Exec(ctx, container.ID, cmd, env, 10*time.Minute, dumpLimit)
	if err != nil {
		return DumpRecord{}, err
	}
	if result.ExitCode != 0 {
		s.logger.Debug("dump failed", "container", container.Name, "exit", result.ExitCode, "stderr", redactEnvValues(string(result.Output), spec))
		return DumpRecord{}, fmt.Errorf("%s: container %s exit %d", reasonDumpFailed, container.Name, result.ExitCode)
	}
	if result.Truncated {
		return DumpRecord{}, fmt.Errorf("%s: dump of %s exceeds %d bytes", reasonSnapshotFailed, container.Name, dumpLimit)
	}
	file := sanitize(container.Name) + ".dump"
	size, sum, err := s.store.WriteFile(ctx, root, snapshot, file, bytes.NewReader(result.Output))
	if err != nil {
		return DumpRecord{}, err
	}
	return DumpRecord{Container: container.Name, File: file, SHA256: sum, Size: size}, nil
}

func redactEnvValues(text string, spec engine.Spec) string {
	var values []string
	if spec.Config != nil {
		for _, pair := range spec.Config.Env {
			if _, value, ok := strings.Cut(pair, "="); ok {
				values = append(values, value)
			}
		}
	}
	return redact.New(redact.Static(values...)).Redact(text)
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
	expected := checksumIndex(manifest)
	for name := range expected {
		if !safeFileName(name) {
			return errors.New("invalid file name")
		}
		info, err := store.Stat(ctx, root, snapshot, name)
		if err != nil || !info.Regular || info.Symlink {
			return errors.New("invalid archive")
		}
		bytes, err := store.ReadFile(ctx, root, snapshot, name)
		if err != nil {
			return err
		}
		want := expected[name]
		if int64(len(bytes)) != want.size || want.sha != sha256Hex(bytes) {
			return errors.New("checksum mismatch")
		}
	}
	return nil
}

func checksumIndex(manifest Manifest) map[string]struct {
	size int64
	sha  string
} {
	index := map[string]struct {
		size int64
		sha  string
	}{}
	for _, record := range manifest.Mounts {
		index[record.File] = struct {
			size int64
			sha  string
		}{record.Size, record.SHA256}
	}
	for _, record := range manifest.Dumps {
		index[record.File] = struct {
			size int64
			sha  string
		}{record.Size, record.SHA256}
	}
	return index
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
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
