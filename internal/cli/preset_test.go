package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hearthroom/cli/internal/auth"
)

const presetSample = `{"suki_response_slots":true,"suki_tool_reply":true,"suki_takeover_last_user_message":true,"suki_material_roles":{"userHistory":"assistant"},"prompts":[{"identifier":"chatHistory","marker":true},{"identifier":"ending","role":"system","content":"{{ruleSlot}} {{writingStyleSlot}} {{perspectiveSlot}} {{lengthSlot}} {{turnInputInstructionSlot}} {{toolReplySlot}} {{lastUserMessage}}"}],"prompt_order":[{"character_id":1,"order":[{"identifier":"chatHistory","enabled":true},{"identifier":"ending","enabled":true}]}]}`
const presetNativeSample = `{"schemaVersion":"hearthroom.st-preset.v1","st":` + presetSample + `,"selection":{"profileId":1,"toolReply":false}}`
const presetFixtureID = "00000000-0000-4000-8000-000000000001"
const previewFixtureID = "00000000-0000-4000-8000-000000000002"

func writePresetFixture(t *testing.T, fixture string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "preset.json")
	if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPresetCheckOffline(t *testing.T) {
	t.Setenv(auth.EnvToken, "")
	for _, tc := range []struct {
		name, fixture, contains string
		invalid                 bool
	}{
		{"slots", presetSample, `"profileId": 1`, false},
		{"native selection", presetNativeSample, `"toolReply": false`, false},
		{"boolean", strings.Replace(presetSample, `"suki_tool_reply":true`, `"suki_tool_reply":"true"`, 1), "suki_tool_reply", true},
		{"user history role", strings.Replace(presetSample, `"assistant"`, `"developer"`, 1), "suki_material_roles", true},
		{"misplaced slot", strings.Replace(presetSample, `"role":"system"`, `"role":"user"`, 1), "after chatHistory", false},
		{"duplicate key", strings.Replace(presetSample, "{", `{"suki_tool_reply":false,`, 1), "duplicate", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, err := runCLI(t, "preset", "check", writePresetFixture(t, tc.fixture), "--config-dir", t.TempDir(), "--api", "http://127.0.0.1:1", "--json")
			if (err != nil) != tc.invalid || !strings.Contains(stdout, tc.contains) || !json.Valid([]byte(stdout)) {
				t.Fatalf("output %s, error %v", stdout, err)
			}
		})
	}
}

func TestPresetWorkflowUsesOwnerRoutes(t *testing.T) {
	t.Setenv(auth.EnvToken, "fixture")
	out := t.TempDir()
	var mu sync.Mutex
	calls := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		path := strings.TrimPrefix(r.URL.Path, "/open/v1")
		calls[r.Method+" "+path]++
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("missing bearer")
		}
		if strings.HasSuffix(path, "/confirm") || strings.HasSuffix(path, "/revisions") {
			if r.Header.Get("Idempotency-Key") == "" {
				t.Error("missing idempotency key")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		var payload map[string]json.RawMessage
		switch r.Method + " " + path {
		case "GET /me":
			io.WriteString(w, `{"accountNumId":123}`)
		case "POST /preset-imports/preview":
			f, _, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			defer f.Close()
			raw, _ := io.ReadAll(f)
			if string(raw) != presetSample || r.FormValue("profileId") != "1" {
				t.Error("preview file or profile changed")
			}
			w.WriteHeader(201)
			io.WriteString(w, `{"previewId":"`+previewFixtureID+`","version":1,"selectionDigest":"fixture-digest"}`)
		case "POST /preset-imports/" + previewFixtureID + "/confirm":
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if string(payload["selectionDigest"]) != `"fixture-digest"` || string(payload["expectedPreviewVersion"]) != "1" {
				t.Error("confirm did not use preview receipt")
			}
			w.WriteHeader(201)
			io.WriteString(w, `{"presetId":"`+presetFixtureID+`","revisionId":"`+previewFixtureID+`","version":1}`)
		case "GET /presets":
			if r.URL.Query().Get("cursor") == "" {
				io.WriteString(w, `{"items":[{"id":"`+presetFixtureID+`","title":"Fixture","version":1}],"nextCursor":"next","quota":{"used":1,"limit":50}}`)
			} else {
				io.WriteString(w, `{"items":[{"id":"`+previewFixtureID+`","title":"Further","version":2}],"quota":{"used":2,"limit":50}}`)
			}
		case "GET /presets/" + presetFixtureID:
			if calls["GET "+path] == 1 {
				// A save changed the content after the first metadata read.
				io.WriteString(w, `{"title":"Original title","headRevisionId":"`+presetFixtureID+`","version":2}`)
				return
			}
			io.WriteString(w, `{"title":"Original title","headRevisionId":"`+previewFixtureID+`","version":3,"recommendedModels":["series:Fixture"],"purpose":"general"}`)
		case "GET /presets/" + presetFixtureID + "/export":
			io.WriteString(w, presetSample)
		case "GET /presets/" + presetFixtureID + "/content":
			io.WriteString(w, `{"revision":{"revisionId":"`+previewFixtureID+`","version":1,"document":`+presetNativeSample+`}}`)
		case "POST /presets/" + presetFixtureID + "/revisions":
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if string(payload["title"]) != `"Original title"` || string(payload["recommendedModels"]) != `["series:Fixture"]` || string(payload["expectedVersion"]) != "3" {
				t.Error("revise lost metadata or expected version")
			}
			w.WriteHeader(409)
			io.WriteString(w, `{"error":"preset_conflict"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	run := func(args ...string) (string, error) {
		stdout, _, err := runCLI(t, append([]string{"--api", server.URL, "--config-dir", out, "--json", "preset"}, args...)...)
		return stdout, err
	}
	if _, err := run("push", writePresetFixture(t, presetSample), "--create", "--title", "Fixture"); err != nil {
		t.Fatal(err)
	}
	if stdout, err := run("list"); err != nil || !strings.Contains(stdout, "Further") {
		t.Fatalf("list: %s, %v", stdout, err)
	}
	for _, format := range []string{"st", "hearthroom"} {
		path := filepath.Join(out, format+".json")
		stdout, err := run("pull", presetFixtureID, "--out", path, "--format", format)
		if err != nil {
			t.Fatal(err)
		}
		expected := presetSample
		if format == "hearthroom" {
			expected = presetNativeSample
			if !strings.Contains(stdout, `"version": 3`) {
				t.Fatalf("edit export did not identify the asset version after publication: %s", stdout)
			}
		}
		if raw, err := os.ReadFile(path); err != nil || string(raw) != expected {
			t.Fatalf("pull %s: file %q, error %v", format, raw, err)
		}
		if _, err := run("pull", presetFixtureID, "--out", path, "--format", format); err == nil {
			t.Fatal("pull overwrote existing export")
		}
	}
	_, err := run("push", filepath.Join(out, "hearthroom.json"), "--id", presetFixtureID, "--expected-version", "3")
	if err == nil || !strings.Contains(err.Error(), "preset_conflict") {
		t.Fatalf("expected conflict, got %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls["POST /presets/"+presetFixtureID+"/revisions"] != 1 || calls["GET /presets"] != 2 {
		t.Fatalf("unexpected retries or pagination: %v", calls)
	}
	if calls["GET /presets/"+presetFixtureID+"/content"] != 2 {
		t.Fatal("editing export did not retry after the head changed")
	}
}

func TestPresetImportRetriesSameConfirmation(t *testing.T) {
	t.Setenv(auth.EnvToken, "fixture")
	var mu sync.Mutex
	var previews, confirmations int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/open/v1/me":
			io.WriteString(w, `{"accountNumId":123}`)
		case "/open/v1/preset-imports/preview":
			previews++
			io.WriteString(w, `{"previewId":"`+previewFixtureID+`","version":3,"selectionDigest":"same-digest"}`)
		case "/open/v1/preset-imports/" + previewFixtureID + "/confirm":
			confirmations++
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if r.Header.Get("Idempotency-Key") != "same-operation" || in["expectedPreviewVersion"] != float64(3) ||
				in["selectionDigest"] != "same-digest" || in["title"] != "Fixture" {
				t.Error("confirmation retry changed the original request")
			}
			if confirmations == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				io.WriteString(w, `{"error":"temporarily_unavailable","detail":"fixture private content"}`)
				return
			}
			io.WriteString(w, `{"presetId":"`+presetFixtureID+`","revisionId":"`+previewFixtureID+`","version":1}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	file := writePresetFixture(t, presetSample)
	args := []string{"preset", "push", file, "--create", "--title", "Fixture", "--api", srv.URL,
		"--config-dir", t.TempDir(), "--operation-id", "same-operation", "--json"}
	_, _, err := runCLI(t, args...)
	if err == nil || !strings.Contains(err.Error(), "--operation-id same-operation") || strings.Contains(err.Error(), "fixture private content") {
		t.Fatalf("expected a private recovery prompt, got %v", err)
	}
	changed := append([]string(nil), args...)
	changed[5] = "Different title"
	if _, _, err := runCLI(t, changed...); err == nil || !strings.Contains(err.Error(), "different preset save") {
		t.Fatalf("changing the inputs before retry should fail, got %v", err)
	}
	if stdout, _, err := runCLI(t, args...); err != nil || !strings.Contains(stdout, presetFixtureID) {
		t.Fatalf("retry: %s, %v", stdout, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if previews != 1 || confirmations != 2 {
		t.Fatalf("%d previews, %d confirmations", previews, confirmations)
	}
}
