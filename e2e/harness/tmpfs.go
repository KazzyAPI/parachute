package harness

import (
	"os"
)

func (e *Env) TmpfsMount(hostPath string) string {
	e.T.Helper()
	if err := os.MkdirAll(hostPath, 0o755); err != nil {
		e.T.Fatal(err)
	}
	name := e.Name("tmpfs")
	e.Docker("run", "-d", "--privileged", "--name", name, "--label", "parachute-e2e="+e.ID,
		"-v", hostPath+":"+hostPath, "--tmpfs", hostPath+":rw,size=2g", suite.fixtureV2Image, "sleep", "infinity")
	return hostPath
}
