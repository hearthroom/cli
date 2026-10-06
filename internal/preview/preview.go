// Package preview serves a card folder to the real sandbox chat shell with a fake host,
// so an author or an agent can see what the play page would draw without pushing:
// streaming, a conversation switch, both themes, five viewports, display rules on or off.
//
// The shell is the community site's own build, fetched once from the card's sandbox
// origin (`https://c<roleId>.<site host>/sandbox/`) and cached by content hash; `--shell`
// points at a local dist-sandbox instead. The harness page is embedded.
package preview

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

//go:embed assets/index.html assets/preview.js
var harness embed.FS

// ShellFiles are the three files the site's Worker serves under /sandbox/.
var ShellFiles = []string{"index.html", "sandbox.js", "sandbox.css"}

// SandboxOrigin is the shell's origin for a card on a site: c<roleId>.<site host>.
func SandboxOrigin(site, roleID string) (string, error) {
	u, err := url.Parse(site)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("site is not a URL: %q", site)
	}
	if roleID == "" {
		roleID = "preview"
	}
	return u.Scheme + "://c" + strings.ToLower(roleID) + "." + u.Host, nil
}

// Fetch downloads the shell from origin into the cache directory and returns the
// directory holding the three files. A cached copy whose sandbox.js hashes the same is
// reused; the returned bool is true when the files came from the network.
func Fetch(client *http.Client, origin, cacheRoot string) (string, bool, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	get := func(name string) ([]byte, error) {
		target := origin + "/sandbox/"
		if name != "index.html" {
			target += name
		}
		res, err := client.Get(target)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: HTTP %d", target, res.StatusCode)
		}
		b, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}
		if name == "index.html" && !strings.Contains(string(b), "sandbox.js") {
			return nil, errors.New(target + " is not the sandbox shell (the site's fallback page came back)")
		}
		return b, nil
	}
	js, err := get("sandbox.js")
	if err != nil {
		return "", false, err
	}
	sum := sha256.Sum256(js)
	dir := filepath.Join(cacheRoot, hex.EncodeToString(sum[:8]))
	if complete(dir) {
		return dir, false, nil
	}
	files := map[string][]byte{"sandbox.js": js}
	for _, name := range []string{"index.html", "sandbox.css"} {
		b, err := get(name)
		if err != nil {
			return "", false, err
		}
		files[name] = b
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return "", false, err
		}
	}
	return dir, true, nil
}

func complete(dir string) bool {
	for _, name := range ShellFiles {
		if st, err := os.Stat(filepath.Join(dir, name)); err != nil || st.Size() == 0 {
			return false
		}
	}
	return true
}

// Handler serves the shell, the harness and the card folder on one origin.
func Handler(shellDir, cardDir string) http.Handler {
	mux := http.NewServeMux()
	noStore := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			h.ServeHTTP(w, r)
		})
	}
	mux.Handle("/sandbox/", noStore(http.StripPrefix("/sandbox/", http.FileServer(http.Dir(shellDir)))))
	sub, _ := fs.Sub(harness, "assets")
	mux.Handle("/bench/card-preview/", noStore(http.StripPrefix("/bench/card-preview/", http.FileServer(http.FS(sub)))))
	mux.Handle("/card/", noStore(http.StripPrefix("/card/", cardFiles(cardDir))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/bench/card-preview/?card=/card/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})
	return mux
}

// cardFiles serves the folder read-only, never anything above it, never the sync state.
func cardFiles(dir string) http.Handler {
	root := http.Dir(dir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + r.URL.Path)
		if strings.HasPrefix(clean, "/.hearthroom") || strings.Contains(clean, "/..") {
			http.NotFound(w, r)
			return
		}
		f, err := root.Open(clean)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, st.Name(), st.ModTime(), f)
	})
}

// Serve listens on 127.0.0.1:port (0 picks a free port) and returns the URL of the
// harness page. The server runs until ctxDone is closed.
func Serve(shellDir, cardDir string, port int, ready func(url string)) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	addr := ln.Addr().(*net.TCPAddr)
	ready(fmt.Sprintf("http://127.0.0.1:%d/bench/card-preview/?card=/card/", addr.Port))
	srv := &http.Server{Handler: Handler(shellDir, cardDir), ReadHeaderTimeout: 10 * time.Second}
	return srv.Serve(ln)
}
