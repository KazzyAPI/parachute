package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const busyboxDigest = "sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e"

type Suite struct {
	repoRoot       string
	workDir        string
	binary         string
	dockerHost     string
	desktop        bool
	fixtureImage   string
	fixtureV2Image string
	runID          string
	imageOnce      sync.Once
	v2Once         sync.Once
	v2Err          error
	image          string
	imageErr       error
}

var suite *Suite

func Main(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e setup failed:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	workDir, err := os.MkdirTemp("", "parachute-e2e-")
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := os.RemoveAll(workDir); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
		}
	}()
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		return 0, err
	}
	s := &Suite{repoRoot: repoRoot, workDir: workDir, runID: randomID()}
	if err := s.prepare(); err != nil {
		return 0, err
	}
	suite = s
	defer s.removeRunResources()
	return m.Run(), nil
}

func (s *Suite) prepare() error {
	s.binary = filepath.Join(s.workDir, "parachute")
	if out, err := s.command(s.repoRoot, nil, "go", "build", "-o", s.binary, "./cmd/parachute"); err != nil {
		return fmt.Errorf("build parachute: %w: %s", err, out)
	}
	host, err := s.command("", nil, "docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}")
	if err != nil {
		return fmt.Errorf("find Docker endpoint: %w: %s", err, host)
	}
	s.dockerHost = strings.TrimSpace(host)
	if value := os.Getenv("DOCKER_HOST"); value != "" {
		s.dockerHost = value
	}
	osName, err := s.command("", nil, "docker", "info", "--format", "{{.OperatingSystem}}")
	if err != nil {
		return fmt.Errorf("docker info: %w: %s", err, osName)
	}
	s.desktop = strings.Contains(osName, "Docker Desktop")
	return s.buildFixture()
}

func (s *Suite) FixtureV2Image() (string, error) {
	s.v2Once.Do(func() { s.v2Err = s.buildFixtureV2() })
	return s.fixtureV2Image, s.v2Err
}

func (s *Suite) buildFixture() error {
	arch, err := s.serverArch()
	if err != nil {
		return err
	}
	context := filepath.Join(s.workDir, "fixture")
	if err := os.MkdirAll(context, 0o755); err != nil {
		return err
	}
	env := []string{"GOOS=linux", "GOARCH=" + arch, "CGO_ENABLED=0"}
	if out, err := s.command(filepath.Join(s.repoRoot, "e2e"), env, "go", "build", "-o", filepath.Join(context, "fixture"), "./fixture"); err != nil {
		return fmt.Errorf("build fixture: %w: %s", err, out)
	}
	dockerfile := "FROM scratch\nCOPY fixture /fixture\nARG VERSION=base\nLABEL parachute-e2e.version=$VERSION\nENTRYPOINT [\"/fixture\"]\n"
	if err := os.WriteFile(filepath.Join(context, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return err
	}
	s.fixtureImage = "parachute-e2e-fixture:" + s.runID
	if out, err := s.command(context, nil, "docker", "build", "-q", "--label", "parachute-e2e.run="+s.runID, "-t", s.fixtureImage, "."); err != nil {
		return fmt.Errorf("build fixture image: %w: %s", err, out)
	}
	return nil
}

func (s *Suite) serverArch() (string, error) {
	arch, err := s.command("", nil, "docker", "version", "--format", "{{.Server.Arch}}")
	if err != nil {
		return "", fmt.Errorf("docker version: %w: %s", err, arch)
	}
	return strings.TrimSpace(arch), nil
}

func (s *Suite) buildFixtureV2() error {
	if out, err := s.command("", nil, "docker", "pull", "busybox@"+busyboxDigest); err != nil {
		return fmt.Errorf("pull busybox: %w: %s", err, out)
	}
	arch, err := s.serverArch()
	if err != nil {
		return err
	}
	context := filepath.Join(s.workDir, "fixturev2")
	if err := os.MkdirAll(context, 0o755); err != nil {
		return err
	}
	env := []string{"GOOS=linux", "GOARCH=" + arch, "CGO_ENABLED=0"}
	if out, err := s.command(filepath.Join(s.repoRoot, "e2e"), env, "go", "build", "-o", filepath.Join(context, "fixturev2"), "./fixturev2"); err != nil {
		return fmt.Errorf("build fixturev2: %w: %s", err, out)
	}
	dockerfile := "FROM busybox@" + busyboxDigest + "\nCOPY fixturev2 /fixturev2\nARG VERSION=base\nLABEL parachute-e2e.version=$VERSION\nENTRYPOINT [\"/fixturev2\"]\n"
	if err := os.WriteFile(filepath.Join(context, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return err
	}
	s.fixtureV2Image = "parachute-e2e-fixturev2:" + s.runID
	if out, err := s.command(context, nil, "docker", "build", "-q", "--label", "parachute-e2e.run="+s.runID, "-t", s.fixtureV2Image, "."); err != nil {
		return fmt.Errorf("build fixturev2 image: %w: %s", err, out)
	}
	return nil
}

func (s *Suite) ParachuteImage() (string, error) {
	s.imageOnce.Do(func() {
		s.image = "parachute-e2e:" + s.runID
		out, err := s.command(s.repoRoot, nil, "docker", "build", "-q", "--label", "parachute-e2e.run="+s.runID, "-t", s.image, ".")
		if err != nil {
			s.imageErr = fmt.Errorf("build parachute image: %w: %s", err, out)
		}
	})
	return s.image, s.imageErr
}

func (s *Suite) removeRunResources() {
	images, err := s.command("", nil, "docker", "images", "-q", "--filter", "label=parachute-e2e.run="+s.runID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cleanup:", err, images)
		return
	}
	for _, image := range strings.Fields(images) {
		if out, err := s.command("", nil, "docker", "rmi", "-f", image); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err, out)
		}
	}
}

func (s *Suite) command(dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
