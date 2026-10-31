package restoreverify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/KazzyAPI/parachute/internal/txn"
)

const Format = "parachute.restore-verify/v1"

const fileName = "restore-verify.json"

type Document struct {
	Format  string        `json:"format"`
	ID      string        `json:"id"`
	Open    bool          `json:"open"`
	Started time.Time     `json:"started"`
	Ended   time.Time     `json:"ended,omitempty"`
	Stacks  []StackRecord `json:"stacks"`
}

type StackRecord struct {
	Host     string `json:"host"`
	Stack    string `json:"stack"`
	Snapshot string `json:"snapshot"`
	Result   string `json:"result,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

func ReadRecord(files FileSystem, dir string) (Document, bool, error) {
	path := filepath.Join(dir, fileName)
	contents, err := files.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Document{}, false, nil
	}
	if err != nil {
		return Document{}, false, fmt.Errorf("cannot read %s: %w", path, err)
	}
	document, err := decodeRecord(contents)
	if err != nil {
		return Document{}, true, fmt.Errorf("%s is not valid %s JSON: %w", path, Format, err)
	}
	return document, true, nil
}

func WriteRecord(files FileSystem, dir string, document Document) error {
	if document.Format == "" {
		document.Format = Format
	}
	if document.Format != Format {
		return fmt.Errorf("restore-verify record format %q, expected %q", document.Format, Format)
	}
	if !txn.ValidRestoreVerifyID(document.ID) {
		return fmt.Errorf("restore-verify run id %q is invalid", document.ID)
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return files.WriteAtomic(filepath.Join(dir, fileName), data)
}

func decodeRecord(contents []byte) (Document, error) {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Document{}, errors.New("trailing data")
	}
	if document.Format != Format {
		return Document{}, fmt.Errorf("format %q, expected %q", document.Format, Format)
	}
	if !txn.ValidRestoreVerifyID(document.ID) {
		return Document{}, fmt.Errorf("run id %q is invalid", document.ID)
	}
	return document, nil
}

func OpenRunID(document Document) string {
	if document.Open && txn.ValidRestoreVerifyID(document.ID) {
		return document.ID
	}
	return ""
}
