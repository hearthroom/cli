package sync

import (
	"context"
	"encoding/json"
	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLostReplyCanonicalDefaults(t *testing.T) {
	f, _ := card.Init(t.TempDir(), "Review", "en")
	f.Lorebook = &card.Lorebook{Name: "Review", Entries: []card.LorebookEntry{{Name: "one", Content: "text", MatchOptions: json.RawMessage(`{"scanDepth":0,"groupOrder":0,"groupId":"","extensions":{},"selectiveLogic":0,"caseSensitive":false,"matchWholeWords":false}`)}}}
	f.Save()
	remote := []map[string]any{}
	lost := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := func(v any) { json.NewEncoder(w).Encode(v) }
		switch r.URL.Path {
		case "/open/v1/worldbook/limits":
			reply(map[string]any{"maxOpsPerCall": 200, "entryContentMaxByLanguage": map[string]int{"en": 12000}, "trialMaxEntries": 1000})
		case "/open/v1/role":
			reply(map[string]any{"roleId": "r", "roleVisibility": "private"})
		case "/open/v1/role/detail":
			reply(map[string]any{"language": "en"})
		case "/open/v1/role/r/document":
			reply(map[string]any{})
		case "/open/v1/worldbook/bindings":
			reply(map[string]any{"bindings": []any{}})
		case "/open/v1/worldbook":
			reply(map[string]any{"worldbookId": "b"})
		case "/open/v1/worldbook/detail":
			reply(map[string]any{"entryContentMax": 12000})
		case "/open/v1/worldbook/entry/list":
			reply(map[string]any{"entries": remote})
		case "/open/v1/worldbook/b/document":
			var body struct{ Entries []map[string]any }
			json.NewDecoder(r.Body).Decode(&body)
			ids := []string{}
			for _, op := range body.Entries {
				id := "e1"
				if len(remote) > 0 {
					id = "e2"
				}
				ids = append(ids, id)
				op["entryId"] = id
				// Actual Provider MatchOptions JSON omits scanDepth=0 and emits these defaults.
				op["matchOptions"] = map[string]any{"caseSensitive": false, "matchWholeWords": false, "selectiveLogic": 0}
				remote = append(remote, op)
			}
			if !lost {
				lost = true
				w.WriteHeader(503)
				reply(map[string]any{"error": "temporarily_unavailable"})
				return
			}
			reply(map[string]any{"createdEntryIds": ids})
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "test", nil })
	opts := PushOptions{Create: true, SkipMedia: true}
	if _, err := Push(context.Background(), c, f, opts); err == nil {
		t.Fatal("want lost reply")
	}
	f, _ = card.Load(f.Dir)
	if _, err := Push(context.Background(), c, f, opts); err != nil {
		t.Fatal(err)
	}
	if len(remote) != 1 {
		t.Fatalf("lost reply with canonical default produced %d entries; want 1", len(remote))
	}
}

func TestCreateResumeKeepsOldBookUntouched(t *testing.T) {
	f, _ := card.Init(t.TempDir(), "Review", "en")
	f.State.RoleID = "old-role"
	f.State.Target = "owned"
	f.State.LorebookID = "old-book"
	f.Lorebook = &card.Lorebook{ID: "old-book", Name: "Review", Entries: []card.LorebookEntry{{ID: "old-entry", Name: "one", Content: "changed text"}}}
	f.Save()
	fail := true
	oldWrites := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := func(v any) { json.NewEncoder(w).Encode(v) }
		switch r.URL.Path {
		case "/open/v1/worldbook/limits":
			reply(map[string]any{"maxOpsPerCall": 200, "entryContentMaxByLanguage": map[string]int{"en": 12000}, "trialMaxEntries": 1000})
		case "/open/v1/role":
			reply(map[string]any{"roleId": "new-role", "roleVisibility": "private"})
		case "/open/v1/role/detail":
			reply(map[string]any{"language": "en"})
		case "/open/v1/role/new-role/document":
			if fail {
				fail = false
				w.WriteHeader(503)
				reply(map[string]any{"error": "temporarily_unavailable"})
				return
			}
			reply(map[string]any{})
		case "/open/v1/worldbook/bindings":
			reply(map[string]any{"bindings": []any{}})
		case "/open/v1/worldbook":
			reply(map[string]any{"worldbookId": "new-book"})
		case "/open/v1/worldbook/detail":
			reply(map[string]any{"entryContentMax": 12000})
		case "/open/v1/worldbook/entry/list":
			if r.URL.Query().Get("worldbookId") == "old-book" {
				reply(map[string]any{"entries": []map[string]any{{"entryId": "old-entry", "name": "one", "content": "original"}}})
			} else {
				reply(map[string]any{"entries": []any{}})
			}
		case "/open/v1/worldbook/old-book/document":
			oldWrites++
			reply(map[string]any{"createdEntryIds": []string{}})
		case "/open/v1/worldbook/new-book/document":
			reply(map[string]any{"createdEntryIds": []string{"new-entry"}})
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "test", nil })
	opts := PushOptions{Create: true, SkipMedia: true}
	if _, err := Push(context.Background(), c, f, opts); err == nil {
		t.Fatal("want first card write failure")
	}
	f, _ = card.Load(f.Dir)
	if _, err := Push(context.Background(), c, f, opts); err != nil {
		t.Fatal(err)
	}
	if oldWrites != 0 {
		t.Fatalf("resuming --create wrote original book %d times; want 0", oldWrites)
	}
}
