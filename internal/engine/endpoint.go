package engine

import (
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/moby/moby/client"
)

type Endpoint struct {
	Host       string
	APIVersion string
	TLSVerify  bool
	CertPath   string
	UserAgent  string
}

func (e Endpoint) Scheme() (string, error) {
	parsed, err := url.Parse(e.Host)
	if err != nil {
		return "", fmt.Errorf("invalid Docker host %q: %w", e.Host, err)
	}
	if parsed.Scheme != "unix" && parsed.Scheme != "tcp" {
		return "", fmt.Errorf("unsupported Docker host scheme %q in %s: use unix:// or tcp://", parsed.Scheme, e.Host)
	}
	return parsed.Scheme, nil
}

func (e Endpoint) Plaintext() bool {
	scheme, err := e.Scheme()
	return err == nil && scheme == "tcp" && !e.TLSVerify
}

func (e Endpoint) clientOptions() ([]client.Opt, error) {
	scheme, err := e.Scheme()
	if err != nil {
		return nil, err
	}
	options := []client.Opt{client.WithHost(e.Host), client.WithUserAgent(e.UserAgent)}
	if e.APIVersion != "" {
		options = append(options, client.WithAPIVersion(e.APIVersion))
	}
	if scheme == "tcp" && e.TLSVerify {
		options = append(options, client.WithTLSClientConfig(
			filepath.Join(e.CertPath, "ca.pem"),
			filepath.Join(e.CertPath, "cert.pem"),
			filepath.Join(e.CertPath, "key.pem"),
		))
	}
	return options, nil
}
