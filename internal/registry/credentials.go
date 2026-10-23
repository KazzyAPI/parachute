package registry

import (
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
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
	byHost  map[string]Credential
	helpers map[string]string
	store   string
	warned  map[string]bool
	logger  *slog.Logger
}

type CredentialStore struct {
	path   string
	files  FileReader
	logger *slog.Logger
}

func NewCredentialStore(dockerConfigDir string, files FileReader, logger *slog.Logger) *CredentialStore {
	return &CredentialStore{path: filepath.Join(dockerConfigDir, "config.json"), files: files, logger: logger}
}

func (s *CredentialStore) Load() (Credentials, error) {
	credentials := Credentials{byHost: map[string]Credential{}, helpers: map[string]string{}, warned: map[string]bool{}, logger: s.logger}
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
		CredHelpers map[string]string `json:"credHelpers"`
		CredsStore  string            `json:"credsStore"`
	}
	if err := json.Unmarshal(contents, &document); err != nil {
		return credentials, fmt.Errorf("parse %s: %w", s.path, err)
	}
	credentials.store = document.CredsStore
	for key, helper := range document.CredHelpers {
		credentials.helpers[s.host(key)] = helper
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

func (s *CredentialStore) Secrets() []string {
	credentials, _ := s.Load()
	var secrets []string
	for _, credential := range credentials.byHost {
		if credential.Password != "" {
			secrets = append(secrets, credential.Password, credential.Username+":"+credential.Password)
		}
	}
	return secrets
}

func (s *CredentialStore) host(key string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(key, "https://"), "http://")
	host, _, _ := strings.Cut(trimmed, "/")
	return host
}

func (c Credentials) For(ref Reference) (Credential, bool) {
	hosts := []string{ref.Domain}
	if ref.IsDockerHub() {
		hosts = []string{"index.docker.io", "docker.io"}
	}
	for _, host := range hosts {
		if helper := cmp.Or(c.helpers[host], c.store); helper != "" {
			c.warnHelper(helper, ref.Domain)
			return Credential{}, false
		}
	}
	for _, host := range hosts {
		if credential, ok := c.byHost[host]; ok {
			return credential, true
		}
	}
	return Credential{}, false
}

func (c Credentials) warnHelper(helper, registry string) {
	if c.warned[registry] {
		return
	}
	c.warned[registry] = true
	c.logger.Warn("credential helpers are not supported, using anonymous access", "helper", helper, "registry", registry)
}
