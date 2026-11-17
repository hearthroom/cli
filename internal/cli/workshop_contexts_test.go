package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/auth"
)

func TestWorkshopContextsPreflightBeforeSave(t *testing.T) {
	for _, supported := range []bool{false, true} {
		t.Run(map[bool]string{false: "old-provider", true: "supported"}[supported], func(t *testing.T) {
			cfg := authTestEnv(t)
			t.Setenv(auth.EnvToken, "tok")
			root := t.TempDir()
			_ = os.Mkdir(filepath.Join(root, "contexts"), 0700)
			_ = os.WriteFile(filepath.Join(root, "draft.json"), []byte(`{"name":"Synthetic DLC","worldbook":[]}`), 0600)
			_ = os.WriteFile(filepath.Join(root, "contexts", "settle.json"), []byte(`{"id":"settle","name":"Settlement","preset":"contexts/settle.preset.json","worldbooks":["self"],"compilerVersion":"untrusted","compiledPreset":{"private":"untrusted"}}`), 0600)
			_ = os.WriteFile(filepath.Join(root, "contexts", "settle.preset.json"), []byte(`{"schemaVersion":"hearthroom.st-preset.v1","st":{},"selection":{}}`), 0600)
			var paths []string
			saves := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if r.URL.Path == "/open/v1/workshop/context-preflight" && !supported {
					http.Error(w, "unsupported", 404)
					return
				}
				if r.URL.Path != "/open/v1/workshop/context-preflight" && r.URL.Path != "/open/v1/workshop/draft/save" {
					t.Errorf("unexpected private/base lookup: %s", r.URL)
					w.WriteHeader(500)
					return
				}
				c := body["contexts"].([]any)[0].(map[string]any)
				if _, ok := c["preset"].(map[string]any); !ok {
					t.Error("sent local preset path")
				}
				if c["compiledPreset"] != nil || c["compilerVersion"] != nil {
					t.Error("sent untrusted compiler metadata")
				}
				if r.URL.Path == "/open/v1/workshop/context-preflight" {
					_, _ = w.Write([]byte(`{"kind":"dlc","cardContexts":true,"contexts":[{"id":"settle","fixedPrefixTokens":123,"materials":["preset"]}],"contextQuotes":[{"id":"settle","model":{"id":"fixture","name":"Fixture"},"maxCharge":2}]}`))
					return
				}
				saves++
				_, _ = w.Write([]byte(`{"revision":2,"errors":[]}`))
			}))
			defer srv.Close()
			stdout, _, err := runCLI(t, "workshop", "update", "mine", "--file", filepath.Join(root, "draft.json"), "--context-root", root, "--expected-revision", "1", "--operation-id", "save-contexts", "--api", srv.URL, "--config-dir", cfg, "--json")
			if !supported {
				if err == nil || len(paths) != 1 || saves != 0 {
					t.Fatalf("unsupported provider did not stop before writes: err=%v paths=%v saves=%d", err, paths, saves)
				}
				return
			}
			if err != nil || saves != 1 || len(paths) != 2 || paths[0] != "/open/v1/workshop/context-preflight" {
				t.Fatalf("context update: err=%v paths=%v saves=%d", err, paths, saves)
			}
			if !strings.Contains(stdout, "fixedPrefixTokens") || !strings.Contains(stdout, "fixture") {
				t.Fatalf("lost server preflight report: %s", stdout)
			}
		})
	}
}
