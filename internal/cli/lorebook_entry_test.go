package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/hearthroom/cli/internal/auth"
	"github.com/hearthroom/cli/internal/output"
)

func TestLorebookEntryCLICommandsEndToEnd(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")

	version := 3
	description := "Keep book description"
	tags := "noir,harbor"
	entries := []map[string]any{
		{"entryId": "e1", "name": "Curfew", "content": "Bells ring at dusk.", "keywords": []string{"curfew"}, "category": "rule", "triggerRegion": "both", "isEnabled": true, "isConstant": false, "semanticRecall": true},
		{"entryId": "e2", "name": "Docks", "content": "Pier 4 stays lit.", "keywords": []string{"docks"}, "category": "location", "triggerRegion": "both", "isEnabled": true, "isConstant": false, "semanticRecall": true},
	}
	var docCalls int

	mux := http.NewServeMux()
	mux.HandleFunc("GET /open/v1/role/detail", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"characterRoleId": "role-1",
			"roleName":        "Harbor Detective",
			"language":        "en",
			"roleDetailDesc":  "A detective on the foggy docks.",
			"roleWelcome":     "Evening, stranger.",
		})
	})
	mux.HandleFunc("GET /open/v1/role/author-asset", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "none"})
	})
	mux.HandleFunc("GET /open/v1/image/list", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("GET /open/v1/worldbook/limits", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"maxOpsPerCall":             200,
			"entryContentMaxByLanguage": map[string]int{"en": 12000},
			"trialMaxEntries":           1000,
		})
	})
	mux.HandleFunc("GET /open/v1/worldbook/bindings", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"bindings": []map[string]any{{"worldbookId": "book-1", "name": "Harbor Lore"}},
		})
	})
	mux.HandleFunc("GET /open/v1/worldbook/detail", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"worldbookId":     "book-1",
			"name":            "Harbor Lore",
			"description":     description,
			"tags":            tags,
			"version":         version,
			"entryCount":      len(entries),
			"entryContentMax": 12000,
		})
	})
	mux.HandleFunc("GET /open/v1/worldbook/entry/list", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"worldbookId": "book-1",
			"entries":     entries,
		})
	})
	mux.HandleFunc("POST /open/v1/role/role-1/document", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	mux.HandleFunc("PATCH /open/v1/role/role-1/welcome", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	mux.HandleFunc("POST /open/v1/worldbook/book-1/document", func(w http.ResponseWriter, r *http.Request) {
		docCalls++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["metadata"] != nil {
			t.Fatalf("unexpected metadata in per-entry write: %v", body["metadata"])
		}
		if ev, ok := body["expectedVersion"].(float64); ok && int(ev) != version {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":          "version_conflict",
				"currentVersion": version,
				"detail":         map[string]any{"currentVersion": version, "expectedVersion": int(ev)},
			})
			return
		}
		created := []string{}
		ops, _ := body["entries"].([]any)
		for _, item := range ops {
			op, _ := item.(map[string]any)
			switch op["op"] {
			case "create":
				id := "e" + strconv.Itoa(len(entries)+1)
				row := map[string]any{
					"entryId":           id,
					"name":              op["name"],
					"content":           op["content"],
					"keywords":          op["keywords"],
					"secondaryKeywords": op["secondaryKeywords"],
					"category":          "custom",
					"triggerRegion":     "both",
					"isEnabled":         true,
					"isConstant":        false,
					"semanticRecall":    true,
				}
				if c, ok := op["category"].(string); ok && c != "" {
					row["category"] = c
				}
				if enabled, ok := op["isEnabled"].(bool); ok {
					row["isEnabled"] = enabled
				}
				if constant, ok := op["isConstant"].(bool); ok {
					row["isConstant"] = constant
				}
				if sr, ok := op["semanticRecall"].(bool); ok {
					row["semanticRecall"] = sr
				}
				entries = append(entries, row)
				created = append(created, id)
			case "update":
				id, _ := op["entryId"].(string)
				for i := range entries {
					if entries[i]["entryId"] == id {
						for _, k := range []string{"name", "content", "keywords", "secondaryKeywords", "category", "triggerRegion", "isEnabled", "isConstant", "semanticRecall", "matchOptions"} {
							if v, exists := op[k]; exists {
								entries[i][k] = v
							}
						}
					}
				}
			case "delete":
				id, _ := op["entryId"].(string)
				kept := entries[:0]
				for _, existing := range entries {
					if existing["entryId"] != id {
						kept = append(kept, existing)
					}
				}
				entries = kept
			}
		}
		version++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"worldbookId":     "book-1",
			"version":         version,
			"createdEntryIds": created,
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dir := filepath.Join(t.TempDir(), "card")
	common := []string{"--api", srv.URL, "--site", srv.URL, "--config-dir", cfg}

	if out, errOut, err := runCLI(t, append([]string{"card", "pull", "role-1", dir}, common...)...); err != nil {
		t.Fatalf("pull: %v (%s / %s)", err, out, errOut)
	}
	if out, errOut, err := runCLI(t, append([]string{"card", "push", dir, "--skip-media"}, common...)...); err != nil {
		t.Fatalf("baseline push: %v (%s / %s)", err, out, errOut)
	}
	docCalls = 0

	// 1. list --json
	listOut, _, err := runCLI(t, append([]string{"lorebook", "entry", "list", dir, "--json"}, common...)...)
	if err != nil {
		t.Fatalf("entry list: %v", err)
	}
	var listJSON struct {
		LorebookID string `json:"lorebookId"`
		Version    int    `json:"version"`
		Entries    []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(listOut), &listJSON); err != nil {
		t.Fatalf("unmarshal list: %v (%s)", err, listOut)
	}
	if listJSON.LorebookID != "book-1" || listJSON.Version != 3 || len(listJSON.Entries) != 2 {
		t.Fatalf("unexpected list json: %+v", listJSON)
	}

	// 2. get <dir> <entry> --json
	getOut, _, err := runCLI(t, append([]string{"lorebook", "entry", "get", dir, "Curfew", "--json"}, common...)...)
	if err != nil {
		t.Fatalf("entry get: %v", err)
	}
	if !strings.Contains(getOut, `"Bells ring at dusk."`) {
		t.Fatalf("unexpected get output: %s", getOut)
	}

	// 3. add via Markdown --file
	mdPath := filepath.Join(t.TempDir(), "lighthouse.md")
	mdContent := "---\nname: Lighthouse\nkeywords: lighthouse, beacon\ncategory: location\n---\nThe old beacon sweeps the bay.\n"
	if err := os.WriteFile(mdPath, []byte(mdContent), 0o644); err != nil {
		t.Fatal(err)
	}
	addOut, _, err := runCLI(t, append([]string{"lorebook", "entry", "add", dir, "--file", mdPath, "--json"}, common...)...)
	if err != nil {
		t.Fatalf("entry add: %v (%s)", err, addOut)
	}
	if !strings.Contains(addOut, `"Lighthouse"`) || version != 4 {
		t.Fatalf("unexpected add output: version=%d out=%s", version, addOut)
	}

	// 4. update via flags
	updOut, _, err := runCLI(t, append([]string{"lorebook", "entry", "update", dir, "Docks", "--content", "Pier 4 and Pier 9 stay lit.", "--keywords", "docks, pier", "--semantic-recall=false", "--disabled", "--json"}, common...)...)
	if err != nil {
		t.Fatalf("entry update: %v (%s)", err, updOut)
	}
	if version != 5 || entries[1]["content"] != "Pier 4 and Pier 9 stay lit." || entries[0]["content"] != "Bells ring at dusk." || entries[1]["isEnabled"] != false {
		t.Fatalf("unexpected entries after update: %+v", entries)
	}

	// 4b. round-trip `entry get --json` wrapper through `entry update --file`:
	// - re-enables omitted disabled field
	// - does NOT send empty matchOptions={} when remote entry has no matchOptions
	// - does NOT leak wrapper top-level "name" ("Harbor Lore") when inner entry omits "name"
	gotDocksJSON, _, err := runCLI(t, append([]string{"lorebook", "entry", "get", dir, "e2", "--json"}, common...)...)
	if err != nil {
		t.Fatalf("get e2 json: %v", err)
	}
	reEnabledJSON := strings.ReplaceAll(gotDocksJSON, `"disabled": true,`, "")
	reEnabledJSON = strings.ReplaceAll(reEnabledJSON, `"disabled":true,`, "")
	reEnabledJSON = strings.ReplaceAll(reEnabledJSON, `"name": "Docks",`, "")
	reEnabledJSON = strings.ReplaceAll(reEnabledJSON, `"name":"Docks",`, "")
	jsonPath := filepath.Join(t.TempDir(), "docks.json")
	if err := os.WriteFile(jsonPath, []byte(reEnabledJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, append([]string{"lorebook", "entry", "update", dir, "e2", "--file", jsonPath}, common...)...); err != nil {
		t.Fatalf("entry update from get --json file: %v", err)
	}
	if entries[1]["isEnabled"] != true || version != 6 {
		t.Fatalf("expected e2 re-enabled at version 6, got version=%d entry=%+v", version, entries[1])
	}
	if entries[1]["name"] != "Docks" {
		t.Fatalf("wrapper top-level Lorebook name leaked into entry name: got %q, want Docks", entries[1]["name"])
	}
	if _, hasMO := entries[1]["matchOptions"]; hasMO && entries[1]["matchOptions"] != nil {
		t.Fatalf("ordinary entry without matchOptions should not gain non-nil matchOptions on --file update: %+v", entries[1]["matchOptions"])
	}

	// 4c. --match-options "" is rejected as usage error (exit 2) without calling /document.
	callsBeforeInvalidMO := docCalls
	if _, _, err := runCLI(t, append([]string{"lorebook", "entry", "update", dir, "e2", "--match-options", ""}, common...)...); err == nil {
		t.Fatal("expected --match-options '' to fail")
	}
	if docCalls != callsBeforeInvalidMO {
		t.Fatalf("--match-options '' should not call server, got %d calls", docCalls-callsBeforeInvalidMO)
	}

	// 5. Simulate concurrent web edit (version 6 -> 7): stale update fails, --expected-version 7 succeeds.
	entries[0]["content"] = "Web edited curfew."
	version = 7
	if _, _, err := runCLI(t, append([]string{"lorebook", "entry", "update", dir, "Lighthouse", "--content", "Beacon repaired."}, common...)...); err == nil || !strings.Contains(err.Error(), "--expected-version 7") {
		t.Fatalf("expected 409 conflict guidance with --expected-version 7, got: %v", err)
	}
	if _, _, err := runCLI(t, append([]string{"lorebook", "entry", "update", dir, "Lighthouse", "--content", "Beacon repaired.", "--expected-version", "7"}, common...)...); err != nil {
		t.Fatalf("retry update with --expected-version 7: %v", err)
	}
	if entries[0]["content"] != "Web edited curfew." {
		t.Fatalf("web edit on e1 overwritten: %v", entries[0])
	}

	// 6. Because version 7 was bypassed (not pulled), delete without --expected-version conflicts at version 8:
	// verify delete-specific guidance AND structured --json detail ("version_conflict"), then succeed with --expected-version 8.
	delOut, delErrOut, err := runCLI(t, append([]string{"lorebook", "entry", "delete", dir, "Lighthouse", "--json"}, common...)...)
	if err == nil || !strings.Contains(err.Error(), "to delete only this entry") {
		t.Fatalf("expected delete-specific 409 conflict message, got err=%v out=%s stderr=%s", err, delOut, delErrOut)
	}
	var pErr output.Detailer
	if !errors.As(err, &pErr) || pErr.ErrorDetail() == nil {
		t.Fatalf("expected 409 conflict error to preserve structured API Detailer for --json, got %T: %v", err, err)
	}
	if _, _, err := runCLI(t, append([]string{"lorebook", "entry", "delete", dir, "Lighthouse", "--expected-version", "8", "--json"}, common...)...); err != nil {
		t.Fatalf("entry delete with --expected-version 8: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries after delete = %d, want 2", len(entries))
	}

	// 7. Full card push afterward sees worldbook as unchanged (0 additional /worldbook/.../document calls).
	callsBeforePush := docCalls
	pushOut, _, err := runCLI(t, append([]string{"card", "push", dir, "--skip-media", "--json"}, common...)...)
	if err != nil {
		t.Fatalf("card push after entry edits: %v (%s)", err, pushOut)
	}
	if docCalls != callsBeforePush {
		t.Fatalf("expected 0 worldbook document calls during push, got %d", docCalls-callsBeforePush)
	}
	if !strings.Contains(pushOut, `"worldbook"`) || strings.Contains(pushOut, `"changed":["worldbook"]`) {
		t.Fatalf("expected worldbook unchanged in push output: %s", pushOut)
	}
}

func TestLorebookEntryPromptSurfaceAudit(t *testing.T) {
	app, _, _ := testApp()
	root := app.rootCommand()
	loreCmd, _, err := root.Find([]string{"lorebook", "entry"})
	if err != nil || loreCmd == nil || loreCmd.Name() != "entry" {
		t.Fatalf("expected lorebook entry command registered, got err=%v", err)
	}

	var surface strings.Builder
	seenSentences := map[string]string{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		path := c.CommandPath()
		surface.WriteString("=== " + path + " ===\n")
		surface.WriteString(c.Short + "\n" + c.Long + "\n")
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			surface.WriteString("  --" + f.Name + ": " + f.Usage + "\n")
		})
		for _, raw := range strings.Split(c.Long, ".") {
			s := strings.TrimSpace(raw)
			if len(s) < 60 {
				continue
			}
			if prev, dup := seenSentences[s]; dup && prev != path {
				t.Errorf("duplicated long help sentence across %q and %q: %q", prev, path, s)
			}
			seenSentences[s] = path
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(loreCmd)

	full := surface.String()
	for _, banned := range []string{
		"worldbook_entry",
		"role_worldbook",
		"ApplyDocument",
		"localHashes",
		"LorebookEntryHashes",
		"sqlc",
	} {
		if strings.Contains(full, banned) {
			t.Errorf("model-facing help surface contains internal/banned token %q:\n%s", banned, full)
		}
	}
}
