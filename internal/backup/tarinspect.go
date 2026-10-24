package backup

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"path"
)

func inspectTar(reader io.Reader) error {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
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
		if path.IsAbs(name) || name == ".." || len(name) >= 3 && name[:3] == "../" {
			return io.EOF
		}
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeDir:
		case tar.TypeSymlink, tar.TypeLink:
			return io.EOF
		default:
			return io.EOF
		}
	}
}
