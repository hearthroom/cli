package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hearthroom/cli/internal/auth"
)

// media mv shows the plan and the cards that would break; only --yes moves.
func TestMediaMovePreviewsThenMovesWithYes(t *testing.T) {
	var mu sync.Mutex
	var calls []map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/open/v1/image/list", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"libraryPrefix": "https://cdn.test/u/abc",
			"capabilities":  map[string]any{"relativePaths": true, "moves": true},
			"imageList":     []any{map[string]any{"id": "i1", "fileName": "scene/art/a.png"}},
		}})
	})
	mux.HandleFunc("/open/v1/image/folder/list", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"folders": []any{map[string]any{"id": "f1", "name": "scene/art"}}}})
	})
	mux.HandleFunc("/open/v1/image/folder/rename", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		calls = append(calls, body)
		mu.Unlock()
		data := map[string]any{"moved": 1, "dryRun": body["dryRun"] == true}
		if body["dryRun"] == true {
			data["usedBy"] = []any{map[string]any{"roleId": "r1", "name": "Faces", "published": true}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv(auth.EnvToken, "tok")
	dir := t.TempDir()

	out, _, err := runCLI(t, "media", "mv", "scene", "story", "--api", srv.URL, "--config-dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0]["dryRun"] != true || calls[0]["path"] != "scene" || calls[0]["name"] != "story" {
		t.Fatalf("preview calls: %v", calls)
	}
	if !strings.Contains(out, "Faces (published version, cannot be edited)") || !strings.Contains(out, "--yes") {
		t.Fatalf("preview output:\n%s", out)
	}
	out, _, err = runCLI(t, "media", "mv", "scene", "story", "--yes", "--json", "--api", srv.URL, "--config-dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || calls[2]["confirm"] != true || calls[2]["path"] != "scene" {
		t.Fatalf("move calls: %v", calls)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil || res["moved"] != true {
		t.Fatalf("json: %s %v", out, err)
	}
	// A folder that exists by name is renamed by id.
	if _, _, err := runCLI(t, "media", "mv", "scene/art", "scene/images", "--api", srv.URL, "--config-dir", dir); err != nil || calls[3]["folderId"] != "f1" {
		t.Fatalf("by id: %v %v", calls, err)
	}
}
