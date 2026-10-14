package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadFileStreamsWithAKnownLength(t *testing.T) {
	type got struct {
		length   int64
		chunked  bool
		path     string
		fileName string
		fileType string
		body     string
	}
	seen := make(chan got, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g := got{length: r.ContentLength, chunked: len(r.TransferEncoding) > 0}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		g.path = r.FormValue("relativePath")
		f, h, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
		} else {
			b, _ := io.ReadAll(f)
			g.body, g.fileName, g.fileType = string(b), h.Filename, h.Header.Get("Content-Type")
		}
		seen <- g
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()
	c := New(srv.URL, srv.URL, "test", nil)

	content := strings.Repeat("layer bytes ", 5000)
	file := filepath.Join(t.TempDir(), "a.webp")
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	fh, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	fields := map[string][]string{"relativePath": {"art/a.webp"}}
	for _, r := range []io.Reader{fh, strings.NewReader(content)} {
		if err := c.UploadFile(context.Background(), "/image/upload", fields, "file", "a.webp", "image/webp", r, nil); err != nil {
			t.Fatal(err)
		}
		g := <-seen
		if g.chunked || g.length <= int64(len(content)) {
			t.Fatalf("%T: content length %d, chunked %v", r, g.length, g.chunked)
		}
		if g.path != "art/a.webp" || g.fileName != "a.webp" || g.fileType != "image/webp" || g.body != content {
			t.Fatalf("%T: received %q %q %q, %d bytes", r, g.path, g.fileName, g.fileType, len(g.body))
		}
	}
}
