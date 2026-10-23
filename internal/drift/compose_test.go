package drift

import (
	"errors"
	"testing"
)

type files map[string]string

func (f files) ReadRegularFile(path string, _ int64) ([]byte, error) {
	contents, ok := f[path]
	if !ok {
		return nil, errors.New("missing")
	}
	return []byte(contents), nil
}

func TestDetectorDrift(t *testing.T) {
	detector := NewDetector(files{
		"/a.yml": "x-base: &base\n  image: reg.test/app:2\nservices:\n  web:\n    <<: *base\n  db:\n    image: postgres\n---\nservices:\n  api:\n    image: reg.test/api:${TAG}\n",
		"/b.yml": "services:\n  web:\n    environment: [A=1]\n  db:\n    image: postgres:17\n",
	})
	for _, test := range []struct {
		files, service, running string
		want                    string
	}{
		{"/a.yml", "web", "reg.test/app:1", "compose reg.test/app:2, running reg.test/app:1"},
		{"/a.yml", "web", "reg.test/app:2", ""},
		{"/a.yml", "db", "docker.io/library/postgres:latest", ""},
		{"/a.yml", "api", "reg.test/api:1", Unknown},
		{"/a.yml,/b.yml", "web", "reg.test/app:2", ""},
		{"/a.yml, /b.yml", "db", "postgres:16", "compose postgres:17, running postgres:16"},
		{"/a.yml", "cache", "redis", ""},
		{"/missing.yml,/a.yml", "web", "reg.test/app:1", ""},
		{"a.yml", "web", "reg.test/app:1", ""},
	} {
		got := detector.Drift(test.files, test.service, test.running)
		if (got == nil) != (test.want == "") || got != nil && *got != test.want {
			t.Errorf("Drift(%q, %q, %q) = %v, want %q", test.files, test.service, test.running, got, test.want)
		}
	}
}
