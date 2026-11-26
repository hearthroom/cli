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

func TestWorkshopLifecycleUsesProviderContracts(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Error("missing bearer")
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		var body map[string]any
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
		}
		switch r.URL.Path {
		case "/open/v1/workshop/create":
			if body["roleId"] != "source-role" || body["idempotencyKey"] != "create-intent" {
				t.Fatalf("create=%v", body)
			}
			_, _ = w.Write([]byte(`{"workshopId":"my-extension"}`))
		case "/open/v1/workshop/mine":
			_, _ = w.Write([]byte(`{"items":[{"workshopId":"my-extension","kind":"dlc","name":"Harbor scenario","status":"draft"}]}`))
		case "/open/v1/workshop/draft":
			if r.URL.Query().Get("workshopId") != "my-extension" {
				t.Fatal(r.URL.String())
			}
			_, _ = w.Write([]byte(`{"workshopId":"my-extension","revision":3,"kind":"dlc","card":{"roleId":"source-role"},"name":"Harbor scenario","worldbook":[{"id":"harbor","packId":"5168c881-178b-4a2f-8a83-cc158709143e"}],"lorePacks":[{"id":"5168c881-178b-4a2f-8a83-cc158709143e","name":"Harbor","defaultEnabled":true}],"scriptCanSwitchPacks":true,"rules":{"rules":[]},"opening":{"mode":"greeting","welcome":"Welcome."}}`))
		case "/open/v1/workshop/draft/save":
			if body["workshopId"] != "my-extension" || body["expectedRevision"] != float64(3) || body["idempotencyKey"] != "save-intent" {
				t.Fatalf("save=%v", body)
			}
			if _, ok := body["kind"]; ok {
				t.Fatal("submitted derived kind")
			}
			if _, ok := body["card"]; ok {
				t.Fatal("submitted source card")
			}
			if body["opening"].(map[string]any)["welcome"] != "Welcome." {
				t.Fatal("opening lost")
			}
			packs, ok := body["lorePacks"].([]any)
			if !ok || len(packs) != 1 || body["scriptCanSwitchPacks"] != true {
				t.Fatal("author pack settings lost")
			}
			pack := packs[0].(map[string]any)
			entries := body["worldbook"].([]any)
			if pack["name"] != "Harbor" || pack["defaultEnabled"] != true || entries[0].(map[string]any)["packId"] != pack["id"] {
				t.Fatal("pack membership lost")
			}
			_, _ = w.Write([]byte(`{"revision":4,"constantChars":0,"errors":[]}`))
		case "/open/v1/workshop/publish", "/open/v1/workshop/unpublish":
			if body["workshopId"] != "my-extension" || body["expectedRevision"] != float64(4) || body["operationId"] != "publish-intent" {
				t.Fatalf("action=%v", body)
			}
			_, _ = w.Write([]byte(`{"workshopId":"my-extension","version":1,"status":"published"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL)
		}
	}))
	defer srv.Close()
	run := func(args ...string) string {
		t.Helper()
		args = append(args, "--api", srv.URL, "--config-dir", cfg, "--json")
		stdout, _, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return stdout
	}
	run("workshop", "create", "source-role", "--operation-id", "create-intent")
	if got := run("workshop", "list"); !strings.Contains(got, "my-extension") {
		t.Fatal(got)
	}
	draft := run("workshop", "get", "my-extension")
	file := filepath.Join(t.TempDir(), "draft.json")
	if err := os.WriteFile(file, []byte(draft), 0600); err != nil {
		t.Fatal(err)
	}
	run("workshop", "update", "my-extension", "--file", file, "--expected-revision", "3", "--operation-id", "save-intent")
	run("workshop", "publish", "my-extension", "--expected-revision", "4", "--operation-id", "publish-intent")
	run("workshop", "unpublish", "my-extension", "--expected-revision", "4", "--operation-id", "publish-intent")
	if len(calls) != 6 {
		t.Fatalf("calls=%v", calls)
	}
}

func TestWorkshopRejectsUnsafeInputAndMissingCASBeforeRequests(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.Error(w, "unexpected", 500) }))
	defer srv.Close()
	for _, payload := range []string{`{"name":"Synthetic","systemPrompt":"private"}`, `{"workshopId":"other","name":"Synthetic"}`, `null`, `[]`, `{"name":"Synthetic"} {}`} {
		file := filepath.Join(t.TempDir(), "draft.json")
		if err := os.WriteFile(file, []byte(payload), 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err := runCLI(t, "workshop", "update", "mine", "--file", file, "--expected-revision", "3", "--operation-id", "save", "--api", srv.URL, "--config-dir", cfg)
		if err == nil {
			t.Errorf("accepted %s", payload)
		}
	}
	for _, args := range [][]string{
		{"workshop", "create", "source"},
		{"workshop", "publish", "mine", "--operation-id", "publish"},
		{"workshop", "unpublish", "mine", "--expected-revision", "0", "--operation-id", "unpublish"},
	} {
		_, _, err := runCLI(t, append(args, "--api", srv.URL, "--config-dir", cfg)...)
		if err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	if calls != 0 {
		t.Fatalf("unsafe requests=%d", calls)
	}
}

func TestWorkshopOpeningPageLimit(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"revision":4}`))
	}))
	defer srv.Close()
	const limit = 512 * 1024
	for _, extra := range []string{"xx", "xxx"} {
		page := strings.Repeat("界", limit/3) + extra
		data, err := json.Marshal(map[string]any{"opening": map[string]string{"openingPage": page}})
		if err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(t.TempDir(), "draft.json")
		if err := os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err = runCLI(t, "workshop", "update", "mine", "--file", file, "--expected-revision", "3", "--operation-id", "save", "--api", srv.URL, "--config-dir", cfg)
		if len(page) == limit && err != nil {
			t.Fatalf("exact limit: %v", err)
		}
		if len(page) > limit && (err == nil || !strings.Contains(err.Error(), "524288")) {
			t.Fatalf("over limit: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("save requests = %d, want 1", calls)
	}
}

func TestWorkshopPolicyAndServerDenial(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open/v1/workshop/policy":
			if r.Method != "GET" || r.URL.Query().Get("roleId") != "source" {
				t.Fatal(r.URL.String())
			}
			_, _ = w.Write([]byte(`{"allowNew":false,"revision":0}`))
		case "/open/v1/workshop/policy/save":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["allowNew"] != false || body["expectedRevision"] != float64(0) {
				t.Fatalf("policy=%v", body)
			}
			_, _ = w.Write([]byte(`{"allowNew":false,"revision":1}`))
		default:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden"}`))
		}
	}))
	defer srv.Close()
	for _, args := range [][]string{{"workshop", "policy", "source"}, {"workshop", "policy-set", "source", "--allow-new=false", "--expected-revision", "0"}} {
		_, _, err := runCLI(t, append(args, "--api", srv.URL, "--config-dir", cfg)...)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, _, err := runCLI(t, "workshop", "get", "foreign", "--api", srv.URL, "--config-dir", cfg)
	if err == nil || !strings.Contains(err.Error(), "403 forbidden") {
		t.Fatalf("denial=%v", err)
	}
}

func TestWorkshopRequiresAuthentication(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "")
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.Error(w, "unexpected", 500) }))
	defer srv.Close()
	_, _, err := runCLI(t, "workshop", "create", "source", "--operation-id", "create", "--api", srv.URL, "--config-dir", cfg)
	if err == nil || !strings.Contains(err.Error(), "sign") {
		t.Fatalf("missing auth error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("requests without auth=%d", calls)
	}
}
