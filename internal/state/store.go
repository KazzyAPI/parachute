package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sync"
)

type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteAtomic(path string, data []byte) error
}

type Store struct {
	path    string
	files   FileSystem
	lock    sync.Mutex
	current Document
}

func NewStore(directory string, files FileSystem) *Store {
	return &Store{path: filepath.Join(directory, "state.json"), files: files}
}

func (s *Store) Open() error {
	document, _, err := s.Read()
	if err != nil {
		return err
	}
	s.lock.Lock()
	s.current = document
	s.lock.Unlock()
	return s.Update(func(*Document) error { return nil })
}

func (s *Store) Read() (Document, bool, error) {
	contents, err := s.files.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return Document{}, false, nil
	}
	if err != nil {
		return Document{}, false, fmt.Errorf("cannot read state file %s: %w", s.path, err)
	}
	document, err := s.decode(contents)
	if err != nil {
		return Document{}, false, err
	}
	return document, true, nil
}

func (s *Store) decode(contents []byte) (Document, error) {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, fmt.Errorf("state file %s is not valid %s JSON: %w", s.path, Format, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Document{}, fmt.Errorf("state file %s is not valid %s JSON: trailing data", s.path, Format)
	}
	if document.Format != Format {
		return Document{}, fmt.Errorf("state file %s has format %q, expected %q", s.path, document.Format, Format)
	}
	return document.discardInvalid(), nil
}

func (s *Store) Current() Document {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.current.clone()
}

func (s *Store) Update(change func(*Document) error) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	document := s.current.clone()
	if err := change(&document); err != nil {
		return fmt.Errorf("update state file %s: %w", s.path, err)
	}
	contents, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state file %s: %w", s.path, err)
	}
	if err := s.files.WriteAtomic(s.path, append(contents, '\n')); err != nil {
		return fmt.Errorf("cannot write state file %s: %w", s.path, err)
	}
	s.current = document
	return nil
}
