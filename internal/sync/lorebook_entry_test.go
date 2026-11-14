package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

type entryTestServer struct {
	t               *testing.T
	roleID          string
	language        string
	entryContentMax int
	bindings        []map[string]any
	books           map[string]*fakeRemoteBook
	docRequests     []map[string]any
	createdBook     int
	createdEntry    int
}

func newEntryTestServer(t *testing.T) (*entryTestServer, *api.Client) {
	t.Helper()
	es := &entryTestServer{
		t:        t,
		roleID:   "r1",
		language: "en",
		books:    map[string]*fakeRemoteBook{},
	}
	srv := httptest.NewServer(http.HandlerFunc(es.serve))
	t.Cleanup(srv.Close)
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	return es, c
}

func (es *entryTestServer) serve(w http.ResponseWriter, r *http.Request) {
	reply := func(code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	switch {
	case r.URL.Path == "/open/v1/role/detail":
		reply(200, map[string]any{
			"characterRoleId": es.roleID,
			"roleName":        "Test Card",
			"language":        es.language,
			"roleDetailDesc":  "Character definition.",
			"roleWelcome":     "Hello!",
		})
	case r.URL.Path == "/open/v1/role/author-asset":
		reply(200, map[string]any{"status": "none"})
	case r.URL.Path == "/open/v1/image/list":
		http.NotFound(w, r)
	case r.URL.Path == "/open/v1/worldbook/limits":
		enMax := 12000
		if es.entryContentMax > 0 {
			enMax = es.entryContentMax
		}
		reply(200, map[string]any{
			"maxOpsPerCall":             200,
			"entryContentMaxByLanguage": map[string]int{"en": enMax, "zh-Hant": 4000},
			"trialMaxEntries":           1000,
		})
	case r.URL.Path == "/open/v1/worldbook" && r.Method == http.MethodPost:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		es.createdBook++
		bid := "b-new-" + strconv.Itoa(es.createdBook)
		name, _ := body["name"].(string)
		enMax := 12000
		if es.entryContentMax > 0 {
			enMax = es.entryContentMax
		}
		es.books[bid] = &fakeRemoteBook{
			id:       bid,
			name:     name,
			version:  1,
			entryMax: enMax,
			entries:  []map[string]any{},
		}
		reply(200, map[string]any{"worldbookId": bid, "name": name, "version": 1})
	case r.URL.Path == "/open/v1/worldbook/bindings":
		reply(200, map[string]any{"bindings": es.bindings})
	case r.URL.Path == "/open/v1/worldbook/detail":
		bid := r.URL.Query().Get("worldbookId")
		b, ok := es.books[bid]
		if !ok {
			reply(404, map[string]any{"error": "not_found"})
			return
		}
		maxChars := b.entryMax
		if maxChars == 0 {
			maxChars = 12000
		}
		reply(200, map[string]any{
			"worldbookId":     b.id,
			"name":            b.name,
			"description":     b.description,
			"tags":            b.tags,
			"version":         b.version,
			"entryCount":      len(b.entries),
			"entryContentMax": maxChars,
		})
	case r.URL.Path == "/open/v1/worldbook/entry/list":
		bid := r.URL.Query().Get("worldbookId")
		b, ok := es.books[bid]
		if !ok {
			reply(404, map[string]any{"error": "not_found"})
			return
		}
		reply(200, map[string]any{"worldbookId": bid, "entries": b.entries})
	case r.URL.Path == "/open/v1/role/"+es.roleID+"/document" || r.URL.Path == "/open/v1/role/"+es.roleID+"/welcome":
		reply(200, map[string]any{})
	case strings.HasPrefix(r.URL.Path, "/open/v1/worldbook/") && strings.HasSuffix(r.URL.Path, "/document"):
		bid := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/open/v1/worldbook/"), "/document")
		b, ok := es.books[bid]
		if !ok {
			reply(404, map[string]any{"error": "not_found"})
			return
		}
		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			es.t.Fatalf("decode document: %v", err)
		}
		raw["_bookId"] = bid
		es.docRequests = append(es.docRequests, raw)
		if ev, ok := raw["expectedVersion"].(float64); ok {
			if int32(ev) != b.version {
				reply(409, map[string]any{
					"error":          "version_conflict",
					"currentVersion": b.version,
					"detail":         map[string]any{"currentVersion": b.version, "expectedVersion": int32(ev)},
				})
				return
			}
		}
		if raw["metadata"] != nil {
			es.t.Fatalf("per-entry operation must not send metadata, got: %v", raw["metadata"])
		}
		created := []string{}
		entries, _ := raw["entries"].([]any)
		for _, item := range entries {
			op, _ := item.(map[string]any)
			switch op["op"] {
			case "create":
				es.createdEntry++
				id := "e-created-" + strconv.Itoa(es.createdEntry)
				cat, _ := op["category"].(string)
				if cat == "" {
					cat = "custom"
				}
				tr, _ := op["triggerRegion"].(string)
				if tr == "" {
					tr = "both"
				}
				enabled := true
				if v, ok := op["isEnabled"].(bool); ok {
					enabled = v
				}
				constant := false
				if v, ok := op["isConstant"].(bool); ok {
					constant = v
				}
				sr := true
				if v, ok := op["semanticRecall"].(bool); ok {
					sr = v
				}
				kw := op["keywords"]
				if kw == nil {
					kw = []string{}
				}
				row := map[string]any{
					"entryId":           id,
					"name":              op["name"],
					"content":           op["content"],
					"keywords":          kw,
					"secondaryKeywords": op["secondaryKeywords"],
					"category":          cat,
					"triggerRegion":     tr,
					"isEnabled":         enabled,
					"isConstant":        constant,
					"semanticRecall":    sr,
					"matchOptions":      op["matchOptions"],
				}
				b.entries = append(b.entries, row)
				created = append(created, id)
			case "update":
				id, _ := op["entryId"].(string)
				found := false
				for i := range b.entries {
					if b.entries[i]["entryId"] == id {
						found = true
						for _, field := range []string{"name", "content", "keywords", "secondaryKeywords", "category", "triggerRegion", "isEnabled", "isConstant", "semanticRecall", "matchOptions"} {
							if val, exists := op[field]; exists {
								b.entries[i][field] = val
							}
						}
					}
				}
				if !found {
					reply(404, map[string]any{"error": "not_found"})
					return
				}
			case "delete":
				id, _ := op["entryId"].(string)
				kept := make([]map[string]any, 0, len(b.entries))
				found := false
				for _, existing := range b.entries {
					if existing["entryId"] == id {
						found = true
						continue
					}
					kept = append(kept, existing)
				}
				if !found {
					reply(404, map[string]any{"error": "not_found"})
					return
				}
				b.entries = kept
			}
		}
		if bind, ok := raw["binding"].(map[string]any); ok {
			if rid, _ := bind["roleId"].(string); rid != "" {
				es.bindings = append(es.bindings, map[string]any{"worldbookId": bid, "name": b.name})
			}
		}
		b.version++
		reply(200, map[string]any{
			"worldbookId":     bid,
			"version":         b.version,
			"createdEntryIds": created,
		})
	default:
		es.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}
}

func TestLorebookEntryListAndGet(t *testing.T) {
	es, c := newEntryTestServer(t)
	es.books["b1"] = &fakeRemoteBook{
		id: "b1", name: "Harbor Lore", description: "Book summary", tags: "noir,harbor", version: 3,
		entries: []map[string]any{
			{"entryId": "e1", "name": "Curfew", "content": "Bells ring at dusk.", "keywords": []string{"curfew"}, "category": "rule", "isEnabled": true},
			{"entryId": "e2", "name": "Guard", "content": "East gate watch.", "keywords": []string{"guard"}, "category": "character", "isEnabled": true},
			{"entryId": "e3", "name": "Guard", "content": "West gate watch.", "keywords": []string{"west"}, "category": "character", "isEnabled": true},
		},
	}
	es.bindings = []map[string]any{{"worldbookId": "b1", "name": "Harbor Lore"}}

	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
		t.Fatalf("pull: %v", err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	listed, err := ListLorebookEntries(context.Background(), c, f)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if listed.LorebookID != "b1" || listed.Version != 3 || len(listed.Entries) != 3 {
		t.Fatalf("unexpected list result: %+v", listed)
	}

	gotByName, err := GetLorebookEntry(context.Background(), c, f, "Curfew")
	if err != nil {
		t.Fatalf("get by unique name: %v", err)
	}
	if gotByName.Entry.ID != "e1" || gotByName.Entry.Content != "Bells ring at dusk." {
		t.Fatalf("unexpected entry by name: %+v", gotByName)
	}

	gotByID, err := GetLorebookEntry(context.Background(), c, f, "e3")
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if gotByID.Entry.Content != "West gate watch." {
		t.Fatalf("unexpected entry by id: %+v", gotByID)
	}

	if _, err := GetLorebookEntry(context.Background(), c, f, "Guard"); err == nil || !strings.Contains(err.Error(), "e2") || !strings.Contains(err.Error(), "e3") {
		t.Fatalf("expected ambiguous name error naming candidate ids, got: %v", err)
	}
}

func TestLorebookEntryUpdatePreservesOtherEntriesAndMetadataAndSyncsState(t *testing.T) {
	es, c := newEntryTestServer(t)
	es.books["b1"] = &fakeRemoteBook{
		id: "b1", name: "Harbor Lore", description: "Keep this description", tags: "noir,harbor", version: 3,
		entries: []map[string]any{
			{"entryId": "e1", "name": "Curfew", "content": "Bells ring at dusk.", "keywords": []string{"curfew"}, "category": "rule", "isEnabled": true},
			{"entryId": "e2", "name": "Docks", "content": "Pier 4 stays lit.", "keywords": []string{"docks"}, "category": "location", "isEnabled": true},
		},
	}
	es.bindings = []map[string]any{{"worldbookId": "b1", "name": "Harbor Lore"}}

	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
		t.Fatalf("pull: %v", err)
	}
	// Make card sections clean too via an initial no-op push.
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatalf("baseline push: %v", err)
	}
	es.docRequests = nil

	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	newContent := "Pier 4 and Pier 7 stay lit all night."
	res, err := UpdateLorebookEntry(context.Background(), c, f, "Docks", EntryPatch{
		Content: &newContent,
	}, EntryWriteOptions{})
	if err != nil {
		t.Fatalf("update entry: %v", err)
	}
	if res.Version != 4 || res.Entry.ID != "e2" || res.Entry.Content != newContent {
		t.Fatalf("unexpected update result: %+v", res)
	}
	if len(es.docRequests) != 1 {
		t.Fatalf("expected 1 document request, got %d", len(es.docRequests))
	}
	ops, _ := es.docRequests[0]["entries"].([]any)
	if len(ops) != 1 {
		t.Fatalf("expected 1 entry op, got %v", ops)
	}
	op0, _ := ops[0].(map[string]any)
	if op0["op"] != "update" || op0["entryId"] != "e2" || op0["name"] != nil {
		t.Fatalf("expected minimal update patch on e2 only, got %v", op0)
	}
	if es.books["b1"].description != "Keep this description" || es.books["b1"].tags != "noir,harbor" {
		t.Fatalf("book metadata mutated: %+v", es.books["b1"])
	}
	if es.books["b1"].entries[0]["content"] != "Bells ring at dusk." {
		t.Fatalf("other entry e1 mutated: %v", es.books["b1"].entries[0])
	}

	// Reload folder and run full Push: worldbook must be reported unchanged with 0 Lorebook writes.
	es.docRequests = nil
	reloaded, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Lorebook.Entries[1].Content != newContent || reloaded.State.LorebookVersion != 4 {
		t.Fatalf("local lorebook/state not synced: version=%d entry=%+v", reloaded.State.LorebookVersion, reloaded.Lorebook.Entries[1])
	}
	pushRes, err := Push(context.Background(), c, reloaded, PushOptions{SkipMedia: true})
	if err != nil {
		t.Fatalf("subsequent push: %v", err)
	}
	if len(es.docRequests) != 0 {
		t.Fatalf("expected 0 Lorebook document writes on subsequent push, got %d: %v", len(es.docRequests), es.docRequests)
	}
	foundUnchanged := false
	for _, s := range pushRes.Unchanged {
		if s == card.SectionWorldbook {
			foundUnchanged = true
		}
	}
	if !foundUnchanged {
		t.Fatalf("expected worldbook in unchanged sections, got changed=%v unchanged=%v", pushRes.Changed, pushRes.Unchanged)
	}
}

func TestLorebookEntryVersionConflictAndExpectedVersionRetry(t *testing.T) {
	es, c := newEntryTestServer(t)
	es.books["b1"] = &fakeRemoteBook{
		id: "b1", name: "Harbor Lore", description: "Keep me", tags: "noir", version: 3,
		entries: []map[string]any{
			{"entryId": "e1", "name": "Curfew", "content": "Bells ring at dusk.", "keywords": []string{"curfew"}, "category": "rule", "isEnabled": true},
			{"entryId": "e2", "name": "Docks", "content": "Pier 4 stays lit.", "keywords": []string{"docks"}, "category": "location", "isEnabled": true},
		},
	}
	es.bindings = []map[string]any{{"worldbookId": "b1", "name": "Harbor Lore"}}

	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
		t.Fatalf("pull: %v", err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}

	// Simulate simultaneous web edit on e1 + web-added e3 (version 3 -> 5).
	es.books["b1"].entries[0]["content"] = "Web author changed curfew."
	es.books["b1"].entries = append(es.books["b1"].entries, map[string]any{
		"entryId": "e3", "name": "Lighthouse", "content": "Added on web.", "keywords": []string{"lighthouse"}, "category": "location", "isEnabled": true,
	})
	es.books["b1"].version = 5
	es.docRequests = nil

	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	updatedDocks := "AI updated Pier 4."
	_, err = UpdateLorebookEntry(context.Background(), c, f, "e2", EntryPatch{Content: &updatedDocks}, EntryWriteOptions{})
	if err == nil {
		t.Fatal("expected 409 version_conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "--expected-version 5") || !strings.Contains(err.Error(), "pull") {
		t.Fatalf("conflict error should guide both --expected-version 5 and pull, got: %v", err)
	}
	if es.books["b1"].entries[1]["content"] != "Pier 4 stays lit." {
		t.Fatalf("remote e2 mutated on conflict: %v", es.books["b1"].entries[1])
	}

	// Retry with explicit ExpectedVersion: 5 — updates only e2 without touching web-edited e1 or e3!
	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := UpdateLorebookEntry(context.Background(), c, f, "e2", EntryPatch{Content: &updatedDocks}, EntryWriteOptions{ExpectedVersion: 5})
	if err != nil {
		t.Fatalf("retry with expectedVersion=5: %v", err)
	}
	if res.Version != 6 {
		t.Fatalf("version = %d, want 6", res.Version)
	}
	if es.books["b1"].entries[0]["content"] != "Web author changed curfew." || len(es.books["b1"].entries) != 3 {
		t.Fatalf("concurrent web edits were harmed: %+v", es.books["b1"].entries)
	}

	// Subsequent Push must still see worldbook as unchanged and must not touch e1 or e3.
	es.docRequests = nil
	reloaded, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), c, reloaded, PushOptions{SkipMedia: true}); err != nil {
		t.Fatalf("push after concurrent per-entry update: %v", err)
	}
	if len(es.docRequests) != 0 {
		t.Fatalf("expected 0 Lorebook writes on push, got %v", es.docRequests)
	}
}

func TestLorebookEntryAddAndDeleteSyncState(t *testing.T) {
	es, c := newEntryTestServer(t)
	// Start with an owned card that has NO bound Lorebook yet.
	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
		t.Fatalf("pull: %v", err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}

	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	name1, content1 := "Harbor Bell", "Rings every hour."
	kw1 := []string{"bell", "hour"}
	addRes1, err := AddLorebookEntry(context.Background(), c, f, EntryPatch{
		Name:     &name1,
		Content:  &content1,
		Keywords: &kw1,
	}, EntryWriteOptions{})
	if err != nil {
		t.Fatalf("add first entry (creating book): %v", err)
	}
	if addRes1.LorebookID == "" || addRes1.Entry.ID == "" || len(es.bindings) != 1 {
		t.Fatalf("expected newly created and bound Lorebook, got res=%+v bindings=%v", addRes1, es.bindings)
	}

	// Add second entry to the now-bound Lorebook.
	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	name2, content2 := "Foghorn", "Sounds when mist rolls in."
	kw2 := []string{"fog", "mist"}
	addRes2, err := AddLorebookEntry(context.Background(), c, f, EntryPatch{
		Name:     &name2,
		Content:  &content2,
		Keywords: &kw2,
	}, EntryWriteOptions{})
	if err != nil {
		t.Fatalf("add second entry: %v", err)
	}
	if addRes2.Entry.ID == addRes1.Entry.ID {
		t.Fatalf("expected distinct entry id, got %s", addRes2.Entry.ID)
	}

	// Immediate Push sends 0 Lorebook writes.
	es.docRequests = nil
	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Lorebook.Entries) != 2 {
		t.Fatalf("local lorebook entries = %d, want 2", len(f.Lorebook.Entries))
	}
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatalf("push after add: %v", err)
	}
	if len(es.docRequests) != 0 {
		t.Fatalf("expected 0 Lorebook writes after add, got %v", es.docRequests)
	}

	// Delete first entry via per-entry command.
	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	delRes, err := DeleteLorebookEntry(context.Background(), c, f, addRes1.Entry.ID, EntryWriteOptions{})
	if err != nil {
		t.Fatalf("delete entry: %v", err)
	}
	if delRes.DeletedEntryID != addRes1.Entry.ID || delRes.DeletedName != "Harbor Bell" {
		t.Fatalf("unexpected delete result: %+v", delRes)
	}

	// Immediate Push (without --prune!) must report unchanged and send 0 Lorebook writes.
	es.docRequests = nil
	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Lorebook.Entries) != 1 || f.Lorebook.Entries[0].ID != addRes2.Entry.ID {
		t.Fatalf("local lorebook after delete = %+v", f.Lorebook.Entries)
	}
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatalf("push after delete: %v", err)
	}
	if len(es.docRequests) != 0 {
		t.Fatalf("expected 0 Lorebook writes after delete, got %v", es.docRequests)
	}
}

func TestLorebookEntryPreservesDirtySiblingEntryForNextPush(t *testing.T) {
	es, c := newEntryTestServer(t)
	es.books["b1"] = &fakeRemoteBook{
		id: "b1", name: "Harbor Lore", version: 2,
		entries: []map[string]any{
			{"entryId": "e1", "name": "Curfew", "content": "Bells ring at dusk.", "keywords": []string{"curfew"}, "category": "rule", "isEnabled": true},
			{"entryId": "e2", "name": "Docks", "content": "Pier 4 stays lit.", "keywords": []string{"docks"}, "category": "location", "isEnabled": true},
		},
	}
	es.bindings = []map[string]any{{"worldbookId": "b1", "name": "Harbor Lore"}}

	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
		t.Fatal(err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}

	// Edit e1 locally without pushing, then update e2 via UpdateLorebookEntry.
	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	f.Lorebook.Entries[0].Content = "Unsynced local edit to Curfew."
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}

	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	docksUpdated := "Synced per-entry edit to Docks."
	if _, err := UpdateLorebookEntry(context.Background(), c, f, "e2", EntryPatch{Content: &docksUpdated}, EntryWriteOptions{}); err != nil {
		t.Fatalf("update e2: %v", err)
	}

	// Next Push must still push dirty e1, while skipping already-synced e2!
	es.docRequests = nil
	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatalf("push dirty sibling: %v", err)
	}
	if len(es.docRequests) != 1 {
		t.Fatalf("expected 1 document request for dirty sibling e1, got %d: %v", len(es.docRequests), es.docRequests)
	}
	ops, _ := es.docRequests[0]["entries"].([]any)
	if len(ops) != 1 || ops[0].(map[string]any)["entryId"] != "e1" {
		t.Fatalf("expected only dirty e1 to be sent, got %v", ops)
	}
}

func TestLorebookEntryBoundBookMismatchAndMarkdownWarning(t *testing.T) {
	es, c := newEntryTestServer(t)
	es.books["b1"] = &fakeRemoteBook{
		id: "b1", name: "Harbor Lore", version: 2,
		entries: []map[string]any{
			{"entryId": "e1", "name": "Curfew", "content": "Bells ring at dusk.", "keywords": []string{"curfew"}, "category": "rule", "isEnabled": true},
		},
	}
	es.books["b2"] = &fakeRemoteBook{id: "b2", name: "Other Lore", version: 1}
	es.bindings = []map[string]any{{"worldbookId": "b1", "name": "Harbor Lore"}}

	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "worldbook"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worldbook", "curfew.md"), []byte("---\nname: Curfew\nkeywords: curfew\n---\nBells ring at dusk.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	newText := "Updated curfew."
	res, err := UpdateLorebookEntry(context.Background(), c, f, "e1", EntryPatch{Content: &newText}, EntryWriteOptions{})
	if err != nil {
		t.Fatalf("update with worldbook/*.md: %v", err)
	}
	if len(res.Warnings) == 0 || !strings.Contains(res.Warnings[0], "worldbook/*.md") {
		t.Fatalf("expected worldbook/*.md warning, got %v", res.Warnings)
	}

	// Switch online bound Lorebook to b2 -> per-entry command must refuse rather than mutating wrong book.
	es.bindings = []map[string]any{{"worldbookId": "b2", "name": "Other Lore"}}
	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ListLorebookEntries(context.Background(), c, f); err == nil || !strings.Contains(err.Error(), "b2") {
		t.Fatalf("expected bound-book mismatch error naming b2, got %v", err)
	}
}

func TestLorebookEntryWithoutLocalLorebookFileAndWebDeletedGuard(t *testing.T) {
	es, c := newEntryTestServer(t)
	es.books["b1"] = &fakeRemoteBook{
		id: "b1", name: "Harbor Lore", description: "Desc", version: 2,
		entries: []map[string]any{
			{"entryId": "e1", "name": "Curfew", "content": "Bells ring at dusk.", "keywords": []string{"curfew"}, "category": "rule", "isEnabled": true},
			{"entryId": "e2", "name": "Docks", "content": "Pier 4 stays lit.", "keywords": []string{"docks"}, "category": "location", "isEnabled": true},
		},
	}
	es.bindings = []map[string]any{{"worldbookId": "b1", "name": "Harbor Lore"}}

	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
		t.Fatal(err)
	}
	// Remove local lorebook.json to simulate a folder that has not pulled its Lorebook file yet.
	if err := os.Remove(filepath.Join(dir, card.LorebookFile)); err != nil {
		t.Fatal(err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	f.State.LorebookEntryHashes = nil
	f.State.LorebookEntryIDs = nil
	if err := f.SaveState(); err != nil {
		t.Fatal(err)
	}

	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	newDocks := "Pier 4 updated without prior local lorebook.json."
	if _, err := UpdateLorebookEntry(context.Background(), c, f, "e2", EntryPatch{Content: &newDocks}, EntryWriteOptions{ExpectedVersion: 2}); err != nil {
		t.Fatalf("update without local lorebook.json: %v", err)
	}
	reloaded, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Lorebook == nil || len(reloaded.Lorebook.Entries) != 2 {
		t.Fatalf("expected full remote entries populated into lorebook.json, got %+v", reloaded.Lorebook)
	}

	// Now simulate web deleting e1 (version 3 -> 4), AI concurrently updating e2 with --expected-version 4 (version 4 -> 5),
	// and then someone editing e2 locally and running full card push: e1 must NOT be silently resurrected!
	es.books["b1"].entries = es.books["b1"].entries[1:]
	es.books["b1"].version = 4
	nextDocks := "Pier 4 updated after web deleted e1."
	if _, err := UpdateLorebookEntry(context.Background(), c, reloaded, "e2", EntryPatch{Content: &nextDocks}, EntryWriteOptions{ExpectedVersion: 4}); err != nil {
		t.Fatalf("concurrent update after web delete: %v", err)
	}

	afterConcurrent, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	afterConcurrent.Lorebook.Entries[1].Content = "Local edit triggering pushLorebook."
	if err := afterConcurrent.SaveLorebook(); err != nil {
		t.Fatal(err)
	}
	afterConcurrent, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), c, afterConcurrent, PushOptions{SkipMedia: true}); err == nil {
		t.Fatalf("expected push to block instead of silently re-creating web-deleted e1, got nil")
	}
	if len(es.books["b1"].entries) != 1 {
		t.Fatalf("web-deleted e1 was resurrected: %+v", es.books["b1"].entries)
	}
}

func TestLorebookEntryPreservesStateTrackedSiblingIDsAndRelativeAssets(t *testing.T) {
	es, c := newEntryTestServer(t)
	es.entryContentMax = 60

	dir := t.TempDir()
	f := &card.Folder{
		Dir: dir,
		Manifest: card.Manifest{
			FormatVersion: card.FormatVersion,
			Name:          "Harbor Watch",
			Summary:       " Summary",
			Type:          "single",
			Language:      "en",
		},
		Lorebook: &card.Lorebook{
			Name: "Harbor Lore",
			Entries: []card.LorebookEntry{
				{
					Name:     "Long Chronicle",
					Content:  "First paragraph of the harbor chronicle.\n\nSecond paragraph of the harbor chronicle.",
					Keywords: []string{"chronicle"},
					Category: "event",
				},
				{
					Name:     "Harbor Map",
					Content:  "Chart reference: assets/map.png",
					Keywords: []string{"map"},
					Category: "item",
				},
			},
		},
		State: card.State{
			RoleID: "r1",
			Target: "owned",
			Assets: map[string]card.Asset{
				"assets/map.png": {URL: "https://cdn.example.test/assets/map.png"},
			},
		},
	}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatalf("initial push of locally authored lorebook: %v", err)
	}

	// Verify locally authored entries still have empty IDs in lorebook.json and split fragments in state.
	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.Lorebook.Entries[0].ID != "" || f.Lorebook.Entries[1].ID != "" {
		t.Fatalf("expected empty source IDs before per-entry edit, got %+v", f.Lorebook.Entries)
	}
	if f.State.LorebookEntryMap["0:0"] == "" || f.State.LorebookEntryMap["0:1"] == "" {
		t.Fatalf("expected split fragments in LorebookEntryMap, got %v", f.State.LorebookEntryMap)
	}

	// Metadata-only update on "Harbor Map" must keep relative assets/map.png AND preserve sibling split map + source aliases.
	newKW := []string{"map", "chart"}
	if _, err := UpdateLorebookEntry(context.Background(), c, f, "Harbor Map", EntryPatch{Keywords: &newKW}, EntryWriteOptions{}); err != nil {
		t.Fatalf("metadata-only update on Harbor Map: %v", err)
	}

	reloaded, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Lorebook.Entries[1].Content != "Chart reference: assets/map.png" {
		t.Fatalf("relative asset path was overwritten with CDN URL: %q", reloaded.Lorebook.Entries[1].Content)
	}
	if reloaded.State.LorebookEntryMap["0:0"] == "" || reloaded.State.LorebookEntryMap["0:1"] == "" {
		t.Fatalf("sibling split fragments wiped from LorebookEntryMap: %v", reloaded.State.LorebookEntryMap)
	}

	// Renaming untouched split sibling locally and pushing WITHOUT --prune must update in place instead of orphaning parts.
	reloaded.Lorebook.Entries[0].Name = "Renamed Chronicle"
	if err := reloaded.SaveLorebook(); err != nil {
		t.Fatal(err)
	}
	reloaded, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), c, reloaded, PushOptions{SkipMedia: true}); err != nil {
		t.Fatalf("push after renaming untouched split sibling failed (likely lost LorebookEntryMap/LorebookSourceIDs): %v", err)
	}
}

func TestLorebookEntryExpectedVersionDoesNotMaskUnpulledWebSiblingEdits(t *testing.T) {
	es, c := newEntryTestServer(t)
	es.books["b1"] = &fakeRemoteBook{
		id: "b1", name: "Harbor Lore", version: 3,
		entries: []map[string]any{
			{"entryId": "e1", "name": "Curfew", "content": "Original curfew v3.", "keywords": []string{"curfew"}, "category": "rule", "isEnabled": true},
			{"entryId": "e2", "name": "Docks", "content": "Original docks v3.", "keywords": []string{"docks"}, "category": "location", "isEnabled": true},
		},
	}
	es.bindings = []map[string]any{{"worldbookId": "b1", "name": "Harbor Lore"}}

	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
		t.Fatal(err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}

	// Web edits e1 (version 3 -> 4).
	es.books["b1"].entries[0]["content"] = "Web curfew v4."
	es.books["b1"].version = 4

	// AI updates e2 with ExpectedVersion: 4 -> remote becomes version 5, while local e1 is still stale v3.
	f, _ = card.Load(dir)
	docksV5 := "AI docks v5."
	if _, err := UpdateLorebookEntry(context.Background(), c, f, "e2", EntryPatch{Content: &docksV5}, EntryWriteOptions{ExpectedVersion: 4}); err != nil {
		t.Fatalf("update e2 with expectedVersion=4: %v", err)
	}

	// 1. Subsequent UpdateLorebookEntry on e1 WITHOUT ExpectedVersion must 409 rather than silently overwriting Web's v4 edit.
	f, _ = card.Load(dir)
	staleOverwrite := "Stale agent overwrite of curfew."
	if _, err := UpdateLorebookEntry(context.Background(), c, f, "e1", EntryPatch{Content: &staleOverwrite}, EntryWriteOptions{}); err == nil {
		t.Fatal("expected 409 when updating unpulled web-edited sibling e1 without --expected-version, got nil")
	}
	if es.books["b1"].entries[0]["content"] != "Web curfew v4." {
		t.Fatalf("web edit on e1 was overwritten: %v", es.books["b1"].entries[0])
	}

	// 2. Subsequent local edit on e1 + full Push must ALSO 409 rather than silently overwriting Web's v4 edit.
	f, _ = card.Load(dir)
	f.Lorebook.Entries[0].Content = staleOverwrite
	if err := f.SaveLorebook(); err != nil {
		t.Fatal(err)
	}
	f, _ = card.Load(dir)
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err == nil {
		t.Fatal("expected 409 on Push when local e1 was edited from stale v3 baseline after e2 --expected-version write")
	}
	if es.books["b1"].entries[0]["content"] != "Web curfew v4." {
		t.Fatalf("web edit on e1 was overwritten by Push: %v", es.books["b1"].entries[0])
	}
}


