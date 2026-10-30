package detect

import (
	"context"
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/registry"
)

type tagList []string

func (l tagList) Tags(context.Context, registry.Reference, *registry.Credential) ([]string, error) {
	return l, nil
}

type labelSet map[string]string

func (l labelSet) Value(_ domain.Container, name string) (string, bool) {
	value, ok := l[name]
	return value, ok
}

func TestTagFollowerTarget(t *testing.T) {
	tags := tagList{"1.2.0", "1.2.3", "1.3.0", "2.0.0", "1.2.4-rc1"}
	cases := []struct {
		image  string
		labels labelSet
		global string
		want   string
	}{
		{"localhost:5000/app:1.2.0", labelSet{"tag-follow": "patch"}, "none", "localhost:5000/app:1.2.3"},
		{"localhost:5000/app:1.2.0", nil, "minor", "localhost:5000/app:1.3.0"},
		{"localhost:5000/app:1.2.0", labelSet{"tag-follow": "none"}, "minor", ""},
		{"localhost:5000/app:latest", labelSet{"tag-follow": "minor"}, "none", ""},
		{"localhost:5000/app:1.2.0", nil, "none", ""},
	}
	for _, c := range cases {
		got, err := NewTagFollower(tags, c.labels, c.global).Target(context.Background(), domain.Container{ImageRef: c.image}, registry.Credentials{})
		if err != nil || got != c.want {
			t.Errorf("%s: got %q err %v, want %q", c.image, got, err, c.want)
		}
	}
}
