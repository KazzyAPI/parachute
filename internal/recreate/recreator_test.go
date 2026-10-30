package recreate

import (
	"context"
	"errors"
	"testing"
	"time"

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
		return engine.Spec{}, errors.New("No such container: " + id)
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

func TestReplacementOfDependentWithoutTargetKeepsItsOwnImage(t *testing.T) {
	fake := &fakeEngine{specs: map[string]engine.Spec{
		"old-id": {
			ID:         "old-id",
			Name:       "sidecar-parachute-abc123",
			ImageID:    "sha256:old",
			Config:     &container.Config{Image: "reg/app:1"},
			HostConfig: &container.HostConfig{},
		},
	}}
	plan := txn.Plan{ID: "txn-1", Targets: []txn.Target{{Container: "app", Image: "reg/app:1", Digest: "sha256:new"}}}

	if _, err := NewRecreator(fake, func() Settings { return Settings{} }).Replacement(context.Background(), plan, "old-id", "sidecar"); err != nil {
		t.Fatalf("Replacement: %v", err)
	}
	if fake.created.Config.Image != "sha256:old" {
		t.Fatalf("created image = %q, want sha256:old", fake.created.Config.Image)
	}
}
