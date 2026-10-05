package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

const prefix = "https://cdn.test/u/abc"

// fakeLibrary serves /image/list and /image/upload with relative paths.
type fakeLibrary struct {
	mu    sync.Mutex
	files map[string]bool // relative paths already in the library
	puts  []string
}

func (l *fakeLibrary) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		defer l.mu.Unlock()
		switch r.URL.Path {
		case "/open/v1/image/list":
			q := r.URL.Query().Get("q")
			var items []map[string]any
			for name := range l.files {
				if strings.Contains(name, q) {
					items = append(items, map[string]any{"fileName": name, "imageUrl": prefix + "/" + name})
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
				"total": len(items), "imageList": items, "libraryPrefix": prefix,
				"capabilities": map[string]any{"relativePaths": true, "overwrite": true},
			}})
		case "/open/v1/image/upload":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			rp := r.FormValue("relativePath")
			replaced := l.files[rp]
			l.files[rp] = true
			l.puts = append(l.puts, rp)
			esc := strings.Split(rp, "/")
			for i := range esc {
				esc[i] = url.PathEscape(esc[i])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
				"imageUrl": prefix + "/" + strings.Join(esc, "/"), "fileName": rp, "replaced": replaced,
			}})
		default:
			http.NotFound(w, r)
		}
	})
}

func setup(t *testing.T, existing ...string) (*fakeLibrary, *api.Client, *card.Folder) {
	t.Helper()
	lib := &fakeLibrary{files: map[string]bool{}}
	for _, e := range existing {
		lib.files[e] = true
	}
	srv := httptest.NewServer(lib.handler(t))
	t.Cleanup(srv.Close)
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	dir := t.TempDir()
	for _, p := range []string{"portrait.webp", "expr/happy.webp", "expr/sad.webp"} {
		full := filepath.Join(dir, "assets", filepath.FromSlash(p))
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(p), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f := &card.Folder{Dir: dir}
	f.Manifest.Name = "天道非要我成仙"
	f.Manifest.Media.Portrait = "assets/portrait.webp"
	f.Rules = &card.Rules{Rules: []card.DisplayRule{{Find: "/<face>(\\w+)<\\/face>/", Replace: `<img src="assets/expr/$1.webp">`, Enabled: true}}}
	return lib, c, f
}

func TestSyncUsesCardFolderAndServesDirectories(t *testing.T) {
	lib, c, f := setup(t)
	caps, _ := Probe(context.Background(), c)
	urls, outcomes, err := Sync(context.Background(), c, f, caps, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range outcomes {
		if o.Err != "" {
			t.Fatalf("outcome: %+v", o)
		}
	}
	if strings.Join(lib.puts, ",") != "天道非要我成仙/expr/happy.webp,天道非要我成仙/expr/sad.webp,天道非要我成仙/portrait.webp" {
		t.Fatalf("puts: %v", lib.puts)
	}
	esc := url.PathEscape("天道非要我成仙")
	if got := urls["assets/expr/"]; got != prefix+"/"+esc+"/expr/" {
		t.Fatalf("directory url: %q", got)
	}
	if f.State.AssetFolder != "天道非要我成仙" {
		t.Fatalf("state folder: %q", f.State.AssetFolder)
	}
}

func TestSyncMovesFilesWhenFolderChanges(t *testing.T) {
	lib, c, f := setup(t)
	caps, _ := Probe(context.Background(), c)
	if _, _, err := Sync(context.Background(), c, f, caps, false); err != nil {
		t.Fatal(err)
	}
	lib.puts = nil
	f.Manifest.Media.Folder = "tiandao"
	_, outcomes, err := Sync(context.Background(), c, f, caps, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.puts) != 3 || !strings.HasPrefix(lib.puts[0], "tiandao/") {
		t.Fatalf("puts after rename: %v", lib.puts)
	}
	if outcomes[0].Previous != "天道非要我成仙/expr/happy.webp" {
		t.Fatalf("previous: %+v", outcomes[0])
	}
	// A pushed-again folder with unchanged files uploads nothing.
	lib.puts = nil
	if _, _, err := Sync(context.Background(), c, f, caps, false); err != nil || len(lib.puts) != 0 {
		t.Fatalf("second push: %v %v", lib.puts, err)
	}
}

func TestSyncRefusesAnotherCardsFolderUnlessExplicit(t *testing.T) {
	lib, c, f := setup(t, "天道非要我成仙/bg-day.webp")
	caps, _ := Probe(context.Background(), c)
	_, _, err := Sync(context.Background(), c, f, caps, false)
	if err == nil || !strings.Contains(err.Error(), "media.folder") || !strings.Contains(err.Error(), "天道非要我成仙/bg-day.webp") {
		t.Fatalf("want collision error, got %v", err)
	}
	if len(lib.puts) != 0 {
		t.Fatalf("uploaded despite collision: %v", lib.puts)
	}
	f.Manifest.Media.Folder = "天道非要我成仙"
	if _, _, err := Sync(context.Background(), c, f, caps, false); err != nil {
		t.Fatalf("explicit share: %v", err)
	}
}

func TestSyncKeepsFolderOfEarlierUploads(t *testing.T) {
	lib, c, f := setup(t)
	f.State.Assets = map[string]card.Asset{"assets/portrait.webp": {SHA256: "old", URL: prefix + "/01a0f07d/portrait.webp", FileName: "01a0f07d/portrait.webp"}}
	caps, _ := Probe(context.Background(), c)
	if _, _, err := Sync(context.Background(), c, f, caps, false); err != nil {
		t.Fatal(err)
	}
	if f.State.AssetFolder != "01a0f07d" || !strings.HasPrefix(lib.puts[0], "01a0f07d/") {
		t.Fatalf("legacy folder not kept: %q %v", f.State.AssetFolder, lib.puts)
	}
}
