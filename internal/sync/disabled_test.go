package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

func TestOwnedPushPreservesDisabledAndRequiresExplicitDeletion(t *testing.T) {
	f, err := card.Init(t.TempDir(), "Synthetic lifecycle", "en")
	if err != nil {
		t.Fatal(err)
	}
	f.State.Target, f.State.RoleID, f.State.LorebookID = "owned", "r", "b"
	f.Lorebook = &card.Lorebook{ID: "b", Entries: []card.LorebookEntry{
		{Name: "remove first", Content: "Remove this."},
		{Name: "disabled split", Content: strings.Repeat("text ", 6), Disabled: true, MatchOptions: json.RawMessage(`{"matchWholeWords":true}`)},
		{Name: "disabled author group", Content: "Keep this entry.", Disabled: true, Keywords: []string{"/keep/i"}, SecondaryKeywords: []string{"second"}, MatchOptions: json.RawMessage(`{"matchWholeWords":true,"groupId":"author","groupOrder":7}`)},
	}}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	remote := map[string]map[string]any{}
	next, writes := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := func(v any) { json.NewEncoder(w).Encode(v) }
		switch r.URL.Path {
		case "/open/v1/worldbook/limits":
			reply(map[string]any{"maxOpsPerCall": 2, "entryContentMaxByLanguage": map[string]int{"en": 20}})
		case "/open/v1/role/detail":
			reply(map[string]any{"language": "en"})
		case "/open/v1/worldbook/detail":
			reply(map[string]any{"entryContentMax": 20})
		case "/open/v1/role/r/document":
			reply(map[string]any{})
		case "/open/v1/worldbook/entry/list":
			entries := []map[string]any{}
			for _, e := range remote {
				entries = append(entries, e)
			}
			reply(map[string]any{"entries": entries})
		case "/open/v1/worldbook/b/document":
			writes++
			var body struct{ Entries []map[string]any }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
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
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "test", nil })
	push := func(opts PushOptions) error {
		t.Helper()
		var err error
		f, err = card.Load(f.Dir)
		if err != nil {
			t.Fatal(err)
		}
		opts.SkipMedia = true
		_, err = Push(context.Background(), c, f, opts)
		return err
	}
	if err := push(PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(remote) != 4 {
		t.Fatalf("push lost disabled entries: got %d, want 4", len(remote))
	}
	firstID, groupID := f.Lorebook.Entries[0].ID, f.Lorebook.Entries[2].ID
	assertKept := func() {
		t.Helper()
		for id, e := range remote {
			if strings.HasPrefix(e["name"].(string), "disabled") && e["isEnabled"] != false {
				t.Fatalf("entry %s enabled=%v", id, e["isEnabled"])
			}
		}
		e := remote[groupID]
		if !reflect.DeepEqual(e["keywords"], []any{"/keep/i"}) || !reflect.DeepEqual(e["secondaryKeywords"], []any{"second"}) || !reflect.DeepEqual(e["matchOptions"], map[string]any{"matchWholeWords": true, "groupId": "author", "groupOrder": float64(7)}) {
			t.Fatalf("matching settings changed: %v", e)
		}
	}
	assertKept()
	before := writes
	if err := push(PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if writes != before || len(remote) != 4 {
		t.Fatal("second push is not idempotent")
	}
	// Removing the first source shifts the fragment keys of every survivor.
	f.Lorebook.Entries = f.Lorebook.Entries[1:]
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	err = push(PushOptions{})
	if err == nil || !strings.Contains(err.Error(), "--prune") || !strings.Contains(err.Error(), "remove first") {
		t.Fatalf("want explicit deletion preview, got %v", err)
	}
	if writes != before || len(remote) != 4 {
		t.Fatal("unconfirmed deletion wrote the Lorebook")
	}
	if err := push(PushOptions{Force: true}); err == nil || !strings.Contains(err.Error(), "--prune") || writes != before {
		t.Fatalf("saved deletion plan bypassed prune protection: %v", err)
	}
	// Another author's tool created this entry: even explicit pruning keeps it.
	remote["external"] = map[string]any{"entryId": "external", "name": "external", "content": "keep", "isEnabled": false}
	if err := push(PushOptions{Prune: true}); err != nil {
		t.Fatal(err)
	}
	if len(remote) != 4 || remote[firstID] != nil || remote[groupID] == nil || remote["external"] == nil {
		t.Fatalf("wrong deletion targets: %v", remote)
	}
	assertKept()
	before = writes
	if err := push(PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if writes != before {
		t.Fatal("post-delete push is not idempotent")
	}
}

func TestUpgradeReconcilesLegacyInFlightBeforeNewPlan(t *testing.T) {
	f, _ := card.Init(t.TempDir(), "Synthetic upgrade", "en")
	f.State.Target, f.State.RoleID, f.State.LorebookID = "owned", "r", "b"
	f.Lorebook = &card.Lorebook{ID: "b", Entries: []card.LorebookEntry{{Name: "one", Content: "text", Disabled: true}}}
	f.State.LorebookUpload = &card.LorebookUpload{Digest: "old-cli-digest", InFlight: true, BatchSize: 1, Keys: []string{"0:0"}, Ops: []map[string]any{{"op": "create", "name": "one", "content": "text", "keywords": []string{}, "isEnabled": true}}}
	remote := map[string]any{"entryId": "e", "name": "one", "content": "text", "keywords": []string{}, "isEnabled": true}
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := func(v any) { json.NewEncoder(w).Encode(v) }
		switch r.URL.Path {
		case "/open/v1/worldbook/entry/list":
			reply(map[string]any{"entries": []any{remote}})
		case "/open/v1/worldbook/b/document":
			writes++
			var body struct{ Entries []map[string]any }
			json.NewDecoder(r.Body).Decode(&body)
			if len(body.Entries) != 1 || body.Entries[0]["op"] != "update" || body.Entries[0]["entryId"] != "e" {
				t.Errorf("legacy reconciliation duplicated/lost identity: %v", body.Entries)
			}
			remote["isEnabled"] = body.Entries[0]["isEnabled"]
			reply(map[string]any{"createdEntryIds": []string{}})
		default:
			reply(map[string]any{})
		}
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "test", nil })
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	if writes != 1 || remote["isEnabled"] != false {
		t.Fatalf("writes=%d enabled=%v", writes, remote["isEnabled"])
	}
}

func TestTrialPushIncludesDisabledEntries(t *testing.T) {
	f, _ := card.Init(t.TempDir(), "Synthetic trial", "en")
	f.Lorebook = &card.Lorebook{Entries: []card.LorebookEntry{{Name: "disabled", Content: "Retained", Disabled: true}, {Name: "active", Content: "Active"}}}
	var entries []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			var body struct {
				Worldbook struct{ Entries []map[string]any }
			}
			json.NewDecoder(r.Body).Decode(&body)
			entries = body.Worldbook.Entries
		}
		json.NewEncoder(w).Encode(map[string]any{"clientKey": "k", "roleId": "r", "sections": map[string]string{}})
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "test", nil })
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0]["isEnabled"] != false || entries[1]["isEnabled"] != true {
		t.Fatalf("trial entries=%v", entries)
	}
}

func TestPushRejectsBlankEntryBeforeWrites(t *testing.T) {
	f, _ := card.Init(t.TempDir(), "Synthetic blank", "en")
	f.Lorebook = &card.Lorebook{Entries: []card.LorebookEntry{{Name: "unfinished", Content: " \n", Disabled: true}}}
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			writes++
		}
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "test", nil })
	_, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true})
	if err == nil || !strings.Contains(err.Error(), "unfinished") || writes != 0 {
		t.Fatalf("err=%v writes=%d", err, writes)
	}
}
