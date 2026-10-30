package harness

import (
	"path/filepath"
	"strings"
)

func (e *Env) hostHelper(mounts []string, entrypoint string, args ...string) string {
	e.T.Helper()
	command := []string{"run", "--rm", "--label", "parachute-e2e=" + e.ID, "--entrypoint", entrypoint}
	for _, mount := range mounts {
		command = append(command, "-v", mount)
	}
	command = append(command, suite.FixtureV2Image())
	return e.Docker(append(command, args...)...)
}

func (e *Env) ReadHostPath(hostPath string) string {
	e.T.Helper()
	return strings.TrimSpace(e.hostHelper([]string{hostPath + ":" + hostPath + ":ro"}, "cat", hostPath))
}

func (e *Env) WriteHostPath(hostPath, contents string) {
	e.T.Helper()
	dir := filepath.Dir(hostPath)
	payload := e.WriteFile("hostfs-payload-"+e.Name(filepath.Base(hostPath)), contents, 0o644)
	e.hostHelper([]string{dir + ":" + dir, payload + ":/payload:ro"}, "sh", "-c", "cp /payload "+hostPath)
}
