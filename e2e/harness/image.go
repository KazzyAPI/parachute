package harness

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

type ContainerRun struct {
	Args   []string
	Env    map[string]string
	Mounts []string
	Labels map[string]string
	Detach bool
}

func (e *Env) ParachuteImage() string {
	e.T.Helper()
	image, err := suite.ParachuteImage()
	if err != nil {
		e.T.Fatal(err)
	}
	return image
}

func (e *Env) containerArgs(name string, run ContainerRun) []string {
	args := []string{"run", "--name", e.Name(name), "--label", "parachute-e2e=" + e.ID,
		"-v", "/var/run/docker.sock:/var/run/docker.sock", "--add-host", "host.docker.internal:host-gateway"}
	if run.Detach {
		args = append(args, "-d")
	}
	for key, value := range run.Env {
		args = append(args, "-e", key+"="+value)
	}
	for _, mount := range run.Mounts {
		args = append(args, "-v", mount)
	}
	for key, value := range run.Labels {
		args = append(args, "--label", key+"="+value)
	}
	args = append(args, e.ParachuteImage())
	if len(run.Args) > 0 && run.Args[0] != "healthcheck" && run.Args[0] != "status" {
		args = append(args, "--log-format", "json")
	}
	return append(args, run.Args...)
}

func (e *Env) StartParachuteContainer(name string, run ContainerRun) string {
	e.T.Helper()
	run.Detach = true
	e.Docker(e.containerArgs(name, run)...)
	return e.Name(name)
}

func (e *Env) RunParachuteContainer(name string, run ContainerRun) *Result {
	e.T.Helper()
	cmd := exec.Command("docker", e.containerArgs(name, run)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	result := &Result{env: e, Stdout: stdout.String(), Stderr: stderr.String()}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.Code = exitErr.ExitCode()
	} else if err != nil {
		e.T.Fatalf("docker run: %v", err)
	}
	e.T.Logf("parachute container exited %d\nstderr:\n%s", result.Code, strings.TrimSpace(result.Stderr))
	return result
}
