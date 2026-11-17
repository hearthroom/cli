package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNamedBooksUseStableBindingsAndIndependentState(t *testing.T) {
	var documents []string
	var keys []string
	next := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/open/v1/worldbook/bindings":
			fmt.Fprint(w, `{"bindings":[{"worldbookId":"book-a","name":"A","contextKey":"a","mainRecall":false},{"worldbookId":"unrelated","name":"Do not touch"}]}`)
		case r.Method == "GET" && r.URL.Path == "/open/v1/worldbook/entry/list":
			fmt.Fprint(w, `{"entries":[]}`)
		case r.Method == "GET" && r.URL.Path == "/open/v1/worldbook/detail":
			fmt.Fprint(w, `{"version":1,"name":"A"}`)
		case r.Method == "POST" && r.URL.Path == "/open/v1/worldbook":
			next++
			fmt.Fprintf(w, `{"worldbookId":"new-%d","version":1}`, next)
		case r.Method == "POST":
			var doc map[string]any
			_ = json.NewDecoder(r.Body).Decode(&doc)
			documents = append(documents, r.URL.Path)
			if b, ok := doc["binding"].(map[string]any); ok {
				key, _ := b["contextKey"].(string)
				keys = append(keys, key)
			}
			fmt.Fprintf(w, `{"createdEntryIds":["entry-%d"],"version":2}`, len(documents))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	f, err := card.Init(t.TempDir(), "Synthetic", "en")
	if err != nil {
		t.Fatal(err)
	}
	off := false
	f.Lorebooks = map[string]*card.Lorebook{"a": {ID: "a", Name: "A", MainRecall: &off, Entries: []card.LorebookEntry{{Name: "A", Content: "One", Keywords: []string{}}}}, "b": {ID: "b", Name: "B", Entries: []card.LorebookEntry{{Name: "B", Content: "Two", Keywords: []string{}}}}}
	f.State.RoleID = "role"
	f.State.Target = "owned"
	f.State.LorebookID = "legacy-book"
	p, err := f.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := api.New(server.URL, server.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	if err := pushNamedLorebooks(context.Background(), client, f, "role", p.Worldbooks, PushOptions{}, &PushResult{}); err != nil {
		t.Fatal(err)
	}
	if len(documents) != 2 || documents[0] != "/open/v1/worldbook/book-a/document" || documents[1] != "/open/v1/worldbook/new-1/document" {
		t.Fatalf("wrong book written: %v", documents)
	}
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("stable bindings not saved: %v", keys)
	}
	if f.State.LorebookID != "legacy-book" || len(f.State.Lorebooks) != 2 || f.State.Lorebooks["a"].ID != "book-a" || f.State.Lorebooks["b"].ID != "new-1" {
		t.Fatalf("mixed state: %+v", f.State)
	}
	raw, err := os.ReadFile(filepath.Join(f.Dir, card.StateDir, card.StateFile))
	if err != nil {
		t.Fatal(err)
	}
	var saved card.State
	if err = json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.LorebookID != "legacy-book" || len(saved.Lorebooks) != 2 {
		t.Fatalf("nested save clobbered parent state: %s", raw)
	}
	if f.Lorebooks["a"].ID != "a" || f.Lorebooks["b"].ID != "b" {
		t.Fatal("stable source IDs rewritten as remote UUIDs")
	}
}

func TestNamedBookRemovalRequiresPruneAndOnlyUnbindsTrackedSource(t *testing.T) {
	writes := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, `{"bindings":[{"worldbookId":"tracked-book","name":"Tracked","contextKey":"old"},{"worldbookId":"foreign-book","name":"Untracked","contextKey":"other"}]}`)
			return
		}
		writes = append(writes, r.URL.Path)
		fmt.Fprint(w, `{"unbound":true}`)
	}))
	defer server.Close()
	f, err := card.Init(t.TempDir(), "Synthetic", "en")
	if err != nil {
		t.Fatal(err)
	}
	f.Lorebooks = map[string]*card.Lorebook{}
	f.State.Lorebooks = map[string]card.LorebookSync{"old": {ID: "tracked-book"}}
	c := api.New(server.URL, server.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	if err := pushNamedLorebooks(context.Background(), c, f, "role", map[string]any{}, PushOptions{}, &PushResult{}); err == nil || len(writes) != 0 {
		t.Fatalf("removal not guarded: %v %v", err, writes)
	}
	if err := pushNamedLorebooks(context.Background(), c, f, "role", map[string]any{}, PushOptions{Prune: true}, &PushResult{}); err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 || writes[0] != "/open/v1/worldbook/tracked-book/unbind" {
		t.Fatalf("wrong source removed: %v", writes)
	}
	if _, ok := f.State.Lorebooks["old"]; ok {
		t.Fatal("confirmed removal left stale sync state")
	}
}

func TestNamedBooksResumeLostReplyAfterReloadWithoutDuplicateEntries(t *testing.T) {
	type remoteBook struct {
		Name, Key string
		Version   int
		Entries   []map[string]any
	}
	books := map[string]*remoteBook{}
	created, documents := 0, 0
	lost := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/open/v1/worldbook/bindings" {
			rows := []map[string]any{}
			for id, b := range books {
				if b.Key != "" {
					rows = append(rows, map[string]any{"worldbookId": id, "name": b.Name, "contextKey": b.Key})
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"bindings": rows})
			return
		}
		if r.Method == "GET" {
			b := books[r.URL.Query().Get("worldbookId")]
			if b == nil {
				t.Error("unknown book read")
				w.WriteHeader(404)
				return
			}
			if strings.HasSuffix(r.URL.Path, "/detail") {
				_ = json.NewEncoder(w).Encode(map[string]any{"name": b.Name, "version": b.Version})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"entries": b.Entries})
			return
		}
		var doc map[string]any
		_ = json.NewDecoder(r.Body).Decode(&doc)
		if r.URL.Path == "/open/v1/worldbook" {
			created++
			id := fmt.Sprintf("book-%d", created)
			books[id] = &remoteBook{Name: doc["name"].(string), Version: 1, Entries: []map[string]any{}}
			_ = json.NewEncoder(w).Encode(map[string]any{"worldbookId": id, "version": 1})
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/open/v1/worldbook/"), "/document")
		b := books[id]
		if b == nil {
			t.Error("unknown book write")
			w.WriteHeader(404)
			return
		}
		documents++
		newIDs := []string{}
		for _, entry := range doc["entries"].([]any) {
			e := entry.(map[string]any)
			rid := fmt.Sprintf("%s-entry-%d", id, len(b.Entries)+1)
			e["entryId"] = rid
			b.Entries = append(b.Entries, e)
			newIDs = append(newIDs, rid)
		}
		if binding, ok := doc["binding"].(map[string]any); ok {
			b.Key = binding["contextKey"].(string)
		}
		b.Version++
		if id == "book-2" && !lost {
			lost = true
			w.WriteHeader(503)
			fmt.Fprint(w, `{"error":"temporarily_unavailable"}`)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"createdEntryIds": newIDs, "version": b.Version})
	}))
	defer server.Close()
	f, err := card.Init(t.TempDir(), "Synthetic", "en")
	if err != nil {
		t.Fatal(err)
	}
	f.State.RoleID = "role"
	f.State.Target = "owned"
	f.Lorebooks = map[string]*card.Lorebook{"a": {ID: "a", Name: "A", Entries: []card.LorebookEntry{{Name: "A", Content: "First", Keywords: []string{}}}}, "b": {ID: "b", Name: "B", Entries: []card.LorebookEntry{{Name: "B", Content: "Second", Keywords: []string{}}}}}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	payload, err := f.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := api.New(server.URL, server.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	if err := pushNamedLorebooks(context.Background(), c, f, "role", payload.Worldbooks, PushOptions{}, &PushResult{}); err == nil {
		t.Fatal("lost reply did not stop the first push")
	}
	reloaded, err := card.Load(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	payload, err = reloaded.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := pushNamedLorebooks(context.Background(), c, reloaded, "role", payload.Worldbooks, PushOptions{}, &PushResult{}); err != nil {
		t.Fatal(err)
	}
	if created != 2 || documents != 2 || len(books["book-1"].Entries) != 1 || len(books["book-2"].Entries) != 1 {
		t.Fatalf("retry duplicated or crossed sources: created=%d documents=%d", created, documents)
	}
	if reloaded.State.Lorebooks["b"].Upload != nil || reloaded.State.Lorebooks["a"].ID == reloaded.State.Lorebooks["b"].ID {
		t.Fatal("retry did not settle independent source state")
	}
}
