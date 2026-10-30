package app

import "testing"

func TestVER_005_probe_status_from_wget_output(t *testing.T) {
	cases := []struct {
		name   string
		exit   int
		output string
		status int
		failed bool
	}{
		{"ok with headers", 0, "  HTTP/1.1 200 OK\n  Content-Length: 2", 200, false},
		{"ok without headers", 0, "", 200, false},
		{"server error", 1, "  HTTP/1.1 500 Internal Server Error\nwget: server returned error: HTTP/1.1 500", 500, false},
		{"redirect then ok", 0, "  HTTP/1.1 302 Found\n  HTTP/1.1 200 OK", 200, false},
		{"refused", 1, "wget: can't connect to remote host (127.0.0.1): Connection refused", 0, true},
	}
	for _, c := range cases {
		status, err := probeStatus(c.exit, c.output)
		if status != c.status || (err != nil) != c.failed {
			t.Errorf("%s: status %d err %v", c.name, status, err)
		}
	}
}
