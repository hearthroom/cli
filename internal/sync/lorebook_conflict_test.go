package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

type fakeLorebookServer struct {
	t           *testing.T
	roleID      string
	language    string
	bindings    []map[string]any
	books       map[string]*fakeRemoteBook
	docRequests []map[string]any
	roleWrites  int
}

type fakeRemoteBook struct {
	id          string
	name        string
	description string
	tags        string
	version     int32
	entryMax    int
	entries     []map[string]any
}

func newFakeLorebookServer(t *testing.T) (*fakeLorebookServer, *api.Client) {
	t.Helper()
	fs := &fakeLorebookServer{
		t:        t,
		roleID:   "r1",
		language: "en",
		books:    map[string]*fakeRemoteBook{},
	}
	srv := httptest.NewServer(http.HandlerFunc(fs.serve))
	t.Cleanup(srv.Close)
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	return fs, c
}

func (fs *fakeLorebookServer) serve(w http.ResponseWriter, r *http.Request) {
	reply := func(code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	switch {
	case r.URL.Path == "/open/v1/role/detail":
		reply(200, map[string]any{
			"characterRoleId": fs.roleID,
			"roleName":        "Test Card",
			"language":        fs.language,
			"roleDetailDesc":  "Character definition.",
			"roleWelcome":     "Hello!",
		})
	case r.URL.Path == "/open/v1/role/author-asset":
		reply(200, map[string]any{"status": "none"})
	case r.URL.Path == "/open/v1/image/list":
		http.NotFound(w, r)
	case r.URL.Path == "/open/v1/worldbook/limits":
		reply(200, map[string]any{
			"maxOpsPerCall":             2,
			"entryContentMaxByLanguage": map[string]int{"en": 12000, "zh-Hant": 4000},
			"trialMaxEntries":           1000,
		})
	case r.URL.Path == "/open/v1/worldbook/bindings":
		reply(200, map[string]any{"bindings": fs.bindings})
	case r.URL.Path == "/open/v1/worldbook/detail":
		bid := r.URL.Query().Get("worldbookId")
		b, ok := fs.books[bid]
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
		b, ok := fs.books[bid]
		if !ok {
			reply(404, map[string]any{"error": "not_found"})
			return
		}
		reply(200, map[string]any{"entries": b.entries})
	case r.URL.Path == "/open/v1/role/"+fs.roleID+"/document" || r.URL.Path == "/open/v1/role/"+fs.roleID+"/welcome":
		fs.roleWrites++
		reply(200, map[string]any{})
	case strings.HasPrefix(r.URL.Path, "/open/v1/worldbook/") && strings.HasSuffix(r.URL.Path, "/document"):
		bid := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/open/v1/worldbook/"), "/document")
		b, ok := fs.books[bid]
		if !ok {
			reply(404, map[string]any{"error": "not_found"})
			return
		}
		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			fs.t.Fatalf("decode document: %v", err)
		}
		raw["_bookId"] = bid
		fs.docRequests = append(fs.docRequests, raw)
		if raw["metadata"] == nil && raw["binding"] == nil {
			entries, _ := raw["entries"].([]any)
			if len(entries) == 0 {
				reply(400, map[string]any{"error": "nothing_to_update"})
				return
			}
		}
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
		if meta, ok := raw["metadata"].(map[string]any); ok {
			if n, ok := meta["name"].(string); ok {
				b.name = n
			}
			if d, exists := meta["description"]; exists {
				s, _ := d.(string)
				b.description = s
			}
			if tg, exists := meta["tags"]; exists {
				arr, _ := tg.([]any)
				parts := make([]string, 0, len(arr))
				for _, item := range arr {
					if s, ok := item.(string); ok {
						parts = append(parts, s)
					}
				}
				b.tags = strings.Join(parts, ",")
			}
		}
		created := []string{}
		entries, _ := raw["entries"].([]any)
		for _, item := range entries {
			op, _ := item.(map[string]any)
			switch op["op"] {
			case "create":
				id := "created-" + string(rune('a'+len(b.entries)))
				row := map[string]any{
					"entryId":   id,
					"name":      op["name"],
					"content":   op["content"],
					"keywords":  op["keywords"],
					"isEnabled": op["isEnabled"],
				}
				b.entries = append(b.entries, row)
				created = append(created, id)
			case "update":
				id, _ := op["entryId"].(string)
				for i := range b.entries {
					if b.entries[i]["entryId"] == id {
						if op["name"] != nil {
							b.entries[i]["name"] = op["name"]
						}
						if op["content"] != nil {
							b.entries[i]["content"] = op["content"]
						}
					}
				}
			case "delete":
				id, _ := op["entryId"].(string)
				kept := b.entries[:0]
				for _, existing := range b.entries {
					if existing["entryId"] != id {
						kept = append(kept, existing)
					}
				}
				b.entries = kept
			}
		}
		b.version++
		reply(200, map[string]any{
			"worldbookId":     bid,
			"version":         b.version,
			"createdEntryIds": created,
		})
	default:
		fs.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}
}

func TestSimultaneousSameEntryConflictAndForceOverwrite(t *testing.T) {
	fs, c := newFakeLorebookServer(t)
	fs.books["b1"] = &fakeRemoteBook{
		id: "b1", name: "Harbor Lore", description: "Keep me", tags: "noir,harbor", version: 3,
		entries: []map[string]any{
			{"entryId": "e1", "name": "Curfew", "content": "Bells ring at dusk.", "keywords": []string{"curfew"}, "isEnabled": true},
			{"entryId": "e2", "name": "Docks", "content": "Pier 4 stays lit.", "keywords": []string{"docks"}, "isEnabled": true},
		},
	}
	fs.bindings = []map[string]any{{"worldbookId": "b1", "name": "Harbor Lore"}}

	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
		t.Fatalf("pull: %v", err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if f.State.LorebookVersion != 3 {
		t.Fatalf("pulled LorebookVersion = %d, want 3", f.State.LorebookVersion)
	}

	// Author edits e1 on web (version 3 -> 4), while AI edits e1 locally.
	fs.books["b1"].entries[0]["content"] = "Web edited curfew."
	fs.books["b1"].version = 4
	f.Lorebook.Entries[0].Content = "Local AI edited curfew."
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}

	_, err = Push(context.Background(), c, f, PushOptions{SkipMedia: true})
	if err == nil {
		t.Fatal("expected 409 version_conflict error on stale push, got nil")
	}
	if !strings.Contains(err.Error(), "pull") {
		t.Fatalf("conflict error should instruct author to pull, got: %v", err)
	}
	if got := fs.books["b1"].entries[0]["content"]; got != "Web edited curfew." {
		t.Fatalf("conflict push mutated remote e1 = %v", got)
	}

	// Reload and push with --force: overwrites and emits explicit warning.
	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true, Force: true})
	if err != nil {
		t.Fatalf("force push: %v", err)
	}
	if got := fs.books["b1"].entries[0]["content"]; got != "Local AI edited curfew." {
		t.Fatalf("force push remote e1 = %v, want Local AI edited curfew.", got)
	}
	if f.State.LorebookVersion != 5 {
		t.Fatalf("after force push LorebookVersion = %d, want 5", f.State.LorebookVersion)
	}
	warned := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "--force") && strings.Contains(strings.ToLower(w), "lorebook") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("expected explicit --force overwrite warning in PushResult.Warnings, got: %v", res.Warnings)
	}
}

func TestSimultaneousDifferentEntriesOnlySendsChangedEntry(t *testing.T) {
	fs, c := newFakeLorebookServer(t)
	fs.books["b1"] = &fakeRemoteBook{
		id: "b1", name: "Harbor Lore", version: 2,
		entries: []map[string]any{
			{"entryId": "e1", "name": "Curfew", "content": "Original e1", "keywords": []string{"curfew"}, "isEnabled": true},
			{"entryId": "e2", "name": "Docks", "content": "Original e2", "keywords": []string{"docks"}, "isEnabled": true},
		},
	}
	fs.bindings = []map[string]any{{"worldbookId": "b1", "name": "Harbor Lore"}}

	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
		t.Fatalf("pull: %v", err)
	}

	// Web edits e1 (version 2 -> 3); local edits ONLY e2.
	fs.books["b1"].entries[0]["content"] = "Web updated e1"
	fs.books["b1"].version = 3

	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	f.Lorebook.Entries[1].Content = "Local updated e2"
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}

	// Normal versioned push stops on 409 AND its planned batch only contained e2 (not untouched e1).
	fs.docRequests = nil
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err == nil {
		t.Fatal("expected conflict error when remote version advanced")
	}
	if len(fs.docRequests) != 1 {
		t.Fatalf("docRequests = %d, want 1", len(fs.docRequests))
	}
	sentEntries, _ := fs.docRequests[0]["entries"].([]any)
	if len(sentEntries) != 1 || sentEntries[0].(map[string]any)["entryId"] != "e2" {
		t.Fatalf("expected ONLY modified entry e2 to be sent, got: %v", sentEntries)
	}

	// Legacy state without LorebookVersion (0) also preserves Web's e1 because unchanged e1 is not sent.
	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	f.State.LorebookVersion = 0
	if err := f.SaveState(); err != nil {
		t.Fatal(err)
	}
	fs.docRequests = nil
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatalf("unversioned push of e2: %v", err)
	}
	if got := fs.books["b1"].entries[0]["content"]; got != "Web updated e1" {
		t.Fatalf("unchanged local e1 overwrote web edit: got %v", got)
	}
	if got := fs.books["b1"].entries[1]["content"]; got != "Local updated e2" {
		t.Fatalf("modified local e2 not updated: got %v", got)
	}
}

func TestSimultaneousOnlineAddAndDeleteEntry(t *testing.T) {
	t.Run("online_add_caught_by_version_and_kept_after_merge", func(t *testing.T) {
		fs, c := newFakeLorebookServer(t)
		fs.books["b1"] = &fakeRemoteBook{
			id: "b1", name: "Harbor Lore", version: 1,
			entries: []map[string]any{
				{"entryId": "e1", "name": "Curfew", "content": "Original e1", "keywords": []string{"curfew"}, "isEnabled": true},
			},
		}
		fs.bindings = []map[string]any{{"worldbookId": "b1", "name": "Harbor Lore"}}

		dir := t.TempDir()
		if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
			t.Fatal(err)
		}
		// Web adds e_web (version 1 -> 2); local edits e1.
		fs.books["b1"].entries = append(fs.books["b1"].entries, map[string]any{
			"entryId": "e_web", "name": "Web New", "content": "Added on web", "keywords": []string{"web"}, "isEnabled": true,
		})
		fs.books["b1"].version = 2

		f, _ := card.Load(dir)
		f.Lorebook.Entries[0].Content = "Local e1 v2"
		_ = f.Save()

		if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err == nil {
			t.Fatal("expected conflict when web added entry")
		}
		if len(fs.books["b1"].entries) != 2 {
			t.Fatalf("remote entries = %d, want 2", len(fs.books["b1"].entries))
		}
	})

	t.Run("online_delete_never_silently_resurrected", func(t *testing.T) {
		fs, c := newFakeLorebookServer(t)
		fs.books["b1"] = &fakeRemoteBook{
			id: "b1", name: "Harbor Lore", version: 1,
			entries: []map[string]any{
				{"entryId": "e1", "name": "Curfew", "content": "Original e1", "keywords": []string{"curfew"}, "isEnabled": true},
				{"entryId": "e_del", "name": "Obsolete", "content": "Delete me on web", "keywords": []string{"old"}, "isEnabled": true},
			},
		}
		fs.bindings = []map[string]any{{"worldbookId": "b1", "name": "Harbor Lore"}}

		dir := t.TempDir()
		if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
			t.Fatal(err)
		}
		// Web deletes e_del (version 1 -> 2); local edits e1 (even if LorebookVersion were 0, must not resurrect e_del).
		fs.books["b1"].entries = fs.books["b1"].entries[:1]
		fs.books["b1"].version = 2

		f, _ := card.Load(dir)
		f.State.LorebookVersion = 0
		f.Lorebook.Entries[0].Content = "Local e1 edit"
		_ = f.Save()

		if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err == nil {
			t.Fatal("expected push to refuse silently re-creating online-deleted entry e_del")
		}
		if len(fs.books["b1"].entries) != 1 {
			t.Fatalf("online-deleted entry was resurrected: %v", fs.books["b1"].entries)
		}
	})
}

func TestBoundLorebookLatestResolutionAndMismatchGuard(t *testing.T) {
	fs, c := newFakeLorebookServer(t)
	fs.books["b_old"] = &fakeRemoteBook{
		id: "b_old", name: "Old Book", version: 1,
		entries: []map[string]any{{"entryId": "eo", "name": "Old", "content": "Old content", "keywords": []string{}, "isEnabled": true}},
	}
	fs.books["b_latest"] = &fakeRemoteBook{
		id: "b_latest", name: "Latest Book", version: 4,
		entries: []map[string]any{{"entryId": "el", "name": "Latest", "content": "Latest content", "keywords": []string{}, "isEnabled": true}},
	}
	fs.bindings = []map[string]any{
		{"worldbookId": "b_old", "name": "Old Book"},
		{"worldbookId": "b_latest", "name": "Latest Book"},
	}

	dir := t.TempDir()
	pullRes, err := Pull(context.Background(), c, "r1", dir, PullOptions{})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if pullRes.Lorebook != "b_latest" {
		t.Fatalf("Pull picked Lorebook %q, want b_latest (matching web editor last bound book)", pullRes.Lorebook)
	}

	// Now simulate web switching bound book to b_switched while local state still points to b_latest.
	fs.books["b_switched"] = &fakeRemoteBook{
		id: "b_switched", name: "Switched Book", version: 1,
		entries: []map[string]any{},
	}
	fs.bindings = []map[string]any{{"worldbookId": "b_switched", "name": "Switched Book"}}

	// Non-Lorebook edits (e.g. welcome.md only) do not write to the Lorebook and succeed cleanly.
	f, _ := card.Load(dir)
	f.Welcome = "Updated opening only"
	_ = f.Save()
	fs.docRequests = nil
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatalf("welcome-only push failed unexpectedly when Lorebook was unchanged: %v", err)
	}
	if len(fs.docRequests) != 0 {
		t.Fatalf("welcome-only push must not write Lorebook, got %d requests", len(fs.docRequests))
	}

	f, _ = card.Load(dir)
	f.Lorebook.Entries[0].Content = "Edited locally"
	_ = f.Save()

	fs.docRequests = nil
	fs.roleWrites = 0
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err == nil {
		t.Fatal("expected push to stop when state.lorebookId != online bound book")
	}
	if fs.roleWrites != 0 || len(fs.docRequests) != 0 {
		t.Fatalf("mismatch guard must stop before mutating card (%d) or Lorebook (%d)", fs.roleWrites, len(fs.docRequests))
	}

	// Even with --force, CLI must target b_switched and NEVER re-bind b_latest.
	f, _ = card.Load(dir)
	forceRes, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true, Force: true})
	if err != nil {
		t.Fatalf("force push after rebind: %v", err)
	}
	for _, req := range fs.docRequests {
		if req["_bookId"] == "b_latest" {
			t.Fatalf("--force re-bound stale Lorebook b_latest instead of using b_switched")
		}
	}
	if f.State.LorebookID != "b_switched" {
		t.Fatalf("state.LorebookID = %q, want b_switched", f.State.LorebookID)
	}
	if len(forceRes.Warnings) == 0 {
		t.Fatalf("expected warning on --force after bound book change")
	}
}

func TestPullWarnsWhenWorldbookMarkdownSourcesExist(t *testing.T) {
	fs, c := newFakeLorebookServer(t)
	fs.books["b1"] = &fakeRemoteBook{
		id: "b1", name: "Harbor Lore", version: 2,
		entries: []map[string]any{
			{"entryId": "e1", "name": "Curfew", "content": "Pulled text", "keywords": []string{"curfew"}, "isEnabled": true},
		},
	}
	fs.bindings = []map[string]any{{"worldbookId": "b1", "name": "Harbor Lore"}}

	dir := t.TempDir()
	if _, err := card.Init(dir, "Test Card", "en"); err != nil {
		t.Fatal(err)
	}
	wbDir := filepath.Join(dir, "worldbook")
	if err := os.MkdirAll(wbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wbDir, "01-curfew.md"), []byte("---\nname: Curfew\n---\nLocal md text\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Pull(context.Background(), c, "r1", dir, PullOptions{Force: true})
	if err != nil {
		t.Fatalf("pull --force: %v", err)
	}
	foundWarning := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "worldbook/") && strings.Contains(w, "lorebook.json") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("expected worldbook/*.md sync warning in PullResult.Warnings, got: %v", res.Warnings)
	}
}

func TestMetadataOmittedWhenNameUnchangedAndPreservesDescriptionTags(t *testing.T) {
	fs, c := newFakeLorebookServer(t)
	fs.books["b1"] = &fakeRemoteBook{
		id: "b1", name: "Harbor Lore", description: "Author book description", tags: "mystery,harbor", version: 1,
		entries: []map[string]any{
			{"entryId": "e1", "name": "One", "content": "c1", "keywords": []string{"k1"}, "isEnabled": true},
			{"entryId": "e2", "name": "Two", "content": "c2", "keywords": []string{"k2"}, "isEnabled": true},
			{"entryId": "e3", "name": "Three", "content": "c3", "keywords": []string{"k3"}, "isEnabled": true},
		},
	}
	fs.bindings = []map[string]any{{"worldbookId": "b1", "name": "Harbor Lore"}}

	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
		t.Fatal(err)
	}
	f, _ := card.Load(dir)
	f.Lorebook.Entries[0].Content = "c1-edited"
	_ = f.Save()

	fs.docRequests = nil
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatalf("push unchanged book name: %v", err)
	}
	if len(fs.docRequests) != 1 {
		t.Fatalf("docRequests = %d, want 1", len(fs.docRequests))
	}
	if _, hasMeta := fs.docRequests[0]["metadata"]; hasMeta {
		t.Fatalf("metadata MUST be omitted when book name did not change, got: %v", fs.docRequests[0]["metadata"])
	}

	// Now rename book AND edit 3 entries (2 batches with maxOpsPerCall=2): metadata sent ONLY on batch 1, without description/tags.
	f, _ = card.Load(dir)
	f.Lorebook.Name = "Harbor Lore Renamed"
	f.Lorebook.Entries[0].Content = "c1-v3"
	f.Lorebook.Entries[1].Content = "c2-v3"
	f.Lorebook.Entries[2].Content = "c3-v3"
	_ = f.Save()

	fs.docRequests = nil
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatalf("push renamed book: %v", err)
	}
	if len(fs.docRequests) != 2 {
		t.Fatalf("docRequests = %d, want 2 batches", len(fs.docRequests))
	}
	meta0, ok := fs.docRequests[0]["metadata"].(map[string]any)
	if !ok || meta0["name"] != "Harbor Lore Renamed" {
		t.Fatalf("batch 1 metadata = %v, want name=Harbor Lore Renamed", fs.docRequests[0]["metadata"])
	}
	if _, hasDesc := meta0["description"]; hasDesc {
		t.Fatalf("batch 1 metadata must not send empty description: %v", meta0)
	}
	if _, hasTags := meta0["tags"]; hasTags {
		t.Fatalf("batch 1 metadata must not send empty tags: %v", meta0)
	}
	if _, hasMeta2 := fs.docRequests[1]["metadata"]; hasMeta2 {
		t.Fatalf("batch 2 must omit metadata, got: %v", fs.docRequests[1]["metadata"])
	}
	if fs.books["b1"].description != "Author book description" || fs.books["b1"].tags != "mystery,harbor" {
		t.Fatalf("book description/tags wiped: desc=%q tags=%q", fs.books["b1"].description, fs.books["b1"].tags)
	}
}

func TestPersistedLorebookUploadStillGuardsBoundBookChange(t *testing.T) {
	fs, c := newFakeLorebookServer(t)
	fs.books["b1"] = &fakeRemoteBook{
		id: "b1", name: "Book 1", version: 2,
		entries: []map[string]any{
			{"entryId": "e1", "name": "Entry 1", "content": "c1", "keywords": []string{"k1"}, "isEnabled": true},
			{"entryId": "e2", "name": "Entry 2", "content": "c2", "keywords": []string{"k2"}, "isEnabled": true},
		},
	}
	fs.bindings = []map[string]any{{"worldbookId": "b1", "name": "Book 1"}}

	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
		t.Fatalf("pull: %v", err)
	}

	// Remove e2 locally and run push without --prune -> stops on prune refusal.
	f, _ := card.Load(dir)
	f.Lorebook.Entries = f.Lorebook.Entries[:1]
	_ = f.Save()
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err == nil {
		t.Fatal("expected missing --prune stop")
	}

	// Also simulate an already-persisted legacy LorebookUpload checkpoint in state.json,
	// while the author switches the card's bound Lorebook on the web to b2.
	f, _ = card.Load(dir)
	f.State.LorebookUpload = &card.LorebookUpload{
		Digest:    "legacy-checkpoint",
		BatchSize: 2,
		Ops:       []map[string]any{{"op": "delete", "entryId": "e2"}},
	}
	_ = f.SaveState()

	fs.books["b2"] = &fakeRemoteBook{id: "b2", name: "Book 2", version: 1, entries: []map[string]any{}}
	fs.bindings = []map[string]any{{"worldbookId": "b2", "name": "Book 2"}}

	f, _ = card.Load(dir)
	fs.docRequests = nil
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true, Prune: true}); err == nil {
		t.Fatal("expected bound-book mismatch error even when LorebookUpload checkpoint is present")
	}
	if len(fs.docRequests) != 0 {
		t.Fatalf("must not write or re-bind detached Lorebook b1, got %v", fs.docRequests)
	}
}

func TestReconcileLostReplyWithLocalEditUpdatesBaseVersion(t *testing.T) {
	fs, c := newFakeLorebookServer(t)
	fs.books["b1"] = &fakeRemoteBook{
		id: "b1", name: "Book 1", version: 3,
		entries: []map[string]any{
			{"entryId": "e1", "name": "Entry 1", "content": "c1-committed", "keywords": []string{"k1"}, "isEnabled": true},
		},
	}
	fs.bindings = []map[string]any{{"worldbookId": "b1", "name": "Book 1"}}

	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
		t.Fatalf("pull: %v", err)
	}

	// Simulate a lost HTTP reply: server already committed batch 0 creating e1 at version 3,
	// while local state still had LorebookVersion=2 and an InFlight batch-0 create.
	f, _ := card.Load(dir)
	f.State.LorebookVersion = 2
	f.State.LorebookUpload = &card.LorebookUpload{
		Digest:      "previous-digest",
		BatchSize:   2,
		BaseVersion: 2,
		InFlight:    true,
		Keys:        []string{"0:0"},
		Ops:         []map[string]any{{"op": "create", "name": "Entry 1", "content": "c1-committed"}},
		BeforeIDs:   []string{},
	}
	// User then edits Entry 1 locally before rerunning push -> Digest != upload.Digest.
	f.Lorebook.Entries[0].Content = "c1-edited-after-lost-reply"
	_ = f.Save()

	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatalf("expected reconciled batch to advance base version to 3 instead of self-conflicting: %v", err)
	}
	if fs.books["b1"].version != 4 {
		t.Fatalf("remote version = %d, want 4", fs.books["b1"].version)
	}
}
