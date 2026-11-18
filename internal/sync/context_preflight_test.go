package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextPreflightBeforeAnyPushMutation(t *testing.T) {
	calls, writes := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open/v1/role/context-preflight" {
			calls++
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":"card_contexts_disabled"}`)
			return
		}
		if r.Method != "GET" {
			writes++
		}
		fmt.Fprint(w, `{"characterRoleId":"owned","language":"en"}`)
	}))
	defer server.Close()
	f, err := card.Init(t.TempDir(), "Synthetic", "en")
	if err != nil {
		t.Fatal(err)
	}
	f.ContextsPresent = true
	before, _ := os.ReadFile(filepath.Join(f.Dir, card.StateDir, card.StateFile))
	client := api.New(server.URL, server.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	_, err = Push(context.Background(), client, f, PushOptions{To: "owned", SkipMedia: true})
	after, _ := os.ReadFile(filepath.Join(f.Dir, card.StateDir, card.StateFile))
	if err == nil || calls != 1 || writes != 0 || string(before) != string(after) {
		t.Fatalf("preflight did not stop writes: err=%v calls=%d writes=%d stateChanged=%v", err, calls, writes, string(before) != string(after))
	}
}

func TestContextPreflightRejectionNamesFileAndField(t *testing.T) {
	cases := []struct {
		detail string
		want   []string
	}{
		{`{"index":0,"context":"settle","field":"history","reason":"invalid"}`, []string{"contexts/settle.json", "history"}},
		{`{"index":1,"field":"id","reason":"invalid"}`, []string{"contexts/advisor.json", "id"}},
		{`{"index":0,"context":"settle","field":"contextBudget","reason":"over_budget"}`, []string{"contexts/settle.json", "contextBudget", "maxOutputTokens"}},
		{`{"index":0,"context":"settle","field":"worldbooks","reason":"not_found"}`, []string{"contexts/settle.json", "worldbooks", "Lorebook"}},
		{`{"index":0,"context":"settle","field":"secret","reason":"unknown"}`, []string{"contexts/settle.json", "secret"}},
		{`{"field":"main.assembly","reason":"not_allowed"}`, []string{"main.assembly"}},
	}
	for _, tc := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			fmt.Fprintf(w, `{"error":"invalid_arguments","message":"card_context_invalid","retryable":false,"detail":%s}`, tc.detail)
		}))
		f := &card.Folder{ContextsPresent: true, Contexts: []json.RawMessage{json.RawMessage(`{"id":"settle"}`), json.RawMessage(`{"id":"advisor"}`)}}
		client := api.New(server.URL, server.URL, "test", func(context.Context) (string, error) { return "tok", nil })
		err := remoteContextPreflight(context.Background(), client, f, card.Payload{}, "", &PushResult{})
		server.Close()
		if err == nil {
			t.Fatalf("%s: accepted", tc.detail)
		}
		for _, w := range tc.want {
			if !strings.Contains(err.Error(), w) {
				t.Fatalf("%s: %q lacks %q", tc.detail, err, w)
			}
		}
		if strings.Contains(err.Error(), "supports contexts") {
			t.Fatalf("%s: field error blamed on provider support: %q", tc.detail, err)
		}
		var apiErr *api.Error
		if !errors.As(err, &apiErr) {
			t.Fatalf("%s: lost the structured error for --json", tc.detail)
		}
	}
}

// Other rejections can carry a detail.reason too; only card_context_invalid is explained.
func TestContextRejectionIgnoresOtherReasons(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":"invalid_request","message":"validation failed","detail":{"reason":"invalid","field":"name"}}`)
	}))
	defer server.Close()
	f := &card.Folder{ContextsPresent: true, Contexts: []json.RawMessage{json.RawMessage(`{"id":"settle"}`)}}
	client := api.New(server.URL, server.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	err := remoteContextPreflight(context.Background(), client, f, card.Payload{}, "", &PushResult{})
	if err == nil || !strings.Contains(err.Error(), "supports contexts") {
		t.Fatalf("explained a rejection that is not card_context_invalid: %v", err)
	}
}
