package handover

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"

	"github.com/KazzyAPI/parachute/internal/txn"
)

const Format = "parachute.handover/v1"

const fileName = "handover.json"

var containerIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteAtomic(path string, data []byte) error
	Remove(path string) error
}

type Marker struct {
	Format      string `json:"format"`
	Transaction string `json:"transaction"`
	Container   string `json:"container"`
	Digest      string `json:"digest"`
}

func Write(files FileSystem, dir string, marker Marker) error {
	if marker.Format == "" {
		marker.Format = Format
	}
	if marker.Format != Format {
		return fmt.Errorf("handover marker format %q, expected %q", marker.Format, Format)
	}
	if !txn.ValidID(marker.Transaction) {
		return fmt.Errorf("handover marker transaction %q is invalid", marker.Transaction)
	}
	if !containerIDPattern.MatchString(marker.Container) {
		return fmt.Errorf("handover marker container %q is invalid", marker.Container)
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	return files.WriteAtomic(filepath.Join(dir, fileName), data)
}

func Read(files FileSystem, dir string) (Marker, bool, error) {
	path := filepath.Join(dir, fileName)
	contents, err := files.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Marker{}, false, nil
	}
	if err != nil {
		return Marker{}, false, fmt.Errorf("cannot read handover marker %s: %w", path, err)
	}
	marker, err := decode(contents)
	if err != nil {
		return Marker{}, true, fmt.Errorf("handover marker %s is not valid %s JSON: %w", path, Format, err)
	}
	return marker, true, nil
}

func Remove(files FileSystem, dir string) error {
	if err := files.Remove(filepath.Join(dir, fileName)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove handover marker: %w", err)
	}
	return nil
}

func decode(contents []byte) (Marker, error) {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var marker Marker
	if err := decoder.Decode(&marker); err != nil {
		return Marker{}, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Marker{}, errors.New("trailing data")
	}
	if marker.Format != Format {
		return Marker{}, fmt.Errorf("format %q, expected %q", marker.Format, Format)
	}
	if !txn.ValidID(marker.Transaction) {
		return Marker{}, fmt.Errorf("transaction %q is invalid", marker.Transaction)
	}
	if !containerIDPattern.MatchString(marker.Container) {
		return Marker{}, fmt.Errorf("container %q is invalid", marker.Container)
	}
	return marker, nil
}
