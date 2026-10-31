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
	symlinks := map[string]bool{}
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
		if throughSymlink(symlinks, name) {
			return errInvalidTarMember
		}
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeDir:
		case tar.TypeSymlink:
			if !linkTargetInside(name, header.Linkname) || throughSymlink(symlinks, path.Dir(name)+"/"+header.Linkname) {
				return errInvalidTarMember
			}
			symlinks[strings.TrimPrefix(name, "./")] = true
		case tar.TypeLink:
			if !hardLinkTargetInside(header.Linkname) || throughSymlink(symlinks, header.Linkname) {
				return errInvalidTarMember
			}
		default:
			return errInvalidTarMember
		}
	}
}

func throughSymlink(symlinks map[string]bool, name string) bool {
	parts := strings.Split(strings.TrimPrefix(name, "./"), "/")
	for end := range parts {
		if symlinks[strings.Join(parts[:end+1], "/")] {
			return true
		}
	}
	return false
}

func hardLinkTargetInside(target string) bool {
	if path.IsAbs(target) {
		return false
	}
	return !stringsHasDotDot(path.Clean(target))
}

func linkTargetInside(member, target string) bool {
	if path.IsAbs(target) {
		return false
	}
	base := path.Clean(path.Dir(member))
	if base == "." {
		base = ""
	}
	return !stringsHasDotDot(path.Clean(path.Join(base, target)))
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
