package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

func TestOpeningPagePreflightBoundsAfterRewriting(t *testing.T) {
	const assetURL = "https://cdn.example/library/long-folder/background.png"
	max := openingPageMax
	for _, tc := range []struct {
		page  string
		valid bool
	}{
		{strings.Repeat("x", max), true},
		{strings.Repeat("x", max+1), false},
		{strings.Repeat("界", max/3+1), false},
		{strings.Repeat("x", max-len(`<img src="assets/bg.png">`)) + `<img src="assets/bg.png">`, false},
	} {
		f, err := card.Init(t.TempDir(), "Synthetic", "en")
		if err != nil {
			t.Fatal(err)
		}
		f.OpeningPage = &tc.page
		f.Rules = &card.Rules{Rules: []card.DisplayRule{}}
		p, err := f.Build(map[string]string{"assets/bg.png": assetURL})
		if err != nil {
			t.Fatal(err)
		}
		_, err = preflight(f, p, "en", true)
		if (err == nil) != tc.valid {
			t.Errorf("len=%d valid=%v err=%v", len(tc.page), tc.valid, err)
		}
		if err != nil && !strings.Contains(err.Error(), "hearthroom media upload") {
			t.Errorf("message lacks the fix: %v", err)
		}
	}
}

func TestOpeningPagePushBoundary(t *testing.T) {
	for _, n := range []int{openingPageMax, openingPageMax + 1} {
		for _, target := range []string{"owned", "trial"} {
			t.Run(target+"/"+map[bool]string{true: "at", false: "over"}[n == openingPageMax], func(t *testing.T) {
				var writes []string
				sent := -1
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != "GET" {
						writes = append(writes, r.URL.Path)
					}
					if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/opening-page") {
						n, err := io.Copy(io.Discard, r.Body)
						if err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
						sent = int(n)
						_, _ = w.Write([]byte(`{"version":1}`))
						return
					}
					if r.Method == http.MethodPut {
						var in map[string]any
						json.NewDecoder(r.Body).Decode(&in)
						if aa, ok := in["authorAsset"].(map[string]any); ok {
							in = aa
						}
						if page, ok := in["openingPage"].(string); ok {
							sent = len(page)
						}
						w.Write([]byte(`{"version":1,"roleId":"r1","clientKey":"k"}`))
						return
					}
					w.Write([]byte(`{}`))
				}))
				defer srv.Close()
				f, _ := card.Init(t.TempDir(), "Boundary", "en")
				page := strings.Repeat("x", n)
				f.OpeningPage = &page
				f.Rules = &card.Rules{Rules: []card.DisplayRule{}}
				c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
				_, err := Push(context.Background(), c, f, PushOptions{To: map[string]string{"owned": "owned"}[target], SkipMedia: true})
				if n == openingPageMax {
					if err != nil {
						t.Fatal(err)
					}
					if sent != n {
						t.Fatalf("sent %d bytes, want %d (writes %v)", sent, n, writes)
					}
					return
				}
				if err == nil || len(writes) != 0 {
					t.Fatalf("err=%v writes=%v", err, writes)
				}
				for _, want := range []string{fmt.Sprint(openingPageMax + 1), fmt.Sprint(openingPageMax), card.HumanBytes(openingPageMax), "hearthroom media upload"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("message lacks %q: %v", want, err)
					}
				}
			})
		}
	}
}

// The provider may still refuse the page (another limit, an older CLI estimate); the author
// sees the same explanation, and the raw response stays reachable for --json.
func TestOpeningPageServerRejectionExplained(t *testing.T) {
	for _, tc := range []struct {
		target, body string
		status       int
	}{
		{"owned", `{"error":"validate_reject","message":"author asset rejected: openingPage","detail":{"reason":"openingPage","index":-1,"name":"","max":%d,"actual":%d,"unit":"bytes"}}`, 400},
		{"trial", `{"error":"trial_payload_too_large","message":"openingPage","detail":{"reason":"openingPage","section":"authorAsset","max":%d,"actual":%d,"unit":"bytes"}}`, 413},
	} {
		t.Run(tc.target, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					w.WriteHeader(tc.status)
					fmt.Fprintf(w, tc.body, openingPageMax, openingPageMax+1000)
					return
				}
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			f, _ := card.Init(t.TempDir(), "Rejected", "en")
			page := "<div>small</div>"
			f.OpeningPage = &page
			f.Rules = &card.Rules{Rules: []card.DisplayRule{}}
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
			_, err := Push(context.Background(), c, f, PushOptions{To: map[string]string{"owned": "owned"}[tc.target], SkipMedia: true})
			if err == nil {
				t.Fatal("expected rejection")
			}
			for _, want := range []string{fmt.Sprint(openingPageMax + 1000), fmt.Sprint(openingPageMax), card.HumanBytes(openingPageMax), "hearthroom media upload"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("message lacks %q: %v", want, err)
				}
			}
			var ae *api.Error
			if !errors.As(err, &ae) || ae.Status != tc.status {
				t.Fatalf("raw provider error not reachable: %v", err)
			}
			detail, _ := ae.Detail.(map[string]any)
			if detail["reason"] != "openingPage" {
				t.Fatalf("detail lost: %v", ae.Detail)
			}
		})
	}
}

// Other author-asset rejections keep their own text.
func TestOtherAuthorAssetRejectionUnchanged(t *testing.T) {
	err := openingPageRejected(&api.Error{Status: 400, Code: "validate_reject", Detail: map[string]any{"reason": "cardCss", "max": float64(openingPageMax), "actual": float64(openingPageMax + 1)}})
	if strings.Contains(err.Error(), "opening") {
		t.Fatalf("cardCss rejection mapped to the opening page: %v", err)
	}
}

// The real push path (media not skipped): an opening page over the limit stops before any
// write, including media uploads and card-image creation.
func TestOpeningPageStopsBeforeMediaWrites(t *testing.T) {
	icon := `<img src="assets/icon.svg">`
	served := `<img src="https://cdn.example/library/safe/icon.svg">`
	for _, tc := range []struct {
		name, page string
		portrait   bool
	}{
		// Under the limit as written, over it once the reference becomes the planned library URL.
		{"over after rewrite", strings.Repeat("x", openingPageMax-len(served)+1) + icon, false},
		// No assets/ reference: the size is exact before card images are created.
		{"inline with card image", strings.Repeat("x", openingPageMax+1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var writes []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes = append(writes, r.Method+" "+r.URL.Path)
				}
				w.Write([]byte(`{"accountNumId":7,"data":{"capabilities":{"relativePaths":true},"libraryPrefix":"https://cdn.example/library"}}`))
			}))
			defer srv.Close()
			f, _ := card.Init(t.TempDir(), "Safe", "en")
			f.Manifest.Media.Folder = "safe"
			os.MkdirAll(filepath.Join(f.Dir, "assets"), 0755)
			os.WriteFile(filepath.Join(f.Dir, "assets/icon.svg"), []byte("svg"), 0644)
			if tc.portrait {
				os.WriteFile(filepath.Join(f.Dir, "assets/portrait.png"), []byte("png"), 0644)
				f.Manifest.Media.Portrait = "assets/portrait.png"
			}
			f.OpeningPage = &tc.page
			f.Rules = &card.Rules{Rules: []card.DisplayRule{}}
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
			_, err := Push(context.Background(), c, f, PushOptions{})
			if err == nil || !strings.Contains(err.Error(), "hearthroom media upload") || len(writes) != 0 {
				t.Fatalf("err=%v writes=%v", err, writes)
			}
		})
	}
}
