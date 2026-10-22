package registry

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

type Credential struct {
	Username string
	Password string
}

type FileReader interface {
	ReadFile(path string) ([]byte, error)
}

type Credentials struct {
	byHost map[string]Credential
}

type CredentialStore struct {
	path  string
	files FileReader
}

func NewCredentialStore(dockerConfigDir string, files FileReader) *CredentialStore {
	return &CredentialStore{path: filepath.Join(dockerConfigDir, "config.json"), files: files}
}

func (s *CredentialStore) Load() (Credentials, error) {
	credentials := Credentials{byHost: map[string]Credential{}}
	contents, err := s.files.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return credentials, nil
	}
	if err != nil {
		return credentials, fmt.Errorf("read %s: %w", s.path, err)
	}
	var document struct {
		Auths map[string]struct {
			Auth     string `json:"auth"`
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(contents, &document); err != nil {
		return credentials, fmt.Errorf("parse %s: %w", s.path, err)
	}
	for key, entry := range document.Auths {
		credential := Credential{Username: entry.Username, Password: entry.Password}
		if entry.Auth != "" {
			decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
			if err != nil {
				return credentials, fmt.Errorf("parse %s: invalid auth for %s", s.path, s.host(key))
			}
			credential.Username, credential.Password, _ = strings.Cut(string(decoded), ":")
		}
		credentials.byHost[s.host(key)] = credential
	}
	return credentials, nil
}

func (s *CredentialStore) host(key string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(key, "https://"), "http://")
	host, _, _ := strings.Cut(trimmed, "/")
	return host
}

func (c Credentials) For(ref Reference) (Credential, bool) {
	if ref.IsDockerHub() {
		for _, host := range []string{"index.docker.io", "docker.io"} {
			if credential, ok := c.byHost[host]; ok {
				return credential, true
			}
		}
		return Credential{}, false
	}
	credential, ok := c.byHost[ref.Domain]
	return credential, ok
}
