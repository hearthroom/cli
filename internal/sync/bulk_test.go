package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

func TestBulkPushResumesAfterFailure(t *testing.T) {
	f, err := card.Init(t.TempDir(), "Synthetic large card", "zh-Hant")
	if err != nil {
		t.Fatal(err)
	}
	f.Welcome = "An opening."
	f.Lorebook = &card.Lorebook{Name: "Synthetic"}
	for i := 0; i < 2501; i++ {
		f.Lorebook.Entries = append(f.Lorebook.Entries, card.LorebookEntry{Name: fmt.Sprintf("Entry %d", i/2), Content: fmt.Sprintf("Synthetic content %d", i)})
	}
	original := strings.Repeat("段落。\n\n", 2200) + "尾端\r\n"
	f.Lorebook.Entries[0].Content = original
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	roles, books, batches := 0, 0, 0
	failed := false
	remote := []map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := func(v any) { json.NewEncoder(w).Encode(v) }
		switch r.URL.Path {
		case "/open/v1/worldbook/limits":
			reply(map[string]any{"maxOpsPerCall": 80, "entryContentMaxByLanguage": map[string]int{"zh-Hant": 4000}, "trialMaxEntries": 1000})
		case "/open/v1/role":
			roles++
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			if b["visibility"] != "private" {
				t.Errorf("create visibility=%v", b["visibility"])
			}
			reply(map[string]any{"roleId": "r"})
		case "/open/v1/role/detail":
			reply(map[string]any{"language": "zh-Hant"})
		case "/open/v1/role/r/document", "/open/v1/role/r/welcome":
			reply(map[string]any{})
		case "/open/v1/worldbook/bindings":
			reply(map[string]any{"bindings": []any{}})
		case "/open/v1/worldbook":
			books++
			reply(map[string]any{"worldbookId": "b"})
		case "/open/v1/worldbook/detail":
			reply(map[string]any{"entryContentMax": 4000})
		case "/open/v1/worldbook/entry/list":
			reply(map[string]any{"entries": remote})
		case "/open/v1/worldbook/b/document":
			var b struct{ Entries []map[string]any }
			json.NewDecoder(r.Body).Decode(&b)
			if len(b.Entries) > 80 {
				t.Errorf("batch has %d operations", len(b.Entries))
				w.WriteHeader(400)
				return
			}
			if len(b.Entries) > 0 {
				batches++
			}
			if batches == 2 && !failed {
				failed = true
				w.WriteHeader(503)
				reply(map[string]any{"error": "temporarily_unavailable"})
				return
			}
			ids := []string{}
			for _, op := range b.Entries {
				if op["op"] != "create" {
					t.Errorf("unexpected operation %v", op["op"])
					continue
				}
				id := fmt.Sprintf("e%d", len(remote))
				op["entryId"] = id
				op["isEnabled"] = true
				remote = append(remote, op)
				ids = append(ids, id)
			}
			reply(map[string]any{"createdEntryIds": ids})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "test", nil })
	opts := PushOptions{Create: true, SkipMedia: true}
	if _, err := Push(context.Background(), c, f, opts); err == nil {
		t.Fatal("want injected failure")
	}
	f, err = card.Load(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.State.RoleID != "r" || f.State.LorebookID != "b" || len(f.State.LorebookEntryIDs) != 80 {
		t.Fatalf("checkpoint role=%q book=%q entries=%d", f.State.RoleID, f.State.LorebookID, len(f.State.LorebookEntryIDs))
	}
	res, err := Push(context.Background(), c, f, opts)
	if err != nil {
		t.Fatal(err)
	}
	if roles != 1 || books != 1 {
		t.Fatalf("created roles=%d books=%d", roles, books)
	}
	if len(remote) <= 2501 {
		t.Fatalf("expected grouped long entry, got %d", len(remote))
	}
	var restored strings.Builder
	group := ""
	order := 0
	for _, e := range remote {
		content := e["content"].(string)
		if utf8.RuneCountInString(content) > 4000 {
			t.Fatal("oversized fragment")
		}
		if mo, ok := e["matchOptions"].(map[string]any); ok && mo["groupId"] != nil {
			if group == "" {
				group = mo["groupId"].(string)
			}
			if mo["groupId"] != group || int(mo["groupOrder"].(float64)) != order {
				t.Fatal("invalid group sequence")
			}
			order++
			restored.WriteString(content)
		}
	}
	if restored.String() != original {
		t.Fatal("grouping lost original content")
	}
	before := batches
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	if batches != before {
		t.Fatal("successful push still looks changed")
	}
	if res.RoleID != "r" {
		t.Fatal("missing card result")
	}
}

func TestTrialLargeCardPreflightBeforeWrites(t *testing.T) {
	f, _ := card.Init(t.TempDir(), "Synthetic", "en")
	f.Lorebook = &card.Lorebook{}
	for i := 0; i < 1001; i++ {
		f.Lorebook.Entries = append(f.Lorebook.Entries, card.LorebookEntry{Name: "Synthetic", Content: "text"})
	}
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			writes++
		}
		json.NewEncoder(w).Encode(map[string]any{"maxOpsPerCall": 200, "entryContentMaxByLanguage": map[string]int{"en": 12000}, "trialMaxEntries": 1000})
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "test", nil })
	_, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true})
	if err == nil || !strings.Contains(err.Error(), "--create") || writes != 0 {
		t.Fatalf("preflight err=%v writes=%d", err, writes)
	}
}

func TestGroupingPreservesExistingSettingsAndEveryCharacter(t *testing.T) {
	f, _ := card.Init(t.TempDir(), "Synthetic", "zh-Hant")
	content := strings.Repeat("甲。乙！\r\n\r\n", 2000) + " 末尾 \n"
	f.Lorebook = &card.Lorebook{Entries: []card.LorebookEntry{
		{Name: "grouped by author", Content: content, MatchOptions: json.RawMessage(`{"groupId":"author","groupOrder":7}`)},
		{Name: "grouped automatically", Content: content},
	}}
	p, err := f.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	wb, _, groups := prepareEntries(f, p.Worldbook, 4000)
	entries := wb["entries"].([]map[string]any)
	if entries[0]["content"] != content || card.Digest(entries[0]["matchOptions"]) != card.Digest(map[string]any{"groupId": "author", "groupOrder": 7}) {
		t.Fatal("existing author group changed")
	}
	var restored strings.Builder
	for _, e := range entries[1:] {
		part := e["content"].(string)
		if utf8.RuneCountInString(part) > 4000 {
			t.Fatal("fragment too long")
		}
		restored.WriteString(part)
	}
	if restored.String() != content || len(groups) != 1 {
		t.Fatal("automatic grouping lost text")
	}
}
