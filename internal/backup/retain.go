package backup

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/KazzyAPI/parachute/internal/txn"
)

type RecordedSnapshot struct {
	ID          string
	Transaction string
	Time        time.Time
}

type SnapshotCatalog func(host, stack string) []RecordedSnapshot

func (s *Snapshotter) Retain(ctx context.Context, host, stack string, keep int) error {
	if keep < 0 {
		return nil
	}
	root, err := s.store.Root(ctx)
	if err != nil {
		return err
	}
	base := s.hostStorePath(root)
	if base == "" {
		return nil
	}
	recorded := map[string]RecordedSnapshot{}
	if s.catalog != nil {
		for _, item := range s.catalog(host, stack) {
			recorded[item.ID] = item
		}
	}
	var keepers []RecordedSnapshot
	for _, item := range recorded {
		if !txn.ValidID(item.ID) || !txn.ValidID(item.Transaction) {
			continue
		}
		if !s.validForRetention(ctx, root, item) {
			continue
		}
		keepers = append(keepers, item)
	}
	sort.Slice(keepers, func(i, j int) bool {
		return keepers[i].Time.After(keepers[j].Time)
	})
	allowed := map[string]bool{}
	for index, item := range keepers {
		if index < keep {
			allowed[item.ID] = true
		}
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !txn.ValidID(name) {
			continue
		}
		path := filepath.Join(base, name)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if allowed[name] {
			continue
		}
		item, ok := recorded[name]
		if !ok {
			continue
		}
		if !s.validForRetention(ctx, root, item) {
			continue
		}
		_ = os.RemoveAll(path)
	}
	return nil
}

func (s *Snapshotter) validForRetention(ctx context.Context, root storeRoot, item RecordedSnapshot) bool {
	data, err := s.store.ReadFile(ctx, root, item.ID, "manifest.json")
	if err != nil {
		return false
	}
	manifest, err := decodeManifest(data)
	if err != nil || manifest.Format != format || manifest.ID != item.ID {
		return false
	}
	if manifest.Transaction != item.Transaction {
		return false
	}
	info, err := s.store.Stat(ctx, root, item.ID, "manifest.json")
	if err != nil || !info.Regular || info.Symlink || info.Size > 16<<20 {
		return false
	}
	return validateSnapshot(ctx, s.store, root, item.ID, "", "", item.Transaction) == nil
}

func (s *Snapshotter) SetCatalog(catalog SnapshotCatalog) {
	s.catalog = catalog
}
