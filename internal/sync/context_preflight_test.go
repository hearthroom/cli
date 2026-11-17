package sync

import (
	"context"
	"fmt"
	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
