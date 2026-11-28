package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hearthroom/cli/internal/auth"
)

const presetRoleID = "00000000-0000-4000-8000-000000000003"

func TestPresetCardBindingWorkflow(t *testing.T) {
	t.Setenv(auth.EnvToken, "fixture")
	dir := t.TempDir()
	var mu sync.Mutex
	bindingVersion, writes := 0, 0
	mode, state := "card_default", "ready"
	conflict, partialReceipt := false, false
	keys := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("missing bearer")
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /open/v1/player/preset-default":
			if r.URL.Query().Get("roleId") != presetRoleID {
				t.Error("wrong card")
			}
		case "POST /open/v1/player/preset-default/save":
			writes++
			key := r.Header.Get("Idempotency-Key")
			if key == "" || keys[key] {
				t.Error("missing or repeated write key")
			}
			keys[key] = true
			var got map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Error(err)
			}
			if string(got["roleId"]) != fmt.Sprintf("%q", presetRoleID) || string(got["scope"]) != `"card"` ||
				string(got["expectedVersion"]) != fmt.Sprint(bindingVersion) {
				t.Errorf("wrong binding request: %v", got)
			}
			if conflict {
				w.WriteHeader(http.StatusConflict)
				io.WriteString(w, `{"error":"preset_conflict","reason":"preset_revision_conflict"}`)
				return
			}
			if mode == "card_default" {
				if string(got["mode"]) != `"preset"` || string(got["presetId"]) != fmt.Sprintf("%q", presetFixtureID) {
					t.Error("wrong preset selection")
				}
				mode, state = "revision", "ready" // Older Providers use revision for the saved selection.
			} else {
				if string(got["mode"]) != `"card_default"` || len(got["presetId"]) != 0 {
					t.Error("reset should omit the preset ID")
				}
				mode = "card_default"
			}
			bindingVersion++
			if partialReceipt {
				io.WriteString(w, `{"mode":"revision"}`)
				return
			}
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if mode == "card_default" {
			fmt.Fprintf(w, `{"mode":"card_default","presetId":null,"bindingVersion":%d,"state":"ready"}`, bindingVersion)
		} else if state == "unavailable" {
			fmt.Fprintf(w, `{"mode":%q,"presetId":%q,"bindingVersion":%d,"state":"unavailable"}`, mode, presetFixtureID, bindingVersion)
		} else {
			fmt.Fprintf(w, `{"mode":%q,"presetId":%q,"bindingVersion":%d,"version":7,"state":"ready"}`, mode, presetFixtureID, bindingVersion)
		}
	}))
	defer server.Close()
	run := func(args ...string) (string, error) {
		t.Helper()
		stdout, _, err := runCLI(t, append([]string{"--api", server.URL, "--config-dir", dir, "--json", "preset"}, args...)...)
		return stdout, err
	}
	check := func(source, id string, version int, args ...string) {
		t.Helper()
		stdout, err := run(append(args, "--role", presetRoleID)...)
		var got struct {
			Source   string
			PresetID string
			Version  int
		}
		if err != nil || json.Unmarshal([]byte(stdout), &got) != nil || got.Source != source || got.PresetID != id || got.Version != version {
			t.Fatalf("%v: %s (%v)", args, stdout, err)
		}
	}
	check("platform-default", "", 0, "status")
	check("custom", presetFixtureID, 7, "bind", presetFixtureID)
	mu.Lock()
	mode = "preset"
	mu.Unlock()
	check("custom", presetFixtureID, 7, "status")
	mu.Lock()
	state = "unavailable"
	mu.Unlock()
	check("unavailable", presetFixtureID, 0, "status")
	check("platform-default", "", 0, "unbind")

	mu.Lock()
	conflict = true
	mu.Unlock()
	if _, err := run("bind", presetFixtureID, "--role", presetRoleID); err == nil || !strings.Contains(err.Error(), "preset status") {
		t.Fatalf("expected a conflict with status guidance, got %v", err)
	}
	mu.Lock()
	conflict, partialReceipt = false, true
	mu.Unlock()
	if _, err := run("bind", presetFixtureID, "--role", presetRoleID); err == nil || !strings.Contains(err.Error(), "did not confirm") {
		t.Fatalf("expected an unconfirmed result for an incomplete receipt, got %v", err)
	}
	check("custom", presetFixtureID, 7, "status")
	mu.Lock()
	defer mu.Unlock()
	if writes != 4 {
		t.Fatalf("writes = %d, want 4", writes)
	}
}

func TestPresetBindingRejectsInvalidIDsBeforeRequest(t *testing.T) {
	t.Setenv(auth.EnvToken, "fixture")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"bind", "invalid", "--role", presetRoleID},
		{"unbind", "--role", "00000000-0000-0000-0000-000000000000"},
		{"status", "--role", "invalid"},
	} {
		_, _, err := runCLI(t, append([]string{"--api", server.URL, "--config-dir", dir, "preset"}, args...)...)
		if err == nil || !strings.Contains(err.Error(), "ID") {
			t.Fatalf("%v: expected invalid ID error, got %v", args, err)
		}
	}
}
