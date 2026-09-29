package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyChecksum(t *testing.T) {
	data := []byte("binary")
	sum := sha256.Sum256(data)
	sums := "deadbeef  other.tar.gz\n" + hex.EncodeToString(sum[:]) + "  hearthroom_1.0.0_linux_amd64.tar.gz\n"
	if err := verifyChecksum(data, "hearthroom_1.0.0_linux_amd64.tar.gz", sums); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksum([]byte("tampered"), "hearthroom_1.0.0_linux_amd64.tar.gz", sums); err == nil {
		t.Fatal("tampered archive accepted")
	}
	if err := verifyChecksum(data, "missing.tar.gz", sums); err == nil {
		t.Fatal("missing checksum accepted")
	}
}

func TestExtractBinary(t *testing.T) {
	var tgz bytes.Buffer
	gz := gzip.NewWriter(&tgz)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{"LICENSE": "l", "hearthroom": "BIN"} {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	bin, err := extractBinary(tgz.Bytes(), "tar.gz")
	if err != nil || string(bin) != "BIN" {
		t.Fatalf("tar: %q %v", bin, err)
	}
	var z bytes.Buffer
	zw := zip.NewWriter(&z)
	w, _ := zw.Create("hearthroom.exe")
	_, _ = w.Write([]byte("EXE"))
	zw.Close()
	bin, err = extractBinary(z.Bytes(), "zip")
	if err != nil || string(bin) != "EXE" {
		t.Fatalf("zip: %q %v", bin, err)
	}
	if _, err := extractBinary([]byte("junk"), "tar.gz"); err == nil {
		t.Fatal("junk accepted")
	}
}
