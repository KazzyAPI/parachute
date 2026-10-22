package harness

import (
	"os"
	"path/filepath"
	"strings"
)

type ContainerOptions struct {
	Labels   map[string]string
	Unscoped bool
	Args     []string
	Extra    []string
}

func (e *Env) BuildImage(ref, version string) {
	e.T.Helper()
	context, err := os.MkdirTemp(e.Dir, "build-")
	if err != nil {
		e.T.Fatal(err)
	}
	dockerfile := "FROM " + suite.fixtureImage + "\nLABEL parachute-e2e.version=" + e.ID + "-" + version + "\n"
	if err := os.WriteFile(filepath.Join(context, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		e.T.Fatal(err)
	}
	e.Docker("build", "-q", "--provenance=false", "-t", ref, context)
	e.trackImage(ref)
}

func (e *Env) Tag(source, target string) string {
	e.T.Helper()
	e.Docker("tag", source, target)
	e.trackImage(target)
	return target
}

func (e *Env) Rehome(ref, host string) string {
	e.T.Helper()
	_, path, _ := strings.Cut(ref, "/")
	return e.Tag(ref, host+"/"+path)
}

func (e *Env) RunContainer(name, image string, options ContainerOptions) string {
	e.T.Helper()
	return e.container("run", name, image, options)
}

func (e *Env) CreateContainer(name, image string, options ContainerOptions) string {
	e.T.Helper()
	return e.container("create", name, image, options)
}

func (e *Env) container(verb, name, image string, options ContainerOptions) string {
	e.T.Helper()
	fullName := e.Name(name)
	args := []string{verb, "--name", fullName, "--label", "parachute-e2e=" + e.ID}
	if verb == "run" {
		args = append(args, "-d")
	}
	if !options.Unscoped {
		args = append(args, "--label", "parachute.scope="+e.Scope())
	}
	for key, value := range options.Labels {
		args = append(args, "--label", key+"="+value)
	}
	args = append(args, options.Extra...)
	args = append(args, image)
	e.Docker(append(args, options.Args...)...)
	return fullName
}

func (e *Env) ContainerState(name string) (id, image string) {
	e.T.Helper()
	fields := strings.Fields(e.Docker("inspect", "--format", "{{.Id}} {{.Image}}", name))
	return fields[0], fields[1]
}

func (e *Env) ImageList() string {
	e.T.Helper()
	var own []string
	for _, line := range strings.Split(e.Docker("image", "ls", "-a", "--digests", "--no-trunc", "--format", "{{.Repository}}:{{.Tag}}@{{.Digest}} {{.ID}}"), "\n") {
		if strings.Contains(line, e.ID) {
			own = append(own, line)
		}
	}
	return strings.Join(own, "\n")
}

func (e *Env) VolumeList() string {
	e.T.Helper()
	var own []string
	for _, name := range strings.Fields(e.Docker("volume", "ls", "-q")) {
		if strings.Contains(name, e.ID) || strings.HasPrefix(name, "parachute") {
			own = append(own, name)
		}
	}
	return strings.Join(own, "\n")
}
