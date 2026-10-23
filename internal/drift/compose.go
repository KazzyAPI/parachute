package drift

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/distribution/reference"
	"go.yaml.in/yaml/v3"
)

const (
	Unknown         = "unknown"
	maxComposeBytes = 1 << 20
)

type FileReader interface {
	ReadRegularFile(path string, limit int64) ([]byte, error)
}

type composeFile struct {
	Services map[string]struct {
		Image *string `yaml:"image"`
	} `yaml:"services"`
}

type Detector struct {
	files FileReader
}

func NewDetector(files FileReader) *Detector {
	return &Detector{files: files}
}

func (d *Detector) Drift(configFiles, service, running string) *string {
	declared, ok := d.declaredImage(configFiles, service)
	if !ok {
		return nil
	}
	if strings.Contains(declared, "$") {
		return pointer(Unknown)
	}
	if canonical(declared) == canonical(running) {
		return nil
	}
	return pointer(fmt.Sprintf("compose %s, running %s", declared, running))
}

func (d *Detector) declaredImage(configFiles, service string) (string, bool) {
	var image *string
	for path := range strings.SplitSeq(configFiles, ",") {
		if path = strings.TrimSpace(path); path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			return "", false
		}
		images, err := d.images(path)
		if err != nil {
			return "", false
		}
		if declared, found := images[service]; found && declared != nil {
			image = declared
		}
	}
	if image == nil {
		return "", false
	}
	return *image, true
}

func (d *Detector) images(path string) (map[string]*string, error) {
	contents, err := d.files.ReadRegularFile(path, maxComposeBytes)
	if err != nil {
		return nil, fmt.Errorf("read compose file %s: %w", path, err)
	}
	images := map[string]*string{}
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	for {
		var document composeFile
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return images, nil
		}
		if err != nil {
			return nil, fmt.Errorf("parse compose file %s: %w", path, err)
		}
		for name, definition := range document.Services {
			if definition.Image != nil {
				images[name] = definition.Image
			}
		}
	}
}

func canonical(raw string) string {
	named, err := reference.ParseNormalizedNamed(raw)
	if err != nil {
		return raw
	}
	return reference.TagNameOnly(named).String()
}

func pointer(value string) *string {
	return &value
}
