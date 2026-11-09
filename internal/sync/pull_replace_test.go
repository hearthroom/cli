package sync

import (
	"context"
	"encoding/json"
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

// Replacing a folder must also replace absence. Otherwise the next load and
// push silently publish the previous card's optional content to the new card.
func TestPullReplacesOptionalContentBeforeNextPush(t *testing.T) {
	for _, alts := range [][]string{nil, {"New alternate."}} {
		t.Run(string(rune('0'+len(alts)))+"_remote_alternates", func(t *testing.T) {
			f := pullReplacementFixture(t)
			var pushed map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/open/v1/role/detail":
					json.NewEncoder(w).Encode(map[string]any{"characterRoleId": "new-source", "roleName": "New source", "language": "en", "creationMethod": "trial", "roleDetailDesc": "New definition.", "roleWelcome": "New opening.", "roleWelcomeAlternates": alts})
				case r.URL.Path == "/open/v1/worldbook/bindings":
					json.NewEncoder(w).Encode(map[string]any{"bindings": []any{}})
				case strings.HasPrefix(r.URL.Path, "/open/v1/trial-cards/"):
					if err := json.NewDecoder(r.Body).Decode(&pushed); err != nil {
						t.Error(err)
					}
					json.NewEncoder(w).Encode(map[string]any{"roleId": "new-push", "created": true, "sections": map[string]string{}})
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
			res, err := Pull(context.Background(), c, "new-source", f.Dir, PullOptions{Force: true})
			if err != nil || len(res.Warnings) != 0 {
				t.Fatalf("pull = %+v, %v", res, err)
			}
			loaded, err := card.Load(f.Dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Push(context.Background(), c, loaded, PushOptions{SkipMedia: true}); err != nil {
				t.Fatal(err)
			}
			for _, section := range []string{"worldbook", "authorAsset", "state"} {
				if _, exists := pushed[section]; exists {
					t.Errorf("next push republished old %s: %v", section, pushed[section])
				}
			}
			welcome := pushed["welcome"].(map[string]any)
			got, _ := welcome["alternates"].([]any)
			want := make([]any, len(alts))
			for i, text := range alts {
				want[i] = text
			}
			if len(got) != len(want) || len(want) > 0 && !reflect.DeepEqual(got, want) {
				t.Errorf("next push alternatives = %v, want %v", got, want)
			}
			for _, stale := range []string{card.LorebookFile, card.RulesFile, card.IntroFile, card.VariablesFile, card.VariableRulesFile, card.PanelFile, "openings/old-one.md", "openings/old-two.md"} {
				if _, err := os.Lstat(filepath.Join(f.Dir, stale)); !os.IsNotExist(err) {
					t.Errorf("obsolete managed file %s still exists: %v", stale, err)
				}
			}
			for _, untouched := range []string{card.ReadmeFile, card.AgentsFile, "assets/keep.txt", "openings/notes.txt", "openings/drafts/keep.md"} {
				if b, err := os.ReadFile(filepath.Join(f.Dir, untouched)); err != nil || string(b) != "keep\n" {
					t.Errorf("unmanaged %s changed: %q, %v", untouched, b, err)
				}
			}
		})
	}
}

func pullReplacementFixture(t *testing.T) *card.Folder {
	t.Helper()
	f, err := card.Init(t.TempDir(), "Old source", "en")
	if err != nil {
		t.Fatal(err)
	}
	f.Definition, f.Welcome = "Old definition.", "Old opening."
	intro := "Old introduction."
	f.Intro = &intro
	f.Lorebook = &card.Lorebook{Name: "Old book", Entries: []card.LorebookEntry{{Name: "Old entry", Content: "Old lore."}}}
	f.Rules = &card.Rules{Rules: []card.DisplayRule{{ID: "old", Find: "/old/", Replace: "Old rule.", Enabled: true}}}
	f.Alternates = []card.Opening{{File: "openings/old-one.md", Text: "Old alternate one."}, {File: "openings/old-two.md", Text: "Old alternate two."}}
	f.Variables = &card.Variables{Convention: "plain", Data: []byte(`{"old":1}`), Rules: "Old rules.", Panel: "<b>{{old}}</b>"}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{card.ReadmeFile, card.AgentsFile, "assets/keep.txt", "openings/notes.txt", "openings/drafts/keep.md"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(f.Dir, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(f.Dir, path), []byte("keep\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// A failed optional read is not confirmation that the remote section is absent.
func TestPullReadWarningsDoNotRemoveOptionalFiles(t *testing.T) {
	f := pullReplacementFixture(t)
	before := map[string][]byte{}
	for _, name := range []string{card.LorebookFile, card.RulesFile} {
		before[name], _ = os.ReadFile(filepath.Join(f.Dir, name))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open/v1/role/detail" {
			json.NewEncoder(w).Encode(map[string]any{"characterRoleId": "new-source", "roleName": "New source", "language": "en", "roleWelcome": "New opening."})
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	res, err := Pull(context.Background(), c, "new-source", f.Dir, PullOptions{Force: true})
	if err != nil || len(res.Warnings) != 2 {
		t.Fatalf("pull = %+v, %v", res, err)
	}
	for name, want := range before {
		if got, err := os.ReadFile(filepath.Join(f.Dir, name)); err != nil || string(got) != string(want) {
			t.Errorf("failed read replaced %s: %q, %v", name, got, err)
		}
	}
}

func TestPullDoesNotCleanThroughOpeningsSymlink(t *testing.T) {
	f, err := card.Init(t.TempDir(), "Local source", "en")
	if err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	keep := filepath.Join(external, "keep.md")
	if err := os.WriteFile(keep, []byte("outside the card folder"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(f.Dir, card.OpeningsDir)); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	before, _ := os.ReadFile(filepath.Join(f.Dir, card.ManifestFile))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open/v1/role/detail":
			json.NewEncoder(w).Encode(map[string]any{"characterRoleId": "new-source", "roleName": "New source", "language": "en", "roleWelcome": "New opening."})
		case "/open/v1/worldbook/bindings":
			json.NewEncoder(w).Encode(map[string]any{"bindings": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	if _, err := Pull(context.Background(), c, "new-source", f.Dir, PullOptions{Force: true}); err == nil {
		t.Fatal("pull accepted an alternate-openings symlink before replacing managed files")
	}
	if got, _ := os.ReadFile(keep); string(got) != "outside the card folder" {
		t.Errorf("pull changed a file outside the card folder: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(f.Dir, card.ManifestFile)); string(got) != string(before) {
		t.Error("pull wrote local content before rejecting the alternate-openings symlink")
	}
}
