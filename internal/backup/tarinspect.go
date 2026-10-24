package backup

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"path"
	"strings"
)

var errInvalidTarMember = errors.New("invalid tar member")

func inspectTar(reader io.Reader) error {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return err
	}
	defer func() { _ = gzipReader.Close() }()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(header.Name)
		if path.IsAbs(name) || name == ".." || stringsHasDotDot(name) {
			return errInvalidTarMember
		}
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeDir:
		case tar.TypeSymlink, tar.TypeLink:
			return errInvalidTarMember
		default:
			return errInvalidTarMember
		}
	}
}

func stringsHasDotDot(name string) bool {
	clean := path.Clean(name)
	for _, part := range strings.Split(clean, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
