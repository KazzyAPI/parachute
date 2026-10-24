package harness

func (e *Env) TmpfsMount(hostPath string) string {
	e.T.Helper()
	e.hostShell("mkdir -p " + hostPath + " && mount -t tmpfs -o size=2g tmpfs " + hostPath)
	e.T.Cleanup(func() {
		if _, err := e.DockerMayFail(hostShellArgs("umount " + hostPath + " && rmdir " + hostPath)...); err != nil {
			e.T.Logf("cleanup: unmount %s: %v", hostPath, err)
		}
	})
	return hostPath
}

func (e *Env) hostShell(script string) {
	e.T.Helper()
	e.Docker(hostShellArgs(script)...)
}

func hostShellArgs(script string) []string {
	return []string{"run", "--rm", "--privileged", "--pid=host", "--entrypoint", "nsenter", suite.fixtureV2Image, "-t", "1", "-m", "--", "sh", "-c", script}
}
