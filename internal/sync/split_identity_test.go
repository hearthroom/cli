package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

func TestSplitPushPreservesSourceAndUpdatesOriginal(t *testing.T) {
	for _, mode := range []string{"unlinked", "linked", "unlinked-no-id", "linked-no-id"} {
		t.Run(mode, func(t *testing.T) {
			linked := strings.HasPrefix(mode, "linked")
			f, _ := card.Init(t.TempDir(), "Synthetic", "zh-Hant")
			f.Lorebook = &card.Lorebook{ID: "b", Entries: []card.LorebookEntry{
				{ID: "original", Name: "Synthetic long", Content: strings.Repeat("字", 4368), Constant: true},
				{ID: "other", Name: "Other", Content: "Other content"},
			}}
			if strings.HasSuffix(mode, "no-id") {
				for i := range f.Lorebook.Entries {
					f.Lorebook.Entries[i].ID = ""
				}
			}
			sourceID := f.Lorebook.Entries[0].ID
			if linked {
				f.State.Target, f.State.RoleID, f.State.LorebookID = "owned", "r", "b"
			}
			if err := f.Save(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(f.Dir, card.LorebookFile)
			before, _ := os.ReadFile(path)
			before = append([]byte(" \n"), before...)
			if err := os.WriteFile(path, before, 0600); err != nil {
				t.Fatal(err)
			}
			remote := map[string]map[string]any{"original": {"entryId": "original", "name": "Synthetic long", "content": "Old content"}, "other": {"entryId": "other", "name": "Other", "content": "Other content"}}
			next := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reply := func(v any) { json.NewEncoder(w).Encode(v) }
				switch r.URL.Path {
				case "/open/v1/role/author-asset":
					reply(map[string]any{"status": "none"})
				case "/open/v1/role/detail":
					reply(map[string]any{"language": "zh-Hant"})
				case "/open/v1/worldbook/limits":
					reply(map[string]any{"entryContentMaxByLanguage": map[string]int{"zh-Hant": 4000}})
				case "/open/v1/worldbook/detail":
					reply(map[string]any{"entryContentMax": 4000})
				case "/open/v1/worldbook/bindings":
					reply(map[string]any{"bindings": []any{map[string]any{"worldbookId": "b"}}})
				case "/open/v1/worldbook/entry/list":
					rows := []map[string]any{}
					for _, row := range remote {
						rows = append(rows, row)
					}
					reply(map[string]any{"entries": rows})
				case "/open/v1/worldbook/b/document":
					var body struct{ Entries []map[string]any }
					json.NewDecoder(r.Body).Decode(&body)
					ids := []string{}
					for _, op := range body.Entries {
						id, _ := op["entryId"].(string)
						if op["op"] == "delete" {
							delete(remote, id)
							continue
						}
						if op["op"] == "create" {
							next++
							id = fmt.Sprintf("new-%d", next)
							ids = append(ids, id)
						}
						op["entryId"] = id
						remote[id] = op
					}
					reply(map[string]any{"createdEntryIds": ids})
				case "/open/v1/role/r/document", "/open/v1/role/r/welcome":
					reply(map[string]any{})
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "test", nil })
			push := func() {
				t.Helper()
				res, err := Push(context.Background(), c, f, PushOptions{To: "r", SkipMedia: true, Force: true})
				if err != nil {
					t.Fatal(err)
				}
				if len(f.Lorebook.Entries[0].Content) > 4000 && (len(res.Grouped) != 1 || res.Grouped[0].Characters != 4368 || res.Grouped[0].Limit != 4000 || res.Grouped[0].Parts != 2) {
					t.Fatalf("split report=%+v", res.Grouped)
				}
				after, _ := os.ReadFile(path)
				if string(after) != string(before) {
					t.Error("push rewrote lorebook.json")
				}
			}
			push()
			if len(remote) != 3 || remote["original"]["content"] != strings.Repeat("字", 4000) || remote["original"]["isConstant"] != true {
				t.Fatalf("original not replaced by first fragment; count=%d original=%v", len(remote), remote["original"])
			}
			if f.Lorebook.Entries[0].ID != sourceID {
				t.Fatal("original in-memory ID changed")
			}
			f, _ = card.Load(f.Dir)
			push()
			if len(remote) != 3 || next != 1 {
				t.Fatalf("repeated push duplicated fragments: count=%d creates=%d", len(remote), next)
			}

			f.Lorebook.Entries[0].Name = "Renamed source"
			if err := f.Save(); err != nil {
				t.Fatal(err)
			}
			before, _ = os.ReadFile(path)
			push()
			if len(remote) != 3 || remote["original"]["name"] != "Renamed source" || next != 1 {
				t.Fatal("rename changed fragment identity")
			}
			f.Lorebook.Entries[0], f.Lorebook.Entries[1] = f.Lorebook.Entries[1], f.Lorebook.Entries[0]
			f.Lorebook.Entries[1].Content = "Shortened"
			if err := f.Save(); err != nil {
				t.Fatal(err)
			}
			before, _ = os.ReadFile(path)
			push()
			if len(remote) != 2 || remote["original"]["content"] != "Shortened" {
				t.Fatal("reorder/shortening left old fragments")
			}
			// Old known leftovers still require the author's explicit prune, then disappear.
			remote["foreign"] = map[string]any{"entryId": "foreign", "name": "External entry", "content": "Keep"}
			remote["legacy"] = map[string]any{"entryId": "legacy", "name": "Legacy duplicate", "content": "Old"}
			f.State.LorebookEntryIDs = append(f.State.LorebookEntryIDs, "legacy")
			if _, err := Push(context.Background(), c, f, PushOptions{To: "r", SkipMedia: true, Force: true}); err == nil || !strings.Contains(err.Error(), "--prune") {
				t.Fatalf("legacy deletion lost protection: %v", err)
			}
			if len(remote) != 4 {
				t.Fatal("prune refusal deleted data")
			}
			if _, err := Push(context.Background(), c, f, PushOptions{To: "r", SkipMedia: true, Force: true, Prune: true}); err != nil {
				t.Fatal(err)
			}
			if len(remote) != 3 || remote["legacy"] != nil || remote["foreign"] == nil {
				t.Fatal("explicit prune left known legacy duplicate")
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(before) {
				t.Fatal("prune changed source file")
			}

		})
	}
}
