package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hearthroom/cli/internal/api"
)

// uploadServer records the Content-Type of the multipart file part.
func uploadServer(t *testing.T, status int, body string) (*api.Client, *atomic.Int32, *atomic.Value) {
	t.Helper()
	var calls atomic.Int32
	var partType atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return
		}
		fhs := r.MultipartForm.File["file"]
		if len(fhs) != 1 {
			t.Errorf("file parts: %d", len(fhs))
			return
		}
		partType.Store(fhs[0].Header.Get("Content-Type") + "|" + fhs[0].Filename)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	return c, &calls, &partType
}

func writeFile(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUploadDeclaresTypeFromExtension(t *testing.T) {
	ok := `{"code":0,"data":{"imageId":1,"imageUrl":"https://cdn.test/x"}}`
	for name, want := range map[string]string{
		"app.js":       "text/javascript",
		"mod.mjs":      "text/javascript",
		"engine.wasm":  "application/wasm",
		"state.json":   "application/json",
		"icon.svg":     "image/svg+xml",
		"clip.mp4":     "video/mp4",
		"Font.WOFF2":   "font/woff2",
		"photo.jpg":    "image/jpeg",
		"theme.ogg":    "audio/ogg",
		"notes.txt":    "application/octet-stream",
		"no-extension": "application/octet-stream",
	} {
		t.Run(name, func(t *testing.T) {
			c, _, part := uploadServer(t, 200, ok)
			if _, err := Upload(context.Background(), c, writeFile(t, name), "", ""); err != nil {
				t.Fatal(err)
			}
			if got := part.Load(); got != want+"|"+name {
				t.Fatalf("part: %v, want %s|%s", got, want, name)
			}
		})
	}
}

func TestUploadRefusesQuickTimeBeforeSending(t *testing.T) {
	c, calls, _ := uploadServer(t, 200, `{}`)
	_, err := Upload(context.Background(), c, writeFile(t, "clip.MOV"), "", "")
	if err == nil || !strings.Contains(err.Error(), "export it as MP4") {
		t.Fatalf("want export-as-MP4 error, got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("sent %d requests", calls.Load())
	}
}

// A QuickTime file renamed to .mp4 is still refused by the server; the
// author gets the same advice.
func TestUploadExplainsServerQuickTimeRefusal(t *testing.T) {
	c, _, _ := uploadServer(t, 400, `{"error":"invalid_param","detail":{"reason":"quicktime","use":"video/mp4"}}`)
	_, err := Upload(context.Background(), c, writeFile(t, "clip.mp4"), "", "")
	if err == nil || err.Error() != quickTimeMessage("clip.mp4") {
		t.Fatalf("got %v", err)
	}
}
