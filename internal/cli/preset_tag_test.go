package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/auth"
	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/output"
)

func TestTagsPresetCommand(t *testing.T) {
	cfg := authTestEnv(t)

	// JSON output matches GET /v1/tags/catalog without contacting any server.
	stdout, _, err := runCLI(t, "tags", "--preset", "--json", "--config-dir", cfg)
	if err != nil {
		t.Fatalf("tags --preset --json failed: %v", err)
	}
	var cat struct {
		Groups []string `json:"groups"`
		Items  []struct {
			Key   string            `json:"key"`
			Group string            `json:"group"`
			Names map[string]string `json:"names"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(stdout), &cat); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if len(cat.Groups) != 4 || len(cat.Items) != 52 {
		t.Fatalf("groups=%d items=%d, want 4 and 52", len(cat.Groups), len(cat.Items))
	}

	// Human output with --lang zh-Hant shows display names and warns against URL keys.
	human, _, err := runCLI(t, "tags", "--preset", "--lang", "zh-Hant", "--config-dir", cfg)
	if err != nil {
		t.Fatalf("tags --preset --lang zh-Hant failed: %v", err)
	}
	for _, want := range []string{"角色扮演", "世界模擬器", "play", "roleplay", "display name", "not its URL key"} {
		if !strings.Contains(human, want) {
			t.Errorf("human output missing %q:\n%s", want, human)
		}
	}

	// Invalid --lang or --lang without --preset exits 2.
	for _, badArgs := range [][]string{
		{"tags", "--preset", "--lang", "zh", "--config-dir", cfg},
		{"tags", "--lang", "en", "--config-dir", cfg},
	} {
		_, _, err := runCLI(t, badArgs...)
		if err == nil {
			t.Fatalf("expected error for %v", badArgs)
		}
		var outBuf, errBuf bytes.Buffer
		p := output.Printer{Out: &outBuf, Err: &errBuf}
		if code := p.Fail(err); code != 2 {
			t.Fatalf("args %v exit code = %d, want 2 (%v)", badArgs, code, err)
		}
	}
}

func TestCardSubmitPresetTagPreflightAndServerRefusal(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")

	// 1. Local preflight blocks before any HTTP call when card.json has only custom tags or URL keys.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := filepath.Join(t.TempDir(), "card")
	f, err := card.Init(dir, "Mira", "zh-Hant")
	if err != nil {
		t.Fatal(err)
	}
	f.Manifest.Tags = []string{"自訂標籤", "roleplay"} // URL key does not count
	f.State = card.State{Target: "owned", RoleID: "role-1"}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveState(); err != nil {
		t.Fatal(err)
	}
	if err := card.WriteRating(dir, &card.Rating{Answers: card.RatingAnswers{Version: 1, Other: "other.none"}, Rating: "G"}); err != nil {
		t.Fatal(err)
	}

	_, _, err = runCLI(t, "card", "submit", dir, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
	if err == nil {
		t.Fatal("expected local preflight failure when preset tag is missing")
	}
	var stdout, stderr bytes.Buffer
	printer := output.Printer{Out: &stdout, Err: &stderr}
	if code := printer.Fail(err); code != 2 {
		t.Fatalf("exit code = %d, want 2; err = %v", code, err)
	}
	for _, want := range []string{
		"no preset category or play-style tag in " + dir + "/card.json",
		"hearthroom tags --preset --lang zh-Hant",
		"not its key",
		"hearthroom card push " + dir,
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("preflight error missing %q:\n%s", want, stderr.String())
		}
	}
	if calls != 0 {
		t.Fatalf("expected 0 HTTP calls on local preflight failure, got %d", calls)
	}

	// 2. Server returns 400 preset_tag_required (e.g. local card.json has preset tag but was not pushed yet).
	f.Manifest.Tags = []string{"角色扮演"}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	refusalSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"preset_tag_required","detail":{"field":"roleTag","catalogEndpoint":"/v1/tags/catalog"}}`))
	}))
	defer refusalSrv.Close()

	for _, jsonMode := range []bool{false, true} {
		args := []string{"card", "submit", dir, "--api", refusalSrv.URL, "--site", refusalSrv.URL, "--config-dir", cfg}
		if jsonMode {
			args = append(args, "--json")
		}
		_, _, err := runCLI(t, args...)
		if err == nil {
			t.Fatal("expected server preset_tag_required failure")
		}
		stdout.Reset()
		stderr.Reset()
		p := output.Printer{JSON: jsonMode, Out: &stdout, Err: &stderr}
		if code := p.Fail(err); code != 2 {
			t.Fatalf("json=%v exit code = %d, want 2; err = %v", jsonMode, code, err)
		}
		if jsonMode {
			if !strings.Contains(stdout.String(), `"preset_tag_required"`) || !strings.Contains(stdout.String(), `"/v1/tags/catalog"`) {
				t.Fatalf("json output lost server detail: %s", stdout.String())
			}
		} else {
			for _, want := range []string{
				"the last pushed version has no platform preset category or play-style tag in card.json `tags`",
				"hearthroom tags --preset",
				"hearthroom card push " + dir,
			} {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("human server error missing %q:\n%s", want, stderr.String())
				}
			}
		}
	}
}

func TestVisibilitySetPublicPresetTagRequiredGuidesPushAndSubmit(t *testing.T) {
	srv, dir, cfg := visibilityFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"preset_tag_required","detail":{"field":"roleTag","catalogEndpoint":"/v1/tags/catalog"}}`))
	}, func(http.ResponseWriter, *http.Request) {
		t.Error("unexpected operation GET")
	})

	_, _, err := runCLI(t, "card", "visibility", "set", "public", dir, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
	if err == nil {
		t.Fatal("expected preset_tag_required rejection")
	}
	var stdout, stderr bytes.Buffer
	p := output.Printer{Out: &stdout, Err: &stderr}
	if code := p.Fail(err); code != 2 {
		t.Fatalf("exit code = %d, want 2; err = %v", code, err)
	}
	for _, want := range []string{
		"restoring public access requires at least one platform preset category or play-style tag, and the approved version has none",
		"hearthroom tags --preset",
		"hearthroom card push " + dir,
		"hearthroom card submit " + dir,
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("visibility error missing %q:\n%s", want, stderr.String())
		}
	}
	f, loadErr := card.Load(dir)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if f.State.VisibilityOperation != nil {
		t.Fatalf("definitive 400 preset_tag_required must clear saved VisibilityOperation, got %+v", f.State.VisibilityOperation)
	}
}
