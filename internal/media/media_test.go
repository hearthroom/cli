package media

import (
	"context"
	"encoding/json"
	"fmt"
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
	files map[string]bool   // relative paths already in the library
	ids   map[string]string // relative path -> item id
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
					items = append(items, map[string]any{"id": l.ids[name], "fileName": name, "imageUrl": prefix + "/" + escapePath(name)})
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
			if l.ids[rp] == "" {
				l.ids[rp] = fmt.Sprint(len(l.ids) + 1)
			}
			l.puts = append(l.puts, rp)
			esc := strings.Split(rp, "/")
			for i := range esc {
				esc[i] = url.PathEscape(esc[i])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
				"imageId": l.ids[rp], "imageUrl": prefix + "/" + strings.Join(esc, "/"), "fileName": rp, "replaced": replaced,
			}})
		default:
			http.NotFound(w, r)
		}
	})
}

func setup(t *testing.T, existing ...string) (*fakeLibrary, *api.Client, *card.Folder) {
	t.Helper()
	lib := &fakeLibrary{files: map[string]bool{}, ids: map[string]string{}}
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
	if outcomes[0].Previous != prefix+"/"+url.PathEscape("天道非要我成仙")+"/expr/happy.webp" {
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

// Renaming a file in the library changes the name it is listed under, not
// the URL it is served from, so it must not trigger a re-upload.
func TestSyncIgnoresLibraryRenames(t *testing.T) {
	lib, c, f := setup(t)
	caps, _ := Probe(context.Background(), c)
	if _, _, err := Sync(context.Background(), c, f, caps, false); err != nil {
		t.Fatal(err)
	}
	a := f.State.Assets["assets/portrait.webp"]
	a.FileName = "renamed in the library.webp"
	f.State.Assets["assets/portrait.webp"] = a
	lib.puts = nil
	if _, _, err := Sync(context.Background(), c, f, caps, false); err != nil || len(lib.puts) != 0 {
		t.Fatalf("re-uploaded after a rename: %v %v", lib.puts, err)
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

// rename moves every library path under from/ to to/, keeping item ids, the way
// a rename on the resource page does.
func (l *fakeLibrary) rename(from, to string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for name := range l.files {
		if strings.HasPrefix(name, from+"/") {
			moved := to + strings.TrimPrefix(name, from)
			l.files[moved], l.ids[moved] = true, l.ids[name]
			delete(l.files, name)
			delete(l.ids, name)
		}
	}
}

// After the author renames the card's folder in the library, the next push
// follows the files to their new URLs instead of uploading them to the old name.
func TestSyncFollowsALibraryRename(t *testing.T) {
	lib, c, f := setup(t)
	caps, _ := Probe(context.Background(), c)
	if _, _, err := Sync(context.Background(), c, f, caps, false); err != nil {
		t.Fatal(err)
	}
	lib.rename("天道非要我成仙", "tiandao")
	lib.puts = nil
	urls, _, err := Sync(context.Background(), c, f, caps, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.puts) != 0 || f.State.AssetFolder != "tiandao" || urls["assets/portrait.webp"] != prefix+"/tiandao/portrait.webp" || urls["assets/expr/"] != prefix+"/tiandao/expr/" {
		t.Fatalf("after rename: puts %v folder %q urls %v", lib.puts, f.State.AssetFolder, urls)
	}
	// A media.folder that still names the old folder is a conflict to resolve, not a re-upload.
	lib.rename("tiandao", "renamed-again")
	f.Manifest.Media.Folder = "tiandao"
	if _, _, err := Sync(context.Background(), c, f, caps, false); err == nil || !strings.Contains(err.Error(), "renamed-again") {
		t.Fatalf("explicit folder conflict: %v", err)
	}
}

func TestSyncUploadsAgainWhatTheLibraryDeleted(t *testing.T) {
	lib, c, f := setup(t)
	caps, _ := Probe(context.Background(), c)
	if _, _, err := Sync(context.Background(), c, f, caps, false); err != nil {
		t.Fatal(err)
	}
	lib.mu.Lock()
	delete(lib.files, "天道非要我成仙/portrait.webp")
	lib.mu.Unlock()
	lib.puts = nil
	if _, _, err := Sync(context.Background(), c, f, caps, false); err != nil || strings.Join(lib.puts, ",") != "天道非要我成仙/portrait.webp" {
		t.Fatalf("re-upload: %v %v", lib.puts, err)
	}
}

// A dry run must report a .mov as refused, not as "would upload".
func TestSyncDryRunRefusesQuickTime(t *testing.T) {
	_, c, f := setup(t)
	full := filepath.Join(f.Dir, "assets", "clip.mov")
	if err := os.WriteFile(full, []byte("mov"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.Manifest.Media.Background = "assets/clip.mov"
	caps, _ := Probe(context.Background(), c)
	_, outcomes, err := Sync(context.Background(), c, f, caps, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range outcomes {
		if o.Path == "assets/clip.mov" {
			if o.Uploaded || !strings.Contains(o.Err, "MP4") {
				t.Fatalf("outcome: %+v", o)
			}
			return
		}
	}
	t.Fatalf("clip.mov missing from %+v", outcomes)
}
