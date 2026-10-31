package backup

import (
	"path/filepath"
	"strings"
)

func systemExactRoots() []string {
	return []string{"/", "/etc", "/usr", "/boot", "/var", "/var/lib"}
}

func systemPrefixRoots() []string {
	return []string{"/proc", "/sys", "/dev", "/run", "/var/run", "/var/lib/docker", "/var/lib/containers"}
}

func systemMount(resolved, storePath string) bool {
	clean := filepath.Clean(resolved)
	if storePath != "" {
		store := filepath.Clean(storePath)
		if clean == store || strings.HasPrefix(clean+string(filepath.Separator), store+string(filepath.Separator)) {
			return true
		}
		if strings.HasPrefix(store+string(filepath.Separator), clean+string(filepath.Separator)) {
			return true
		}
	}
	sep := string(filepath.Separator)
	for _, root := range systemExactRoots() {
		if clean == root {
			return true
		}
	}
	for _, root := range systemPrefixRoots() {
		if clean == root || strings.HasPrefix(clean+sep, root+sep) {
			return true
		}
	}
	if strings.HasSuffix(clean, "docker.sock") || strings.Contains(clean, ".sock") && strings.Contains(clean, "docker") {
		return true
	}
	return false
}
