package config

import (
	"fmt"
	"regexp"
	"strings"
)

func (l *Loader) hosts(b *binder) ([]HostSpec, error) {
	valid := regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	seen := map[string]bool{}
	var hosts []HostSpec
	for _, entry := range b.list("hosts") {
		name, endpoint, found := strings.Cut(entry, "=")
		if !found || endpoint == "" {
			return nil, fmt.Errorf("invalid --hosts entry %q: use name=endpoint", entry)
		}
		if !valid.MatchString(name) {
			return nil, fmt.Errorf("invalid host name %q in --hosts: names match [a-z0-9][a-z0-9-]{0,31}", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate host name %q in --hosts", name)
		}
		seen[name] = true
		hosts = append(hosts, HostSpec{Name: name, Endpoint: endpoint})
	}
	return hosts, nil
}
