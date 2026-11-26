package sync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

// withOpeningPageMax stands in for the contract value the provider will publish (100 MB) without
// building pages that large: anything above the inline size behaves the same.
func withOpeningPageMax(t *testing.T, n int) {
	t.Helper()
	old := openingPageMax
	openingPageMax = n
	t.Cleanup(func() { openingPageMax = old })
}

type recorded struct {
	Method, Path, Query, ContentType string
	Length                           int64
	Body                             []byte
	Auth                             string
}

// assetServer is a provider with the opening-page object routes. version is the author asset's lock.
type assetServer struct {
	mu        sync.Mutex
	version   int64
	page      string
	calls     []recorded
	rawStatus int // non-zero: the raw page PUT fails with this status
}

func (s *assetServer) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		s.calls = append(s.calls, recorded{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Content-Type"), r.ContentLength, body, r.Header.Get("Authorization")})
		switch {
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/author-asset"):
			var in map[string]any
			json.Unmarshal(body, &in)
			if v, _ := in["version"].(float64); int64(v) != s.version {
				w.WriteHeader(409)
				w.Write([]byte(`{"error":"version_conflict"}`))
				return
			}
			if page, ok := in["openingPage"].(string); ok {
				s.page = page
			}
			if in["openingPageClear"] == true {
				s.page = ""
			}
			s.version++
			json.NewEncoder(w).Encode(map[string]any{"version": s.version})
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/opening-page"):
			if s.rawStatus != 0 {
				w.WriteHeader(s.rawStatus)
				w.Write([]byte(`{"error":"temporarily_unavailable"}`))
				return
			}
			if r.URL.Query().Get("version") != jsonNumber(s.version) {
				w.WriteHeader(409)
				w.Write([]byte(`{"error":"version_conflict"}`))
				return
			}
			s.page = string(body)
			s.version++
			json.NewEncoder(w).Encode(map[string]any{"version": s.version, "openingPage": "", "openingPageBytes": len(body), "openingPageUrl": "/open/v1/role/author-asset/opening?roleId=r1"})
		case r.Method == http.MethodGet && r.URL.Path == "/open/v1/role/author-asset":
			json.NewEncoder(w).Encode(map[string]any{"version": s.version, "rules": []any{}, "status": "none"})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/open/v1/trial-cards/"):
			var in map[string]any
			json.Unmarshal(body, &in)
			if aa, ok := in["authorAsset"].(map[string]any); ok {
				if page, ok := aa["openingPage"].(string); ok {
					s.page = page
				}
				if aa["openingPageClear"] == true {
					s.page = ""
				}
				s.version++
			}
			w.Write([]byte(`{"roleId":"r1","clientKey":"k","changed":["authorAsset"]}`))
		default:
			w.Write([]byte(`{}`))
		}
	})
}

func jsonNumber(n int64) string { b, _ := json.Marshal(n); return string(b) }

func (s *assetServer) writes(path string) []recorded {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []recorded
	for _, c := range s.calls {
		if c.Method != http.MethodGet && strings.HasSuffix(c.Path, path) {
			out = append(out, c)
		}
	}
	return out
}

func (s *assetServer) reset() { s.mu.Lock(); s.calls = nil; s.mu.Unlock() }

func pushClient(srv *httptest.Server) *api.Client {
	return api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
}

func pageFolder(t *testing.T, page string) *card.Folder {
	t.Helper()
	f, _ := card.Init(t.TempDir(), "Object", "en")
	f.OpeningPage = &page
	f.Rules = &card.Rules{Rules: []card.DisplayRule{}}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	// Load again: push sends what the files hold (trimmed, normalised), as the CLI does.
	f, err := card.Load(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// Above the inline size the page goes through the raw upload, after the author asset and with
// the version that save returned, so one push never conflicts with itself.
func TestOwnedLargeOpeningPageUsesRawUpload(t *testing.T) {
	withOpeningPageMax(t, 100000000) // the value the contract will publish
	s := &assetServer{}
	srv := httptest.NewServer(s.handler(t))
	defer srv.Close()
	f := pageFolder(t, strings.Repeat("界", openingPageInlineMax/3+1)) // just over the inline size
	page := *f.OpeningPage
	if _, err := Push(context.Background(), pushClient(srv), f, PushOptions{To: "owned", SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	json := s.writes("/author-asset")
	raw := s.writes("/opening-page")
	if len(json) != 1 || strings.Contains(string(json[0].Body), "openingPage\"") || len(raw) != 1 {
		t.Fatalf("author asset %d (body has page: %v), raw %d", len(json), len(json) == 1 && strings.Contains(string(json[0].Body), "openingPage\""), len(raw))
	}
	if raw[0].Query != "version=1" || raw[0].Length != int64(len(page)) || string(raw[0].Body) != page || !strings.HasPrefix(raw[0].ContentType, "text/html") || raw[0].Auth != "Bearer tok" {
		t.Fatalf("raw upload = %s len=%d type=%q auth=%q", raw[0].Query, raw[0].Length, raw[0].ContentType, raw[0].Auth)
	}
	if s.page != page || f.State.AuthorAssetVersion != 2 {
		t.Fatalf("stored page %d bytes, state version %d", len(s.page), f.State.AuthorAssetVersion)
	}
	// Nothing changed: no second upload.
	s.reset()
	f2, _ := card.Load(f.Dir)
	if _, err := Push(context.Background(), pushClient(srv), f2, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	if n := len(s.writes("/opening-page")) + len(s.writes("/author-asset")); n != 0 {
		t.Fatalf("unchanged push wrote %d times", n)
	}
	// A rule change alone re-saves the author asset but not the unchanged page.
	f2.Rules.Rules = []card.DisplayRule{{ID: "r", Find: "x", Replace: "y", Enabled: true}}
	if _, err := Push(context.Background(), pushClient(srv), f2, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	if len(s.writes("/author-asset")) != 1 || len(s.writes("/opening-page")) != 0 || s.page != page {
		t.Fatalf("rule-only push: asset %d raw %d", len(s.writes("/author-asset")), len(s.writes("/opening-page")))
	}
}

// A failed page upload leaves the author asset marked unsynced, so the next push sends both again
// with the version the first save produced.
func TestOwnedLargeOpeningPageRetryAfterFailure(t *testing.T) {
	withOpeningPageMax(t, 4<<20)
	s := &assetServer{rawStatus: 503}
	srv := httptest.NewServer(s.handler(t))
	defer srv.Close()
	f := pageFolder(t, strings.Repeat("x", openingPageInlineMax+10))
	page := *f.OpeningPage
	if _, err := Push(context.Background(), pushClient(srv), f, PushOptions{To: "owned", SkipMedia: true}); err == nil {
		t.Fatal("expected the page upload to fail")
	}
	s.rawStatus = 0
	s.reset()
	f2, _ := card.Load(f.Dir)
	if _, err := Push(context.Background(), pushClient(srv), f2, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	if len(s.writes("/author-asset")) != 1 || len(s.writes("/opening-page")) != 1 || s.page != page {
		t.Fatalf("retry: asset %d raw %d page %d", len(s.writes("/author-asset")), len(s.writes("/opening-page")), len(s.page))
	}
}

// At or below the inline size nothing changes: the page travels in the JSON save, as with the
// provider deployed today.
func TestSmallOpeningPageStaysInline(t *testing.T) {
	for _, target := range []string{"owned", ""} {
		s := &assetServer{}
		srv := httptest.NewServer(s.handler(t))
		f := pageFolder(t, strings.Repeat("x", openingPageInlineMax-1)) // the file gains a final newline
		page := *f.OpeningPage
		if len(page) != openingPageInlineMax {
			t.Fatalf("fixture is %d bytes", len(page))
		}
		if _, err := Push(context.Background(), pushClient(srv), f, PushOptions{To: target, SkipMedia: true}); err != nil {
			t.Fatal(err)
		}
		if len(s.writes("/opening-page")) != 0 || s.page != page {
			t.Fatalf("%q: raw %d, page %d", target, len(s.writes("/opening-page")), len(s.page))
		}
		for _, c := range s.calls {
			if strings.Contains(string(c.Body), "openingPageClear") {
				t.Fatalf("%q: unexpected clear in %s", target, c.Path)
			}
		}
		srv.Close()
	}
}

// A trial card takes the large page through the raw upload once the trial exists, using the
// version read back from the author asset.
func TestTrialLargeOpeningPageUsesRawUpload(t *testing.T) {
	withOpeningPageMax(t, 4<<20)
	s := &assetServer{}
	srv := httptest.NewServer(s.handler(t))
	defer srv.Close()
	f := pageFolder(t, strings.Repeat("y", openingPageInlineMax+1))
	page := *f.OpeningPage
	if _, err := Push(context.Background(), pushClient(srv), f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	trial := s.writes("")
	if len(trial) != 2 || !strings.HasPrefix(trial[0].Path, "/open/v1/trial-cards/") || strings.Contains(string(trial[0].Body), "openingPage\"") {
		t.Fatalf("writes = %v", paths(trial))
	}
	if trial[1].Path != "/open/v1/role/r1/opening-page" || trial[1].Query != "version=1" || string(trial[1].Body) != page {
		t.Fatalf("raw upload = %s?%s", trial[1].Path, trial[1].Query)
	}
	// Unchanged: the trial document is sent again (it always is), the page is not.
	s.reset()
	f2, _ := card.Load(f.Dir)
	if _, err := Push(context.Background(), pushClient(srv), f2, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	if len(s.writes("/opening-page")) != 0 || s.page != page {
		t.Fatalf("unchanged trial push uploaded the page again")
	}
}

func paths(rs []recorded) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Method+" "+r.Path)
	}
	return out
}

// Removing opening.html after it was pushed removes the stored page whatever its size; an emptied
// file does the same. A folder that never had a page sends nothing about it.
func TestOpeningPageRemovalSendsClear(t *testing.T) {
	for _, target := range []string{"owned", ""} {
		for _, how := range []string{"delete", "empty"} {
			t.Run(target+"/"+how, func(t *testing.T) {
				withOpeningPageMax(t, 4<<20)
				s := &assetServer{}
				srv := httptest.NewServer(s.handler(t))
				defer srv.Close()
				f := pageFolder(t, strings.Repeat("z", openingPageInlineMax+5))
				if _, err := Push(context.Background(), pushClient(srv), f, PushOptions{To: target, SkipMedia: true}); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(f.Dir, card.OpeningFile)
				if how == "delete" {
					os.Remove(path)
				} else {
					os.WriteFile(path, nil, 0o644)
				}
				s.reset()
				f2, _ := card.Load(f.Dir)
				if _, err := Push(context.Background(), pushClient(srv), f2, PushOptions{SkipMedia: true}); err != nil {
					t.Fatal(err)
				}
				var sent map[string]any
				w := s.writes("/author-asset")
				if target == "" {
					w = s.writes("/trial-cards/" + f2.State.TrialKey)
				}
				if len(w) != 1 {
					t.Fatalf("writes = %v", paths(s.calls))
				}
				json.Unmarshal(w[0].Body, &sent)
				if aa, ok := sent["authorAsset"].(map[string]any); ok {
					sent = aa
				}
				page, hasPage := sent["openingPage"]
				if sent["openingPageClear"] != true || (hasPage && page != "") || s.page != "" {
					t.Fatalf("sent clear=%v page=%v; stored %d bytes", sent["openingPageClear"], page, len(s.page))
				}
				// Once cleared, later pushes say nothing more about the page.
				s.reset()
				f3, _ := card.Load(f.Dir)
				f3.Rules.Rules = []card.DisplayRule{{ID: "r", Find: "x", Replace: "y", Enabled: true}}
				if _, err := Push(context.Background(), pushClient(srv), f3, PushOptions{SkipMedia: true}); err != nil {
					t.Fatal(err)
				}
				for _, c := range s.calls {
					if strings.Contains(string(c.Body), "openingPageClear") && how == "delete" {
						t.Fatalf("clear sent again: %s", c.Body)
					}
				}
			})
		}
	}
}

func pullServer(t *testing.T, asset map[string]any, page func(w http.ResponseWriter)) (*httptest.Server, *[]string) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path+"|"+r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/open/v1/role/detail":
			json.NewEncoder(w).Encode(map[string]any{"characterRoleId": "r1", "roleName": "Pulled", "language": "en", "creationMethod": "owned"})
		case "/open/v1/worldbook/bindings":
			json.NewEncoder(w).Encode(map[string]any{"bindings": []any{}})
		case "/open/v1/role/author-asset":
			json.NewEncoder(w).Encode(asset)
		case "/open/v1/role/author-asset/opening":
			if r.URL.Query().Get("roleId") != "r1" {
				http.NotFound(w, r)
				return
			}
			page(w)
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, &seen
}

func largeAsset(n int) map[string]any {
	return map[string]any{"status": "none", "version": 4, "rules": []any{map[string]any{"id": "r", "find": "x", "replace": "y", "enabled": true}},
		"pageMode": "sandbox", "openingPage": "", "openingPageBytes": n, "openingPageUrl": "/open/v1/role/author-asset/opening?roleId=r1", "openingMode": "page"}
}

// A page above the inline size arrives as a size and a link; pull reads it with the author's
// sign-in and writes it whole, so a later push does not publish an empty page.
func TestPullStreamsLargeOpeningPage(t *testing.T) {
	withOpeningPageMax(t, 4<<20)
	page := strings.Repeat("頁", openingPageInlineMax/3+7)
	srv, seen := pullServer(t, largeAsset(len(page)), func(w http.ResponseWriter) { io.WriteString(w, page) })
	defer srv.Close()
	dir := t.TempDir()
	res, err := Pull(context.Background(), pushClient(srv), "r1", dir, PullOptions{})
	if err != nil || len(res.Warnings) != 0 {
		t.Fatalf("pull = %+v, %v", res, err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, card.OpeningFile))
	if strings.TrimSpace(string(got)) != page {
		t.Fatalf("opening.html = %d bytes, want %d", len(got), len(page))
	}
	authed := false
	for _, s := range *seen {
		authed = authed || s == "/open/v1/role/author-asset/opening|Bearer tok"
	}
	if !authed {
		t.Fatalf("opening page not read with the sign-in: %v", *seen)
	}
	// Deleting it now removes the stored page.
	f, _ := card.Load(dir)
	if !f.State.OpeningPageSent {
		t.Fatal("pull did not record the page")
	}
}

// An expired or unreadable page is never replaced by an empty file: the local copy stays and
// pull says why.
func TestPullKeepsOpeningPageWhenUnreadable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		asset  func(map[string]any)
		status int
	}{
		{"expired flag", func(a map[string]any) { a["openingPageExpired"] = true }, 0},
		{"410", nil, 410},
		{"503", nil, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asset := largeAsset(600000)
			if tc.asset != nil {
				tc.asset(asset)
			}
			srv, _ := pullServer(t, asset, func(w http.ResponseWriter) {
				if tc.status == 0 {
					io.WriteString(w, "unexpected")
					return
				}
				w.WriteHeader(tc.status)
				w.Write([]byte(`{"error":"opening_page_expired"}`))
			})
			defer srv.Close()
			old := pullReplacementFixture(t)
			local := "<main>local copy</main>"
			old.OpeningPage = &local
			if err := old.Save(); err != nil {
				t.Fatal(err)
			}
			res, err := Pull(context.Background(), pushClient(srv), "r1", old.Dir, PullOptions{Force: true})
			if err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(filepath.Join(old.Dir, card.OpeningFile))
			if strings.TrimSpace(string(got)) != local {
				t.Fatalf("opening.html = %q", got)
			}
			if len(res.Warnings) == 0 || !strings.Contains(strings.Join(res.Warnings, "\n"), card.OpeningFile) {
				t.Fatalf("warnings = %v", res.Warnings)
			}
		})
	}
}

// The deployed provider sends no size or link: pull behaves as before and never asks for the page.
func TestPullInlineOpeningPageUnchanged(t *testing.T) {
	asset := map[string]any{"status": "none", "version": 4, "rules": []any{map[string]any{"id": "r", "find": "x", "replace": "y", "enabled": true}}, "openingPage": "<p>inline</p>", "openingMode": "page"}
	srv, seen := pullServer(t, asset, func(w http.ResponseWriter) { t.Error("opening page requested") })
	defer srv.Close()
	dir := t.TempDir()
	if _, err := Pull(context.Background(), pushClient(srv), "r1", dir, PullOptions{}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, card.OpeningFile))
	if strings.TrimSpace(string(got)) != "<p>inline</p>" {
		t.Fatalf("opening.html = %q (%v)", got, *seen)
	}
}

// With the contract limit raised, preflight passes a page that was over the old limit and still
// stops one byte over the new one.
func TestOpeningPagePreflightFollowsTheContractLimit(t *testing.T) {
	withOpeningPageMax(t, 2<<20)
	for _, tc := range []struct {
		n     int
		valid bool
	}{{openingPageInlineMax * 2, true}, {2 << 20, true}, {2<<20 + 1, false}} {
		f := pageFolder(t, strings.Repeat("x", tc.n-1)) // the file gains a final newline
		p, err := f.Build(nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := preflight(f, p, "en", true); (err == nil) != tc.valid {
			t.Errorf("n=%d valid=%v err=%v", tc.n, tc.valid, err)
		}
	}
}

// A pull that could not read the page into an empty folder leaves nothing to remove, so the next
// push must not clear the card's page.
func TestUnreadPageIsNotClearedByTheNextPush(t *testing.T) {
	withOpeningPageMax(t, 4<<20)
	srv, _ := pullServer(t, largeAsset(600000), func(w http.ResponseWriter) {
		w.WriteHeader(503)
		w.Write([]byte(`{"error":"temporarily_unavailable"}`))
	})
	defer srv.Close()
	dir := t.TempDir()
	res, err := Pull(context.Background(), pushClient(srv), "r1", dir, PullOptions{})
	if err != nil || len(res.Warnings) == 0 {
		t.Fatalf("pull = %+v, %v", res, err)
	}
	f, _ := card.Load(dir)
	if f.State.OpeningPageSent || f.OpeningPage != nil {
		t.Fatalf("sent=%v page=%v", f.State.OpeningPageSent, f.OpeningPage != nil)
	}
	body, _ := wireAuthorAsset(f, map[string]any{"rules": []any{}})
	if body["openingPageClear"] == true {
		t.Fatal("a push after the failed read would clear the card's page")
	}
}

// A card whose only author content is a large page is still pulled.
func TestPullPageOnlyAuthorAsset(t *testing.T) {
	withOpeningPageMax(t, 4<<20)
	page := strings.Repeat("p", openingPageInlineMax+3)
	asset := largeAsset(len(page))
	asset["rules"] = []any{}
	srv, _ := pullServer(t, asset, func(w http.ResponseWriter) { io.WriteString(w, page) })
	defer srv.Close()
	dir := t.TempDir()
	if _, err := Pull(context.Background(), pushClient(srv), "r1", dir, PullOptions{}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, card.OpeningFile))
	if strings.TrimSpace(string(got)) != page {
		t.Fatalf("opening.html = %d bytes", len(got))
	}
}

// Refusals that are not about size keep the provider's message.
func TestOpeningPageEncodingRejectionNotASizeMessage(t *testing.T) {
	raw := &api.Error{Status: 400, Code: "validate_reject", Detail: map[string]any{"reason": "openingPage", "index": -1.0, "name": "encoding", "max": 0.0, "actual": 0.0}}
	if err := openingPageRejected(raw); err != error(raw) {
		t.Fatalf("encoding refusal rewritten: %v", err)
	}
}

// A file holding only whitespace clears the page with "" beside the flag; the provider refuses
// the flag next to a non-empty page.
func TestWhitespaceOpeningPageClearsWithEmptyString(t *testing.T) {
	f, _ := card.Init(t.TempDir(), "Blank", "en")
	for _, page := range []string{"\n  \n", strings.Repeat(" ", openingPageInlineMax+1)} {
		body, large := wireAuthorAsset(f, map[string]any{"openingPage": page})
		if large != nil || body["openingPage"] != "" || body["openingPageClear"] != true {
			t.Fatalf("whitespace page of %d bytes: sends raw page = %t, body = %v", len(page), large != nil, body)
		}
	}
}
