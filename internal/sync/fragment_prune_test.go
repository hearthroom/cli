package sync

import (
	"context"
	"encoding/json"
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

func TestOwnedPushObsoleteFragmentsDoNotRequirePrune(t *testing.T) {
	for _, change := range []string{"shorten", "author group", "larger limit"} {
		t.Run(change, func(t *testing.T) {
			f, _ := card.Init(t.TempDir(), "Synthetic", "en")
			f.State.Target, f.State.RoleID, f.State.LorebookID = "owned", "r", "b"
			f.Lorebook = &card.Lorebook{ID: "b", Entries: []card.LorebookEntry{{Name: "Synthetic", Content: strings.Repeat("text ", 6), Disabled: true}}}
			limit, next, writes := 20, 0, 0
			remote := map[string]map[string]any{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reply := func(v any) { json.NewEncoder(w).Encode(v) }
				switch r.URL.Path {
				case "/open/v1/worldbook/limits":
					reply(map[string]any{"maxOpsPerCall": 1, "entryContentMaxByLanguage": map[string]int{"en": limit}})
				case "/open/v1/worldbook/detail":
					reply(map[string]any{"entryContentMax": limit})
				case "/open/v1/role/detail":
					reply(map[string]any{"language": "en"})
				case "/open/v1/worldbook/entry/list":
					rows := []map[string]any{}
					for _, e := range remote {
						rows = append(rows, e)
					}
					reply(map[string]any{"entries": rows})
				case "/open/v1/worldbook/b/document":
					writes++
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
							id = fmt.Sprintf("e%d", next)
							ids = append(ids, id)
						}
						op["entryId"] = id
						remote[id] = op
					}
					reply(map[string]any{"createdEntryIds": ids})
				default:
					reply(map[string]any{})
				}
			}))
			defer srv.Close()
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "test", nil })
			push := func(opts PushOptions) *PushResult {
				t.Helper()
				opts.SkipMedia = true
				res, err := Push(context.Background(), c, f, opts)
				if err != nil {
					t.Fatal(err)
				}
				return res
			}
			push(PushOptions{})
			if len(remote) != 2 {
				t.Fatalf("initial fragments=%d", len(remote))
			}
			firstID := f.Lorebook.Entries[0].ID
			switch change {
			case "shorten":
				f.Lorebook.Entries[0].Content = "Short text"
			case "author group":
				f.Lorebook.Entries[0].Content = "Short text"
				f.Lorebook.Entries[0].MatchOptions = json.RawMessage(`{"groupId":"author","groupOrder":4}`)
			case "larger limit":
				limit = 40
			}
			if err := f.Save(); err != nil {
				t.Fatal(err)
			}
			// Force is needed for a provider-limit-only change; it does not authorize prune.
			res := push(PushOptions{Force: true})
			if len(remote) != 1 || remote[firstID] == nil || remote[firstID]["isEnabled"] != false || len(res.PruneEntries) != 0 {
				t.Fatalf("wrong fragment cleanup: %v prune=%v", remote, res.PruneEntries)
			}
			if f.Lorebook.Entries[0].ID != firstID {
				t.Fatal("first fragment identity changed")
			}
			if change == "author group" && remote[firstID]["matchOptions"].(map[string]any)["groupId"] != "author" {
				t.Fatal("author group lost")
			}
			before := writes
			push(PushOptions{})
			if writes != before {
				t.Fatal("repeat push wrote unchanged content")
			}
			// Removing the whole local entry still needs prune; dry-run lists it.
			f.Lorebook.Entries = nil
			if err := f.Save(); err != nil {
				t.Fatal(err)
			}
			statePath := filepath.Join(f.Dir, ".hearthroom", "state.json")
			stateBefore, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}
			bookBefore, err := os.ReadFile(filepath.Join(f.Dir, card.LorebookFile))
			if err != nil {
				t.Fatal(err)
			}
			res = push(PushOptions{DryRun: true})
			if !reflect.DeepEqual(res.PruneEntries, []string{"Synthetic"}) || writes != before || len(remote) != 1 {
				t.Fatalf("dry-run prune=%v writes=%d", res.PruneEntries, writes-before)
			}
			stateAfter, _ := os.ReadFile(statePath)
			bookAfter, _ := os.ReadFile(filepath.Join(f.Dir, card.LorebookFile))
			if string(stateBefore) != string(stateAfter) || string(bookBefore) != string(bookAfter) {
				t.Fatal("dry-run changed local files")
			}
			res = push(PushOptions{DryRun: true, Create: true})
			if len(res.PruneEntries) != 0 || writes != before {
				t.Fatal("fresh-create dry-run previews deleting the old card")
			}
			res = push(PushOptions{DryRun: true, To: "other", Force: true})
			if len(res.PruneEntries) != 0 || writes != before {
				t.Fatal("relink dry-run reused the old card identities")
			}
			if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err == nil || !strings.Contains(err.Error(), "--prune") {
				t.Fatalf("whole-entry deletion bypassed protection: %v", err)
			}
		})
	}
}

func TestPrunePlanListsOnlyWholeRemovedSources(t *testing.T) {
	f, _ := card.Init(t.TempDir(), "Synthetic mixed plan", "en")
	f.Lorebook = &card.Lorebook{Entries: []card.LorebookEntry{{ID: "a0", Name: "Retained", Content: "Short"}}}
	f.State.LorebookEntryMap = map[string]string{"0:0": "a0", "0:1": "a1", "1:0": "b0", "1:1": "b1"}
	f.State.LorebookEntryIDs = []string{"a0", "a1", "b0", "b1"}
	existing := []card.RemoteEntry{{EntryID: "a0", Name: "Retained"}, {EntryID: "a1", Name: "Retained"}, {EntryID: "b0", Name: "Removed"}, {EntryID: "b1", Name: "Removed"}}
	p, _ := f.Build(nil)
	opts := PushOptions{entryKeys: []string{"0:0"}}
	plan := planLorebook(f, p.Worldbook, existing, opts)
	if !reflect.DeepEqual(plan.FragmentDeletes, []string{"a1"}) {
		t.Fatalf("fragment cleanup=%v", plan.FragmentDeletes)
	}
	if got := pruneEntryNames(f, plan, existing); !reflect.DeepEqual(got, []string{"Removed", "Removed"}) {
		t.Fatalf("prune list=%v", got)
	}
	// Completed update batches may rewrite positional maps before a later retry.
	f.State.LorebookEntryMap = nil
	if got := pruneEntryNames(f, plan, existing); !reflect.DeepEqual(got, []string{"Removed", "Removed"}) {
		t.Fatalf("saved plan lost fragment classification: %v", got)
	}
}
