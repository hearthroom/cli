package preview

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeShell(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html":  `<!doctype html><script defer src="./sandbox.js"></script><link rel="stylesheet" href="./sandbox.css">`,
		"sandbox.js":  "window.__shell = 1",
		"sandbox.css": "body{}",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSandboxOrigin(t *testing.T) {
	o, err := SandboxOrigin("https://sukisuki.ai", "01A0-F07D")
	if err != nil || o != "https://c01a0-f07d.sukisuki.ai" {
		t.Fatalf("%q %v", o, err)
	}
	if o, _ := SandboxOrigin("https://sukisuki.ai", ""); o != "https://cpreview.sukisuki.ai" {
		t.Fatalf("%q", o)
	}
}

func TestHandlerRoutes(t *testing.T) {
	shell := fakeShell(t)
	card := t.TempDir()
	os.WriteFile(filepath.Join(card, "rules.json"), []byte(`{"rules":[]}`), 0o644)
	os.MkdirAll(filepath.Join(card, ".hearthroom"), 0o755)
	os.WriteFile(filepath.Join(card, ".hearthroom", "state.json"), []byte(`{"secret":1}`), 0o644)
	srv := httptest.NewServer(Handler(shell, card))
	defer srv.Close()
	get := func(p string) (int, string) {
		res, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	if code, body := get("/sandbox/"); code != 200 || !strings.Contains(body, "sandbox.js") {
		t.Fatalf("shell index: %d %q", code, body)
	}
	if code, body := get("/sandbox/sandbox.js"); code != 200 || body != "window.__shell = 1" {
		t.Fatalf("shell js: %d %q", code, body)
	}
	if code, body := get("/bench/card-preview/"); code != 200 || !strings.Contains(body, `src="/sandbox/"`) {
		t.Fatalf("harness: %d", code)
	}
	if code, _ := get("/bench/card-preview/preview.js"); code != 200 {
		t.Fatalf("harness js: %d", code)
	}
	if code, body := get("/card/rules.json"); code != 200 || !strings.Contains(body, "rules") {
		t.Fatalf("card file: %d %q", code, body)
	}
	if code, _ := get("/card/.hearthroom/state.json"); code != 404 {
		t.Fatalf("state must not be served: %d", code)
	}
	if code, _ := get("/card/../rules.json"); code == 200 {
		t.Fatalf("path escape must fail")
	}
	res, _ := http.Get(srv.URL + "/sandbox/sandbox.js")
	if res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("cache header")
	}
}

func TestFetchCachesByHash(t *testing.T) {
	shell := fakeShell(t)
	origin := httptest.NewServer(http.StripPrefix("/sandbox/", http.FileServer(http.Dir(shell))))
	defer origin.Close()
	cache := t.TempDir()
	dir, fetched, err := Fetch(origin.Client(), origin.URL, cache)
	if err != nil || !fetched {
		t.Fatalf("first fetch: %v %v", err, fetched)
	}
	for _, name := range ShellFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s missing", name)
		}
	}
	if _, fetched, err := Fetch(origin.Client(), origin.URL, cache); err != nil || fetched {
		t.Fatalf("second fetch should hit the cache: %v %v", err, fetched)
	}
	// The site's SPA fallback page is not a shell.
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "<!doctype html><title>Hearthroom</title>")
	}))
	defer fallback.Close()
	if _, _, err := Fetch(fallback.Client(), fallback.URL, t.TempDir()); err == nil || !strings.Contains(err.Error(), "not the sandbox shell") {
		t.Fatalf("fallback should be rejected: %v", err)
	}
}

func TestResolveFallsBackToTheCacheOnlyWhenOffline(t *testing.T) {
	shell := fakeShell(t)
	origin := httptest.NewServer(http.StripPrefix("/sandbox/", http.FileServer(http.Dir(shell))))
	cache := t.TempDir()
	got, err := Resolve(origin.Client(), origin.URL, cache)
	if err != nil || got.Source != SourceFetched {
		t.Fatalf("online: %+v %v", got, err)
	}
	// An older complete copy and a newer incomplete one must not win.
	old := filepath.Join(cache, "0000000000000000")
	os.MkdirAll(old, 0o755)
	for _, name := range ShellFiles {
		os.WriteFile(filepath.Join(old, name), []byte("x"), 0o644)
		os.Chtimes(filepath.Join(old, name), time.Now().Add(-48*time.Hour), time.Now().Add(-48*time.Hour))
	}
	os.MkdirAll(filepath.Join(cache, "ffffffffffffffff"), 0o755)
	os.WriteFile(filepath.Join(cache, "ffffffffffffffff", "sandbox.js"), []byte("x"), 0o644)
	origin.Close()

	got2, err := Resolve(&http.Client{Timeout: time.Second}, origin.URL, cache)
	if err != nil || got2.Source != SourceOfflineCache || got2.Dir != got.Dir || got2.CachedAt.IsZero() {
		t.Fatalf("offline: %+v %v", got2, err)
	}
	if _, err := Resolve(&http.Client{Timeout: time.Second}, origin.URL, t.TempDir()); err == nil {
		t.Fatalf("offline with an empty cache must fail")
	}
	// A reachable site that answers with something else is not "offline": no fallback.
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "<!doctype html><title>Hearthroom</title>")
	}))
	defer fallback.Close()
	if _, err := Resolve(fallback.Client(), fallback.URL, cache); err == nil {
		t.Fatalf("an HTTP answer must not fall back to the cache")
	}
}
