package backup

import (
	"bytes"
	"context"
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

type fakeEngine struct {
	Engine
	specs   []engine.HelperSpec
	output  string
	ensured []string
	written []string
}

func (f *fakeEngine) RunHelper(_ context.Context, spec engine.HelperSpec) (string, error) {
	f.specs = append(f.specs, spec)
	return "helper", nil
}

func (f *fakeEngine) CreateHelper(_ context.Context, spec engine.HelperSpec) (string, error) {
	f.specs = append(f.specs, spec)
	return "helper", nil
}

func (f *fakeEngine) WriteFile(_ context.Context, _, dir, name string, _ []byte) error {
	f.written = append(f.written, dir+"/"+name)
	return nil
}
func (f *fakeEngine) Wait(context.Context, string) (int64, error) { return 0, nil }
func (f *fakeEngine) Logs(context.Context, string, int64) ([]byte, error) {
	return []byte(f.output), nil
}
func (f *fakeEngine) Remove(context.Context, string, bool, bool) error { return nil }
func (f *fakeEngine) EnsureVolume(_ context.Context, name string) error {
	f.ensured = append(f.ensured, name)
	return nil
}
func (f *fakeEngine) Volume(_ context.Context, name string) (engine.Volume, error) {
	return engine.Volume{Name: name, Mountpoint: "/var/lib/docker/volumes/" + name + "/_data"}, nil
}

type fakeHostFS map[string]Resolved

func (h fakeHostFS) Resolve(_ context.Context, path string) (Resolved, error) { return h[path], nil }
func (h fakeHostFS) Size(context.Context, mount.Mount) (int64, error)         { return 0, nil }

type noLabels struct{}

func (noLabels) Value(domain.Container, string) (string, bool) { return "", false }

func TestStoreUsesVolumeNotHostPath(t *testing.T) {
	fake := &fakeEngine{output: "file 42\n"}
	store := NewDockerStore(fake, &runner{engine: fake, image: "helper"}, "parachute-snapshots")
	ctx := context.Background()
	root, err := store.Root(ctx)
	if err != nil {
		t.Fatal(err)
	}
	info, err := store.Stat(ctx, root, "snap-1", "manifest.json")
	if err != nil || !info.Regular || info.Size != 42 {
		t.Fatalf("stat: %+v %v", info, err)
	}
	if _, err := store.ReadFile(ctx, root, "snap-1", "manifest.json"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.WriteFile(ctx, root, "snap-1", "x.dump", bytesReader("abc")); err != nil {
		t.Fatal(err)
	}
	if len(fake.written) != 1 || fake.written[0] != "/snap/snap-1/x.dump" {
		t.Fatalf("written %v", fake.written)
	}
	for _, spec := range fake.specs {
		if spec.Labels["parachute.helper"] != "true" || len(spec.Mounts) != 1 {
			t.Fatalf("spec %+v", spec)
		}
		if m := spec.Mounts[0]; m.Type != mount.TypeVolume || m.Source != "parachute-snapshots" {
			t.Fatalf("mount %+v", m)
		}
	}
	if _, err := store.ReadFile(ctx, root, "../etc", "passwd"); err == nil {
		t.Fatal("expected path rejection")
	}
}

func TestPlanMountsUsesHelperMounts(t *testing.T) {
	fake := &fakeEngine{}
	hostFS := fakeHostFS{
		"/srv/app":  {Path: "/srv/app", Backable: true},
		"/srv/link": {Path: "/etc", Backable: true},
		"/run/sock": {Path: "/run/sock"},
	}
	spec := engine.Spec{HostConfig: &container.HostConfig{Mounts: []mount.Mount{
		{Type: mount.TypeVolume, Source: "data", Target: "/data"},
		{Type: mount.TypeBind, Source: "/srv/app", Target: "/app"},
		{Type: mount.TypeBind, Source: "/srv/link", Target: "/cfg"},
		{Type: mount.TypeBind, Source: "/run/sock", Target: "/sock"},
		{Type: mount.TypeVolume, Source: "parachute-snapshots", Target: "/store"},
	}}}
	planned, skipped, err := planMounts(context.Background(), fake, hostFS, noLabels{}, "parachute-snapshots", domain.Container{Name: "a"}, spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 2 || len(skipped) != 3 {
		t.Fatalf("planned %d skipped %d", len(planned), len(skipped))
	}
	if m := planned[0].SrcMount; m.Type != mount.TypeVolume || m.Source != "data" || !m.ReadOnly || m.Target != "/src" {
		t.Fatalf("volume mount %+v", m)
	}
	if m := planned[1].SrcMount; m.Type != mount.TypeBind || m.Source != "/srv/app" || !m.ReadOnly {
		t.Fatalf("bind mount %+v", m)
	}
}

func bytesReader(s string) *bytes.Reader { return bytes.NewReader([]byte(s)) }
