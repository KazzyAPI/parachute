package harness

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type ImageSpec struct {
	Version     string
	Labels      map[string]string
	Env         map[string]string
	Volumes     []string
	Healthcheck []string
	Cmd         []string
	BuildArgs   map[string]string
}

func (e *Env) BuildImage(ref, version string) {
	e.T.Helper()
	e.BuildImageSpec(ref, ImageSpec{Version: version})
}

func (e *Env) BuildImageSpec(ref string, spec ImageSpec) {
	e.T.Helper()
	context, err := os.MkdirTemp(e.Dir, "build-")
	if err != nil {
		e.T.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(context, "Dockerfile"), []byte(e.dockerfile(spec)), 0o644); err != nil {
		e.T.Fatal(err)
	}
	if _, err := e.DockerMayFail("image", "inspect", ref); err == nil {
		previous := ref + "-previous-" + randomID()
		e.Docker("tag", ref, previous)
		e.trackImage(previous)
	}
	args := []string{"build", "-q", "--provenance=false", "-t", ref}
	for _, key := range slices.Sorted(maps.Keys(spec.BuildArgs)) {
		args = append(args, "--build-arg", key+"="+spec.BuildArgs[key])
	}
	e.Docker(append(args, context)...)
	e.trackImage(ref)
}

func (e *Env) dockerfile(spec ImageSpec) string {
	var file strings.Builder
	fmt.Fprintf(&file, "FROM %s\nLABEL parachute-e2e.version=%s-%s\n", suite.fixtureImage, e.ID, spec.Version)
	for _, key := range slices.Sorted(maps.Keys(spec.Labels)) {
		fmt.Fprintf(&file, "LABEL %q=%q\n", key, spec.Labels[key])
	}
	for _, key := range slices.Sorted(maps.Keys(spec.Env)) {
		fmt.Fprintf(&file, "ENV %s=%q\n", key, spec.Env[key])
	}
	if len(spec.Volumes) > 0 {
		fmt.Fprintf(&file, "VOLUME %s\n", e.execForm(spec.Volumes))
	}
	if len(spec.Healthcheck) > 0 {
		fmt.Fprintf(&file, "HEALTHCHECK --interval=1s --timeout=5s --retries=3 CMD %s\n", e.execForm(spec.Healthcheck))
	}
	if len(spec.Cmd) > 0 {
		fmt.Fprintf(&file, "CMD %s\n", e.execForm(spec.Cmd))
	}
	return file.String()
}

func (e *Env) execForm(items []string) string {
	encoded, err := json.Marshal(items)
	if err != nil {
		e.T.Fatal(err)
	}
	return string(encoded)
}

func (r *Registry) PushSpec(repo, tag string, spec ImageSpec) string {
	r.env.T.Helper()
	ref := r.Ref(repo, tag)
	r.env.BuildImageSpec(ref, spec)
	r.env.Docker("--config", r.cliConfig, "push", "-q", ref)
	return ref
}
