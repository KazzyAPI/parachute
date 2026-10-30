package registry

import (
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
)

type Credential struct {
	Username string
	Password string
}

type FileReader interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (fs.FileInfo, error)
}

type Credentials struct {
	byHost  map[string]Credential
	helpers map[string]string
	store   string
}

type Helper struct {
	Name     string
	Registry string
}

type CredentialStore struct {
	path  string
	files FileReader
	lock  sync.Mutex
	seen  string
	cache []string
}

func NewCredentialStore(dockerConfigDir string, files FileReader) *CredentialStore {
	return &CredentialStore{path: filepath.Join(dockerConfigDir, "config.json"), files: files}
}

func (s *CredentialStore) Load() (Credentials, error) {
	credentials := Credentials{byHost: map[string]Credential{}, helpers: map[string]string{}}
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
	s.lock.Lock()
	defer s.lock.Unlock()
	version := "absent"
	if info, err := s.files.Stat(s.path); err == nil {
		version = fmt.Sprint(info.ModTime().UnixNano(), info.Size())
	}
	if version == s.seen {
		return s.cache
	}
	s.seen = version
	credentials, err := s.Load()
	if err != nil {
		return s.cache
	}
	s.cache = nil
	for _, credential := range credentials.byHost {
		if credential.Password != "" {
			s.cache = append(s.cache, credential.Password, credential.Username+":"+credential.Password)
		}
	}
	return s.cache
}

func (s *CredentialStore) host(key string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(key, "https://"), "http://")
	host, _, _ := strings.Cut(trimmed, "/")
	return host
}

func (c Credentials) For(ref Reference) (*Credential, Helper) {
	hosts := []string{ref.Domain}
	if ref.IsDockerHub() {
		hosts = []string{"index.docker.io", "docker.io"}
	}
	for _, host := range hosts {
		if helper := cmp.Or(c.helpers[host], c.store); helper != "" {
			return nil, Helper{Name: helper, Registry: ref.Domain}
		}
	}
	for _, host := range hosts {
		if credential, ok := c.byHost[host]; ok {
			return &credential, Helper{}
		}
	}
	return nil, Helper{}
}

func (c Credentials) PullAuth(ref Reference) string {
	credential, _ := c.For(ref)
	if credential == nil {
		return ""
	}
	encoded, err := json.Marshal(map[string]string{"username": credential.Username, "password": credential.Password, "serveraddress": ref.Domain})
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(encoded)
}
