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

func TestInspectTarRejectsChainedSymlinkEscape(t *testing.T) {
	data := tarGz(
		tar.Header{Name: "d1/d2/s", Typeflag: tar.TypeSymlink, Linkname: "../..", Mode: 0o777},
		tar.Header{Name: "t", Typeflag: tar.TypeSymlink, Linkname: "d1/d2/s/../x", Mode: 0o777},
	)
	if err := validateArchive(data); err == nil {
		t.Fatal("expected invalid tar member")
	}
}

func TestInspectTarRejectsMemberThroughSymlink(t *testing.T) {
	data := tarGz(
		tar.Header{Name: "v1/", Typeflag: tar.TypeDir, Mode: 0o755},
		tar.Header{Name: "current", Typeflag: tar.TypeSymlink, Linkname: "v1", Mode: 0o777},
		tar.Header{Name: "current/file", Typeflag: tar.TypeReg, Mode: 0o644},
	)
	if err := validateArchive(data); err == nil {
		t.Fatal("expected invalid tar member")
	}
}

func TestInspectTarRejectsAbsoluteSymlinkAndEscapingHardLink(t *testing.T) {
	for _, header := range []tar.Header{
		{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc", Mode: 0o777},
		{Name: "hard", Typeflag: tar.TypeLink, Linkname: "../x", Mode: 0o644},
	} {
		if err := validateArchive(tarGz(header)); err == nil {
			t.Fatalf("expected invalid tar member for %s", header.Name)
		}
	}
}
