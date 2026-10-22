package harness

import "time"

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
	if len(run.Args) > 0 && !isCommand(run.Args[0]) {
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

func (e *Env) RunParachuteContainer(name string, run ContainerRun, timeout time.Duration) *Result {
	e.T.Helper()
	return e.runCommand(timeout, nil, "docker", e.containerArgs(name, run)...)
}
