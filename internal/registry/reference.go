package registry

import (
	"fmt"
	"net"
	"strings"

	"github.com/distribution/reference"
)

const dockerHubDomain = "docker.io"

type Reference struct {
	Domain     string
	Repository string
	Tag        string
}

func ParseReference(raw string) (Reference, error) {
	named, err := reference.ParseNormalizedNamed(raw)
	if err != nil {
		return Reference{}, fmt.Errorf("invalid image reference %q: %w", raw, err)
	}
	tagged, ok := reference.TagNameOnly(named).(reference.Tagged)
	if !ok {
		return Reference{}, fmt.Errorf("image reference %q has no tag", raw)
	}
	return Reference{Domain: reference.Domain(named), Repository: reference.Path(named), Tag: tagged.Tag()}, nil
}

func IsDigestPinned(raw string) bool {
	return strings.Contains(raw, "@sha256:")
}

func (r Reference) IsDockerHub() bool {
	return r.Domain == dockerHubDomain
}

func (r Reference) APIHost() string {
	if r.IsDockerHub() {
		return "registry-1.docker.io"
	}
	return r.Domain
}

func (r Reference) Scheme() string {
	if isLoopback(r.Domain) {
		return "http"
	}
	return "https"
}

func isLoopback(host string) bool {
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (r Reference) ManifestURL() string {
	return r.Scheme() + "://" + r.APIHost() + "/v2/" + r.Repository + "/manifests/" + r.Tag
}
