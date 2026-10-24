package engine

import (
	"archive/tar"
	"bytes"
	"io"
	"path"
	"strings"
)

func tarBytes(data []byte, name string) (io.Reader, error) {
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	header := &tar.Header{Name: path.Clean(strings.TrimPrefix(name, "/")), Mode: 0o644, Size: int64(len(data))}
	if err := writer.WriteHeader(header); err != nil {
		return nil, err
	}
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return &buffer, nil
}

func readTarFile(reader io.Reader, name string) ([]byte, error) {
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil, io.EOF
		}
		if err != nil {
			return nil, err
		}
		if header.Name == name || path.Base(header.Name) == name {
			return io.ReadAll(tarReader)
		}
	}
}
