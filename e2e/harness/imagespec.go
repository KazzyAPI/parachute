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
	Base                   string
	Version                string
	Verify                 bool
	Labels                 map[string]string
	Env                    map[string]string
	Volumes                []string
	Healthcheck            []string
	HealthcheckInterval    string
	HealthcheckStartPeriod string
	Cmd                    []string
	BuildArgs              map[string]string
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
	if spec.Verify {
		if err := copyFile(suite.verifyFixtureBinary(), filepath.Join(context, "fixture")); err != nil {
			e.T.Fatal(err)
		}
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
	base := spec.Base
	switch {
	case base == "" && spec.Verify:
		base = "alpine:3.22@sha256:d406446bc05b82480756ce8f1287964d28768c23955fa9480d2a74a2eae7a654"
	case base == "":
		base = suite.fixtureImage
	}
	fmt.Fprintf(&file, "FROM %s\n", base)
	if spec.Verify {
		fmt.Fprintln(&file, "COPY fixture /fixture")
	}
	fmt.Fprintf(&file, "LABEL parachute-e2e.version=%s-%s\n", e.ID, spec.Version)
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
		interval := "1s"
		if spec.HealthcheckInterval != "" {
			interval = spec.HealthcheckInterval
		}
		startPeriod := ""
		if spec.HealthcheckStartPeriod != "" {
			startPeriod = " --start-period=" + spec.HealthcheckStartPeriod
		}
		command := spec.Healthcheck
		if command[0] == "CMD" {
			command = command[1:]
		}
		fmt.Fprintf(&file, "HEALTHCHECK --interval=%s --timeout=5s --retries=3%s CMD %s\n", interval, startPeriod, e.execForm(command))
	}
	if len(spec.Cmd) > 0 {
		fmt.Fprintf(&file, "CMD %s\n", e.execForm(spec.Cmd))
	} else if spec.Verify {
		fmt.Fprintf(&file, "ENTRYPOINT [\"/fixture\"]\n")
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

func copyFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o755)
}

func (r *Registry) PushSpec(repo, tag string, spec ImageSpec) string {
	r.env.T.Helper()
	ref := r.Ref(repo, tag)
	r.env.BuildImageSpec(ref, spec)
	r.env.Docker("--config", r.cliConfig, "push", "-q", ref)
	return ref
}
