package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

const Format = "parachute.state/v1"

type CycleInfo struct {
	ID      string    `json:"id"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended"`
}

type Document struct {
	Format    string          `json:"format"`
	LastCycle *CycleInfo      `json:"last_cycle"`
	Results   []domain.Result `json:"results"`
}

type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteAtomic(path string, data []byte) error
}

type Store struct {
	path  string
	files FileSystem
}

func NewStore(directory string, files FileSystem) *Store {
	return &Store{path: filepath.Join(directory, "state.json"), files: files}
}

func (s *Store) Path() string { return s.path }

func (s *Store) Open() (Document, error) {
	document, found, err := s.Read()
	if err != nil {
		return Document{}, err
	}
	if !found {
		document = Document{Format: Format, Results: []domain.Result{}}
	}
	if err := s.Save(document); err != nil {
		return Document{}, err
	}
	return document, nil
}

func (s *Store) Read() (Document, bool, error) {
	contents, err := s.files.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return Document{}, false, nil
	}
	if err != nil {
		return Document{}, false, fmt.Errorf("cannot read state file %s: %w", s.path, err)
	}
	var document Document
	if err := json.Unmarshal(contents, &document); err != nil {
		return Document{}, false, fmt.Errorf("state file %s is not valid JSON: %w", s.path, err)
	}
	if document.Format != Format {
		return Document{}, false, fmt.Errorf("state file %s has format %q, expected %q", s.path, document.Format, Format)
	}
	return document, true, nil
}

func (s *Store) Save(document Document) error {
	contents, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state file %s: %w", s.path, err)
	}
	if err := s.files.WriteAtomic(s.path, append(contents, '\n')); err != nil {
		return fmt.Errorf("cannot write state file %s: %w", s.path, err)
	}
	return nil
}
