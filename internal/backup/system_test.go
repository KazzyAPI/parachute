package backup

import "testing"

func TestSystemMountExactVsPrefix(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/etc", true},
		{"/etc/myapp", false},
		{"/usr", true},
		{"/usr/local/share", false},
		{"/var", true},
		{"/var/log/app", false},
		{"/var/lib", true},
		{"/var/lib/postgresql/data", false},
		{"/boot/efi", false},
		{"/proc/self", true},
		{"/srv/app", false},
	}
	for _, tc := range cases {
		if got := systemMount(tc.path, ""); got != tc.want {
			t.Fatalf("systemMount(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
