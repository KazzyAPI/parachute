package recreate

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/container"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type fakeEngine struct {
	specs   map[string]engine.Spec
	created engine.Spec
}

func (f *fakeEngine) Inspect(_ context.Context, id string) (engine.Spec, error) {
	spec, ok := f.specs[id]
	if !ok {
		return engine.Spec{}, fmt.Errorf("No such container: %s: %w", id, cerrdefs.ErrNotFound)
	}
	return spec, nil
}

func (f *fakeEngine) Image(context.Context, string) (engine.Image, bool, error) {
	return engine.Image{ID: "sha256:new"}, true, nil
}

func (f *fakeEngine) ImageConfig(context.Context, string) (*dockerspec.DockerOCIImageConfig, error) {
	return &dockerspec.DockerOCIImageConfig{}, nil
}

func (f *fakeEngine) Pull(context.Context, string, string, engine.RegistryAuth) error { return nil }
func (f *fakeEngine) Tag(context.Context, string, string) error                       { return nil }
func (f *fakeEngine) Start(context.Context, string) error                             { return nil }
func (f *fakeEngine) Stop(context.Context, string, string, time.Duration) error       { return nil }
func (f *fakeEngine) Rename(context.Context, string, string) error                    { return nil }
func (f *fakeEngine) Remove(context.Context, string, bool, bool) error                { return nil }
func (f *fakeEngine) RemoveImage(context.Context, string) error                       { return nil }

func (f *fakeEngine) Create(_ context.Context, spec engine.Spec) (string, error) {
	spec.ImageID = "sha256:new"
	f.created = spec
	f.specs["new-id"] = spec
	return "new-id", nil
}

func TestReplacementInspectsRenamedOriginalByIDAndKeepsName(t *testing.T) {
	fake := &fakeEngine{specs: map[string]engine.Spec{
		"old-id": {
			ID:         "old-id",
			Name:       "app-parachute-abc123",
			ImageID:    "sha256:old",
			Config:     &container.Config{Image: "reg/app:1"},
			HostConfig: &container.HostConfig{},
		},
	}}
	plan := txn.Plan{ID: "txn-1", Targets: []txn.Target{{Container: "app", Image: "reg/app:1", Digest: "sha256:new"}}}

	id, err := NewRecreator(fake, func() Settings { return Settings{} }).Replacement(context.Background(), plan, "old-id", "app")
	if err != nil {
		t.Fatalf("Replacement: %v", err)
	}
	if id != "new-id" {
		t.Fatalf("id = %q", id)
	}
	if fake.created.Name != "app" {
		t.Fatalf("created name = %q, want app", fake.created.Name)
	}
}

func TestFollowNetworkPeerPointsAtTheReplacement(t *testing.T) {
	r := NewRecreator(nil, nil)
	r.remember(engine.Spec{ID: "old-id", Name: "app"}, "new-id")
	for _, peer := range []string{"old-id", "app"} {
		host := &container.HostConfig{NetworkMode: container.NetworkMode("container:" + peer)}
		r.followNetworkPeer(host)
		if host.NetworkMode != "container:new-id" {
			t.Fatalf("peer %s: network mode %s", peer, host.NetworkMode)
		}
	}
	host := &container.HostConfig{NetworkMode: "container:other"}
	r.followNetworkPeer(host)
	if host.NetworkMode != "container:other" {
		t.Fatalf("unrelated peer rewritten: %s", host.NetworkMode)
	}
}

func TestReplacementOfNetworkModeDependentDropsHostname(t *testing.T) {
	fake := &fakeEngine{specs: map[string]engine.Spec{
		"old-id": {
			ID:         "old-id",
			Name:       "sidecar",
			ImageID:    "sha256:old",
			Config:     &container.Config{Image: "reg/side:1", Hostname: "abc123def456", Domainname: "local"},
			HostConfig: &container.HostConfig{NetworkMode: "container:app"},
		},
		"app": {ID: "app", Name: "app"},
	}}

	if _, err := NewRecreator(fake, func() Settings { return Settings{} }).Replacement(context.Background(), txn.Plan{ID: "txn-1"}, "old-id", "sidecar"); err != nil {
		t.Fatalf("Replacement: %v", err)
	}
	if fake.created.Config.Hostname != "" || fake.created.Config.Domainname != "" {
		t.Fatalf("created hostname %q domain %q", fake.created.Config.Hostname, fake.created.Config.Domainname)
	}
}

func TestFollowRenamedLinksRestoresOriginalTargetName(t *testing.T) {
	host := &container.HostConfig{Links: []string{"/db-parachute-77bb95:/web/db", "/cache:/web/cache"}}
	followRenamedLinks(host, "txn-20261030T094910Z-77bb95")
	want := []string{"/db:/web/db", "/cache:/web/cache"}
	if len(host.Links) != 2 || host.Links[0] != want[0] || host.Links[1] != want[1] {
		t.Fatalf("links = %v, want %v", host.Links, want)
	}
}

type timeoutPeerEngine struct{ *fakeEngine }

func (timeoutPeerEngine) Inspect(context.Context, string) (engine.Spec, error) {
	return engine.Spec{}, context.DeadlineExceeded
}

func TestCheckReproducibleDistinguishesMissingPeerFromEngineError(t *testing.T) {
	spec := engine.Spec{HostConfig: &container.HostConfig{NetworkMode: "container:app"}}
	var failure Failure
	if err := checkReproducible(context.Background(), &fakeEngine{specs: map[string]engine.Spec{}}, spec); !errors.As(err, &failure) || failure.Detail != "NetworkMode" {
		t.Fatalf("missing peer: %v", err)
	}
	err := checkReproducible(context.Background(), timeoutPeerEngine{&fakeEngine{}}, spec)
	if !errors.Is(err, context.DeadlineExceeded) || errors.As(err, &failure) {
		t.Fatalf("engine error: %v", err)
	}
}
