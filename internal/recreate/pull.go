package recreate

import (
	"context"
	"strings"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/registry"
	"github.com/KazzyAPI/parachute/internal/txn"
)

func PullTarget(ctx context.Context, e Engine, target txn.Target) error {
	_, err := pullApproved(ctx, e, target)
	return err
}

func pullApproved(ctx context.Context, e Engine, target txn.Target) (engine.Image, error) {
	ref, err := registry.ParseReference(target.Image)
	if err != nil {
		return engine.Image{}, Failure{State: "failed", Reason: "pull_failed", Detail: err.Error()}
	}
	repository := ref.Domain + "/" + ref.Repository
	if err := e.Pull(ctx, repository, target.Digest, ""); err != nil {
		return engine.Image{}, Failure{State: "failed", Reason: "pull_failed", Detail: err.Error()}
	}
	pulled, found, err := e.Image(ctx, repository+"@"+target.Digest)
	if err != nil {
		return engine.Image{}, Failure{State: "failed", Reason: "pull_failed", Detail: err.Error()}
	}
	if !found || !containsDigest(pulled.Digests, target.Digest) {
		return engine.Image{}, Failure{State: "failed", Reason: "pull_failed", Detail: target.Digest + " " + strings.Join(pulled.Digests, ",")}
	}
	if err := e.Tag(ctx, repository+"@"+target.Digest, target.Image); err != nil {
		return engine.Image{}, Failure{State: "failed", Reason: "pull_failed", Detail: err.Error()}
	}
	return pulled, nil
}

func containsDigest(digests []string, want string) bool {
	want = strings.TrimPrefix(want, "sha256:")
	for _, digest := range digests {
		if digest == want || digest == "sha256:"+want {
			return true
		}
	}
	return false
}
