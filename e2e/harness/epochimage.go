package harness

import (
	"os"
	"path/filepath"
)

func (r *Registry) PushEpoch(repo, tag, version string) string {
	r.env.T.Helper()
	ref := r.Ref(repo, tag)
	context, err := os.MkdirTemp(r.env.Dir, "build-")
	if err != nil {
		r.env.T.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(context, "Dockerfile"), []byte(r.env.dockerfile(ImageSpec{Version: version})), 0o644); err != nil {
		r.env.T.Fatal(err)
	}
	r.env.Docker("build", "-q", "--provenance=false", "--build-arg", "SOURCE_DATE_EPOCH=0", "-t", ref, context)
	r.env.trackImage(ref)
	r.env.Docker("--config", r.cliConfig, "push", "-q", ref)
	return ref
}
