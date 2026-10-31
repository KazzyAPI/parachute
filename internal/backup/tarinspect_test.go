package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func tarGz(members ...tar.Header) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, header := range members {
		if err := tw.WriteHeader(&header); err != nil {
			panic(err)
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestInspectTarAllowsInTargetSymlink(t *testing.T) {
	data := tarGz(
		tar.Header{Name: "v1/", Typeflag: tar.TypeDir, Mode: 0o755},
		tar.Header{Name: "current", Typeflag: tar.TypeSymlink, Linkname: "v1", Mode: 0o777},
	)
	if err := validateArchive(data); err != nil {
		t.Fatalf("validateArchive: %v", err)
	}
}

func TestInspectTarAllowsRelativeSymlinkWithinTree(t *testing.T) {
	data := tarGz(
		tar.Header{Name: "pkg/share/", Typeflag: tar.TypeDir, Mode: 0o755},
		tar.Header{Name: "pkg/lib", Typeflag: tar.TypeSymlink, Linkname: "../share", Mode: 0o777},
	)
	if err := validateArchive(data); err != nil {
		t.Fatalf("validateArchive: %v", err)
	}
}

func TestInspectTarRejectsEscapeSymlink(t *testing.T) {
	data := tarGz(
		tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../escape", Mode: 0o777},
	)
	if err := validateArchive(data); err == nil {
		t.Fatal("expected invalid tar member")
	}
}
