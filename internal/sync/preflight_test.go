package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

func TestPushPreflightRejectsBeforeAnyWrite(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*card.Folder)
		want string
	}{
		{"replace", func(f *card.Folder) { f.Rules.Rules[0].Replace = strings.Repeat("界", 43691) }, "131073"},
		{"empty find", func(f *card.Folder) { f.Rules.Rules[0].Find = " " }, "find"},
		{"shared state", func(f *card.Folder) {
			f.Rules.SharedState = json.RawMessage(`{"counters":{"x":{"maxStep":1001,"perMember":1}}}`)
		}, "sharedState"},
		{"card format", func(f *card.Folder) { f.Rules.CardFormat = "bad" }, "cardFormat"},
		{"definition", func(f *card.Folder) { f.Definition = strings.Repeat("a", 50001) }, "roleDetailDesc"},
		{"welcome", func(f *card.Folder) { f.Welcome = strings.Repeat("a", 10001) }, "roleWelcome"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
				}
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			f, _ := card.Init(t.TempDir(), "Safe", "en")
			f.Rules = &card.Rules{Rules: []card.DisplayRule{{ID: "r", Name: "Rule", Find: "x", Replace: "ok", Enabled: true}}}
			tc.edit(f)
			before, _ := os.ReadFile(filepath.Join(f.Dir, ".hearthroom/state.json"))
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
			_, err := Push(context.Background(), c, f, PushOptions{To: "owned", SkipMedia: true, Force: true})
			after, _ := os.ReadFile(filepath.Join(f.Dir, ".hearthroom/state.json"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err=%v, want %s", err, tc.want)
			}
			if writes != 0 || string(before) != string(after) {
				t.Errorf("writes=%d; state changed=%v", writes, string(before) != string(after))
			}
		})
	}
}

func TestAuthorAssetFailureDoesNotWriteContent(t *testing.T) {
	var writes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			writes = append(writes, r.URL.Path)
		}
		if strings.HasSuffix(r.URL.Path, "/author-asset") {
			w.WriteHeader(413)
			w.Write([]byte(`{"error":"validate_reject"}`))
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	f, _ := card.Init(t.TempDir(), "Safe", "en")
	f.Welcome = "new opening"
	f.Rules = &card.Rules{Rules: []card.DisplayRule{{Find: "x", Replace: "ok"}}}
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	_, err := Push(context.Background(), c, f, PushOptions{To: "owned", SkipMedia: true})
	if err == nil {
		t.Fatal("expected rejection")
	}
	if !reflect.DeepEqual(writes, []string{"/open/v1/role/owned/author-asset"}) {
		t.Fatalf("wrote %v", writes)
	}
}

func TestDryRunReportsExactRewrittenRuleBytesWithoutState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("dry-run wrote %s", r.URL.Path)
		}
		w.Write([]byte(`{"data":{"capabilities":{"relativePaths":true},"libraryPrefix":"https://cdn.example/library"}}`))
	}))
	defer srv.Close()
	f, _ := card.Init(t.TempDir(), "Safe", "en")
	f.Manifest.Media.Folder = "safe"
	os.MkdirAll(filepath.Join(f.Dir, "assets"), 0755)
	os.WriteFile(filepath.Join(f.Dir, "assets/icon.svg"), []byte("svg"), 0644)
	f.Rules = &card.Rules{Rules: []card.DisplayRule{{ID: "icon", Find: "x", Replace: `<img src="assets/icon.svg">`}}}
	before, _ := json.Marshal(f.State)
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	res, err := Push(context.Background(), c, f, PushOptions{DryRun: true, To: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(res)
	var out struct {
		Rules []struct {
			ID        string `json:"id"`
			Bytes     int    `json:"rewrittenBytes"`
			Remaining int    `json:"remainingBytes"`
		} `json:"rules"`
	}
	json.Unmarshal(raw, &out)
	want := len(`<img src="https://cdn.example/library/safe/icon.svg">`)
	if len(out.Rules) != 1 || out.Rules[0].ID != "icon" || out.Rules[0].Bytes != want || out.Rules[0].Remaining != 131072-want {
		t.Fatalf("report=%s; want bytes=%d", raw, want)
	}
	after, _ := json.Marshal(f.State)
	if string(before) != string(after) {
		t.Fatalf("dry-run mutated state: %s -> %s", before, after)
	}
}

func TestRuleReportMatchesWireAndBoundary(t *testing.T) {
	for _, n := range []int{131072, 131073} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			writes := 0
			actual := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
				}
				if strings.HasSuffix(r.URL.Path, "/author-asset") {
					var in struct{ Rules []card.DisplayRule }
					json.NewDecoder(r.Body).Decode(&in)
					actual = len(in.Rules[0].Replace)
					w.Write([]byte(`{"version":1}`))
					return
				}
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			f, _ := card.Init(t.TempDir(), "Boundary", "en")
			f.Rules = &card.Rules{Rules: []card.DisplayRule{{ID: "r", Find: "x", Replace: strings.Repeat("界", 43690) + strings.Repeat("x", n-131070)}}}
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
			res, err := Push(context.Background(), c, f, PushOptions{To: "owned", SkipMedia: true})
			if n == 131072 {
				if err != nil {
					t.Fatal(err)
				}
				if actual != n || res.Rules[0].RewrittenBytes != actual || res.Rules[0].RemainingBytes != 0 {
					t.Fatalf("report=%+v actual=%d", res.Rules, actual)
				}
			} else {
				if err == nil || writes != 0 {
					t.Fatalf("err=%v writes=%d", err, writes)
				}
				var detail interface{ ErrorDetail() any }
				if !errors.As(err, &detail) {
					t.Fatal("missing JSON error detail")
				}
			}
		})
	}
}

func TestRulesSavedCheckpointSurvivesLaterFailure(t *testing.T) {
	var writes []string
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			writes = append(writes, r.URL.Path)
		}
		if strings.HasSuffix(r.URL.Path, "/document") && fail {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"invalid"}`))
			return
		}
		w.Write([]byte(`{"version":7}`))
	}))
	defer srv.Close()
	f, _ := card.Init(t.TempDir(), "Resume", "en")
	f.Rules = &card.Rules{Rules: []card.DisplayRule{{Find: "x", Replace: "ok"}}}
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	_, err := Push(context.Background(), c, f, PushOptions{To: "owned", SkipMedia: true})
	if err == nil || !strings.Contains(err.Error(), "Display rules were saved, but the push is incomplete") {
		t.Fatalf("err=%v", err)
	}
	reloaded, err := card.Load(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.State.AuthorAssetVersion != 7 || reloaded.State.LocalHashes[card.SectionAuthorAsset] == "" || reloaded.State.LocalHashes[card.SectionCard] != "" {
		t.Fatalf("state=%+v", reloaded.State)
	}
	fail = false
	writes = nil
	if _, err := Push(context.Background(), c, reloaded, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(writes, []string{"/open/v1/role/owned/document"}) {
		t.Fatalf("retry wrote %v", writes)
	}
}

func TestRemoteLanguageControlsPreflight(t *testing.T) {
	for _, tc := range []struct {
		local, remote string
		allowed       bool
	}{{"en", "zh-Hant", false}, {"ja", "en", true}} {
		t.Run(tc.local+tc.remote, func(t *testing.T) {
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
				}
				json.NewEncoder(w).Encode(map[string]any{"language": tc.remote})
			}))
			defer srv.Close()
			f, _ := card.Init(t.TempDir(), "Language", tc.local)
			f.Definition = strings.Repeat("a", 10001)
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
			_, err := Push(context.Background(), c, f, PushOptions{To: "owned", SkipMedia: true})
			if (err == nil) != tc.allowed || (!tc.allowed && writes != 0) {
				t.Fatalf("allowed=%v err=%v writes=%d", tc.allowed, err, writes)
			}
		})
	}
}

func TestPreflightRejectsLateLorebookAndRuleLimits(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*card.Folder)
		want string
	}{
		{"total includes names", func(f *card.Folder) { f.Rules.Rules[0].Name = strings.Repeat("n", rulesTotalMax) }, "Display rules total"},
		{"response defaults", func(f *card.Folder) { f.Manifest.ResponseDefaults = card.ResponseDefaults{"length": "target"} }, "lengthTarget"},
		{"book name", func(f *card.Folder) { f.Lorebook = &card.Lorebook{Name: strings.Repeat("a", 121)} }, "name contains 121"},
		{"entry name", func(f *card.Folder) { f.Lorebook = &card.Lorebook{Entries: []card.LorebookEntry{{Content: "text"}}} }, "has no name"},
		{"entry category", func(f *card.Folder) {
			f.Lorebook = &card.Lorebook{Entries: []card.LorebookEntry{{Name: "x", Content: "text", Category: "bad"}}}
		}, "category"},
		{"entry options", func(f *card.Folder) {
			f.Lorebook = &card.Lorebook{Entries: []card.LorebookEntry{{Name: "x", Content: "text", MatchOptions: json.RawMessage(`{"scanDepth":101}`)}}}
		}, "out of range"},
		{"entry options type", func(f *card.Folder) {
			f.Lorebook = &card.Lorebook{Entries: []card.LorebookEntry{{Name: "x", Content: "text", MatchOptions: json.RawMessage(`{"groupOrder":"bad"}`)}}}
		}, "supported Lorebook options format"},
		{"explicit oversized group", func(f *card.Folder) {
			f.Lorebook = &card.Lorebook{Entries: []card.LorebookEntry{{Name: "x", Content: strings.Repeat("a", 12001), MatchOptions: json.RawMessage(`{"groupId":"g"}`)}}}
		}, "limit is 12000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
				}
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			f, _ := card.Init(t.TempDir(), "Late", "en")
			f.Rules = &card.Rules{Rules: []card.DisplayRule{{Find: "x", Replace: "ok"}}}
			tc.edit(f)
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
			_, err := Push(context.Background(), c, f, PushOptions{To: "owned", SkipMedia: true})
			if err == nil || !strings.Contains(err.Error(), tc.want) || writes != 0 {
				t.Fatalf("err=%v writes=%d", err, writes)
			}
		})
	}
}

func TestExpiredTrialStillRecreatesOnPush(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/open/v1/role/detail" {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"role_not_found"}`))
			return
		}
		w.Write([]byte(`{"clientKey":"key","roleId":"new","created":true}`))
	}))
	defer srv.Close()
	f, _ := card.Init(t.TempDir(), "Expired", "en")
	f.State.Target = "trial"
	f.State.RoleID = "expired"
	f.State.TrialKey = "key"
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	res, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.RoleID != "new" {
		t.Fatalf("result=%+v", res)
	}
}

func TestProviderWithoutPredictableURLs(t *testing.T) {
	for _, dry := range []bool{true, false} {
		t.Run(fmt.Sprint(dry), func(t *testing.T) {
			var writes []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes = append(writes, r.URL.Path)
				}
				if strings.HasSuffix(r.URL.Path, "/image/upload") {
					w.Write([]byte(`{"data":{"imageId":"1","imageUrl":"https://cdn.example/random.svg"}}`))
					return
				}
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			f, _ := card.Init(t.TempDir(), "Unknown", "en")
			os.MkdirAll(filepath.Join(f.Dir, "assets"), 0755)
			os.WriteFile(filepath.Join(f.Dir, "assets/icon.svg"), []byte("svg"), 0644)
			f.Rules = &card.Rules{Rules: []card.DisplayRule{{Find: "x", Replace: `<img src="assets/icon.svg">`}}}
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
			res, err := Push(context.Background(), c, f, PushOptions{To: "owned", DryRun: dry})
			if dry {
				if err == nil || !strings.Contains(err.Error(), "only after upload") || len(writes) != 0 {
					t.Fatalf("err=%v wrote=%v", err, writes)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if len(writes) < 3 || !strings.HasSuffix(writes[0], "/image/upload") || !strings.HasSuffix(writes[1], "/author-asset") || !strings.HasSuffix(writes[2], "/document") {
					t.Fatalf("writes=%v", writes)
				}
				if res.Rules[0].RewrittenBytes != len(`<img src="https://cdn.example/random.svg">`) {
					t.Fatalf("rules=%+v", res.Rules)
				}
			}
		})
	}
}

func TestPreflightPreservesAcceptedAgencyAliases(t *testing.T) {
	for _, agency := range []string{"assist", "lines"} {
		t.Run(agency, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) }))
			defer srv.Close()
			f, _ := card.Init(t.TempDir(), "Legacy", "en")
			f.Manifest.ResponseDefaults = card.ResponseDefaults{"agency": agency}
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
			if _, err := Push(context.Background(), c, f, PushOptions{To: "owned", SkipMedia: true}); err != nil {
				t.Fatal(err)
			}
			if f.Manifest.ResponseDefaults["agency"] != agency {
				t.Fatal("preflight rewrote the author's value")
			}
		})
	}
}

func TestURLExpansionRejectedBeforeMediaUpload(t *testing.T) {
	for _, dry := range []bool{true, false} {
		t.Run(fmt.Sprint(dry), func(t *testing.T) {
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
				}
				w.Write([]byte(`{"data":{"capabilities":{"relativePaths":true},"libraryPrefix":"https://cdn.example/library"}}`))
			}))
			defer srv.Close()
			f, _ := card.Init(t.TempDir(), "Expansion", "en")
			f.Manifest.Media.Folder = "safe"
			os.MkdirAll(filepath.Join(f.Dir, "assets"), 0755)
			os.WriteFile(filepath.Join(f.Dir, "assets/icon.svg"), []byte("svg"), 0644)
			f.Rules = &card.Rules{Rules: []card.DisplayRule{{ID: "expands", Find: "x", Replace: strings.Repeat("a", 131020) + `<img src="assets/icon.svg">`}}}
			before, _ := json.Marshal(f.State)
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
			_, err := Push(context.Background(), c, f, PushOptions{To: "owned", DryRun: dry})
			after, _ := json.Marshal(f.State)
			if err == nil || !strings.Contains(err.Error(), "131073") || writes != 0 || string(before) != string(after) {
				t.Fatalf("err=%v writes=%d state changed=%v", err, writes, string(before) != string(after))
			}
		})
	}
}

func TestPreflightRejectsImpossibleScriptSwitchBeforeRules(t *testing.T) {
	for _, remote := range []string{"en", "ja", "ko"} {
		t.Run(remote, func(t *testing.T) {
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
				}
				json.NewEncoder(w).Encode(map[string]any{"language": remote})
			}))
			defer srv.Close()
			f, _ := card.Init(t.TempDir(), "Script", "zh-Hant")
			f.Rules = &card.Rules{Rules: []card.DisplayRule{{Find: "x", Replace: "ok"}}}
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
			_, err := Push(context.Background(), c, f, PushOptions{To: "owned", SkipMedia: true, Force: true})
			if err == nil || !strings.Contains(err.Error(), "language") || writes != 0 {
				t.Fatalf("err=%v writes=%d", err, writes)
			}
		})
	}
}

func TestMalformedMediaRejectedBeforeRules(t *testing.T) {
	for _, raw := range []string{"https://[bad", "https://", "https://cdn.example:bad/image.svg"} {
		t.Run(raw, func(t *testing.T) {
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
				}
				w.Write([]byte(`{"language":"en","version":1}`))
			}))
			defer srv.Close()
			f, _ := card.Init(t.TempDir(), "Malformed", "en")
			f.Manifest.Media.Background = raw
			f.Rules = &card.Rules{Rules: []card.DisplayRule{{Find: "x", Replace: "new rule"}}}
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
			_, err := Push(context.Background(), c, f, PushOptions{To: "owned", SkipMedia: true})
			if err == nil || !strings.Contains(err.Error(), "media.background") || writes != 0 {
				t.Fatalf("err=%v writes=%d", err, writes)
			}
		})
	}
}

func TestPreflightDoesNotApplyExternalPolicyToHostedAliases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"language":"en","version":1}`)) }))
	defer srv.Close()
	f, _ := card.Init(t.TempDir(), "Alias", "en")
	f.Manifest.Media.Background = "http://hosted.example/library/image.svg"
	f.Rules = &card.Rules{Rules: []card.DisplayRule{{Find: "x", Replace: "rule"}}}
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	if _, err := Push(context.Background(), c, f, PushOptions{To: "owned", SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
}
