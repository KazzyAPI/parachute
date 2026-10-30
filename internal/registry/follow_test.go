package registry

import "testing"

func TestFollowTag(t *testing.T) {
	tags := []string{"1.2.0", "1.2.3", "1.3.0", "2.0.0", "1.2.4-rc1", "latest", "1.10.0"}
	cases := []struct{ current, mode, want string }{
		{"1.2.0", "patch", "1.2.3"},
		{"1.2.0", "minor", "1.10.0"},
		{"v1.2.0", "patch", "1.2.3"},
		{"1.10.0", "minor", ""},
		{"latest", "minor", ""},
		{"1.2.0", "none", ""},
	}
	for _, c := range cases {
		if got, moved := FollowTag(c.current, c.mode, tags); got != c.want && moved || moved != (c.want != "") {
			t.Errorf("%s %s: got %q moved %v, want %q", c.current, c.mode, got, moved, c.want)
		}
	}
}
