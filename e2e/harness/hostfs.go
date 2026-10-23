package harness

import (
	"path/filepath"
	"strings"
)

func (e *Env) ReadHostPath(hostPath string) string {
	e.T.Helper()
	return strings.TrimSpace(e.Docker("run", "--rm", "-v", hostPath+":"+hostPath+":ro", suite.fixtureV2Image, "cat", hostPath))
}

func (e *Env) WriteHostPath(hostPath, contents string) {
	e.T.Helper()
	dir := filepath.Dir(hostPath)
	name := filepath.Base(hostPath)
	payload := e.WriteFile("hostfs-payload", contents, 0o644)
	e.Docker("run", "--rm", "-v", dir+":"+dir, "-v", payload+":/payload:ro", suite.fixtureV2Image,
		"cp", "/payload", filepath.Join(dir, name))
}
