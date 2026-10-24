package backup

import (
	"path/filepath"
	"strings"
)

func systemRoots() []string {
	return []string{"/", "/etc", "/usr", "/boot", "/var", "/var/lib", "/proc", "/sys", "/dev", "/run", "/var/run", "/var/lib/docker", "/var/lib/containers"}
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
	for _, root := range systemRoots() {
		if clean == root || strings.HasPrefix(clean+string(filepath.Separator), root+string(filepath.Separator)) {
			return true
		}
	}
	if strings.HasSuffix(clean, "docker.sock") || strings.Contains(clean, ".sock") && strings.Contains(clean, "docker") {
		return true
	}
	return false
}
