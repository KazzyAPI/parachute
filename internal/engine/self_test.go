package engine

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type fakeFiles map[string]string

func (f fakeFiles) ReadFile(path string) ([]byte, error) {
	if contents, ok := f[path]; ok {
		return []byte(contents), nil
	}
	return nil, fs.ErrNotExist
}

func (f fakeFiles) Stat(path string) (fs.FileInfo, error) {
	if _, ok := f[path]; ok {
		return nil, nil
	}
	return nil, fs.ErrNotExist
}

func (f fakeFiles) Hostname() (string, error) {
	if hostname, ok := f["hostname"]; ok {
		return hostname, nil
	}
	return "", errors.New("no hostname")
}

func TestProbeSelfSources(t *testing.T) {
	own, other := strings.Repeat("a", 64), strings.Repeat("b", 64)
	cases := map[string]struct {
		files fakeFiles
		local bool
		want  string
	}{
		"mountinfo":                 {files: fakeFiles{"/proc/self/mountinfo": "1 2 0:1 /data/docker/containers/" + own + "/hostname /etc/hostname rw - ext4 /dev/vda1 rw", "hostname": "parachute-main"}, want: own},
		"cgroup":                    {files: fakeFiles{"/proc/self/cgroup": "0::/system.slice/docker-" + own + ".scope"}, want: own},
		"hostname":                  {files: fakeFiles{"hostname": own[:12]}, local: true, want: own},
		"remote":                    {files: fakeFiles{"hostname": own[:12]}},
		"unconfirmed falls through": {files: fakeFiles{"/proc/self/cgroup": "0::/docker/" + strings.Repeat("c", 64), "hostname": other[:12]}, local: true, want: other},
		"custom hostname":           {files: fakeFiles{"hostname": "parachute-main", "/.dockerenv": ""}, local: true},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			containers := []domain.Container{{ID: own, State: "running"}, {ID: other, State: "running"}, {ID: own[:10] + "ff", State: "exited"}}
			self := ProbeSelf(test.files)
			self.Mark([]SelfHost{{Local: test.local, Containers: containers}})
			got := ""
			for _, container := range containers {
				if container.Own {
					got += container.ID
				}
			}
			if got != test.want {
				t.Errorf("own %q, want %q", got, test.want)
			}
		})
	}
}

func TestMarkDiscardsIDConfirmedByTwoHosts(t *testing.T) {
	own := strings.Repeat("a", 64)
	self := ProbeSelf(fakeFiles{"/proc/self/cgroup": "0::/docker/" + own})
	first, second := []domain.Container{{ID: own, State: "running"}}, []domain.Container{{ID: own, State: "running"}}
	if self.Mark([]SelfHost{{Containers: first}, {Containers: second}}) || first[0].Own || second[0].Own {
		t.Error("an ID confirmed by two engines must be discarded")
	}
}
