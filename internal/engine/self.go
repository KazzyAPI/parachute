package engine

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type SelfFiles interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (fs.FileInfo, error)
	Hostname() (string, error)
}

type SelfHost struct {
	Local      bool
	Containers []domain.Container
}

type Self struct {
	ids       []string
	hostname  string
	contained bool
}

func ProbeSelf(files SelfFiles) Self {
	mountinfo, _ := files.ReadFile("/proc/self/mountinfo")
	cgroup, _ := files.ReadFile("/proc/self/cgroup")
	self := Self{ids: append(mountinfoIDs(string(mountinfo)), cgroupIDs(string(cgroup))...)}
	if hostname, err := files.Hostname(); err == nil && regexp.MustCompile(`^([0-9a-f]{12}|[0-9a-f]{64})$`).MatchString(hostname) {
		self.hostname = hostname
	}
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := files.Stat(marker); err == nil {
			self.contained = true
		}
	}
	return self
}

func mountinfoIDs(mountinfo string) []string {
	root := regexp.MustCompile(`/containers/([0-9a-f]{64})/(hostname|hosts|resolv\.conf)$`)
	var ids []string
	for _, line := range strings.Split(mountinfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !slices.Contains([]string{"/etc/hostname", "/etc/hosts", "/etc/resolv.conf"}, fields[4]) {
			continue
		}
		if match := root.FindStringSubmatch(fields[3]); match != nil {
			ids = append(ids, match[1])
		}
	}
	return ids
}

func cgroupIDs(cgroup string) []string {
	pattern := regexp.MustCompile(`(?:^|[/-])([0-9a-f]{64})(?:\.scope)?(?:/|$)`)
	var ids []string
	for _, line := range strings.Split(cgroup, "\n") {
		fields := strings.SplitN(line, ":", 3)
		if len(fields) < 3 {
			continue
		}
		if match := pattern.FindStringSubmatch(fields[2]); match != nil {
			ids = append(ids, match[1])
		}
	}
	return ids
}

func LocalEndpoint(endpoint string) bool {
	return strings.HasPrefix(endpoint, "unix://")
}

func (s Self) Contained() bool { return s.contained }

func (s Self) Mark(hosts []SelfHost) bool {
	for _, id := range s.ids {
		if markOne(hosts, false, func(container domain.Container) bool { return container.ID == id }) {
			return true
		}
	}
	return s.hostname != "" && markOne(hosts, true, func(container domain.Container) bool {
		return strings.HasPrefix(container.ID, s.hostname)
	})
}

func markOne(hosts []SelfHost, localOnly bool, match func(domain.Container) bool) bool {
	var found []*domain.Container
	for _, host := range hosts {
		if localOnly && !host.Local {
			continue
		}
		for index := range host.Containers {
			if container := &host.Containers[index]; container.State == "running" && match(*container) {
				found = append(found, container)
			}
		}
	}
	if len(found) != 1 {
		return false
	}
	found[0].Own = true
	return true
}
