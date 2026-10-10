package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

func detailServer(t *testing.T, readme *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open/v1/role/detail":
			json.NewEncoder(w).Encode(map[string]any{"characterRoleId": "r1", "roleName": "Mira", "roleReadme": *readme})
		case "/open/v1/worldbook/bindings":
			json.NewEncoder(w).Encode(map[string]any{"bindings": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
}

// Pull writes intro.md from roleReadme, lists it, and on a forced pull of a
// card whose introduction was cleared removes the stale file so the next
// push does not send it back.
func TestPullWritesIntro(t *testing.T) {
	readme := "# Mira\n\nA lighthouse keeper."
	srv := detailServer(t, &readme)
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	dir := t.TempDir()
	res, err := Pull(context.Background(), c, "r1", dir, PullOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, card.IntroFile))
	if err != nil || string(b) != readme+"\n" {
		t.Fatalf("intro.md = %q, %v", b, err)
	}
	listed := false
	for _, f := range res.Files {
		listed = listed || f == card.IntroFile
	}
	if !listed {
		t.Fatalf("files = %v, want %s", res.Files, card.IntroFile)
	}

	readme = ""
	res, err = Pull(context.Background(), c, "r1", dir, PullOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, card.IntroFile)); !os.IsNotExist(err) {
		t.Fatalf("stale intro.md kept after the remote cleared it: %v", err)
	}
	for _, f := range res.Files {
		if f == card.IntroFile {
			t.Fatalf("files = %v lists intro.md for an empty roleReadme", res.Files)
		}
	}
}

// Both push targets carry roleReadme with the card fields.
func TestPushSendsIntro(t *testing.T) {
	var trialCard, ownedFields map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			var body struct{ Card map[string]any }
			json.NewDecoder(r.Body).Decode(&body)
			trialCard = body.Card
			json.NewEncoder(w).Encode(map[string]any{"clientKey": "k", "roleId": "r", "created": true, "sections": map[string]string{}})
		case r.Method == http.MethodPost && r.URL.Path == "/open/v1/role/owned/document":
			var body struct{ Fields map[string]any }
			json.NewDecoder(r.Body).Decode(&body)
			ownedFields = body.Fields
			w.Write([]byte(`{}`))
		default:
			w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })

	dir := t.TempDir()
	if _, err := card.Init(dir, "Intro", "en"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, card.IntroFile), []byte("# Page\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	if trialCard["roleReadme"] != "# Page" {
		t.Fatalf("trial card = %v, want roleReadme", trialCard)
	}

	if err := os.WriteFile(filepath.Join(dir, card.IntroFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f, _ = card.Load(dir)
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true, To: "owned"}); err != nil {
		t.Fatal(err)
	}
	if v, ok := ownedFields["roleReadme"]; !ok || v != "" {
		t.Fatalf("owned document fields = %v, want roleReadme cleared", ownedFields)
	}
}
