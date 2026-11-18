package sync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

// Named Lorebooks (bound with a contextKey) share the bindings list with the
// main Lorebook. The main lorebook.json must never be matched to one of them.
func TestMainLorebookIgnoresNamedBindings(t *testing.T) {
	bindings := `{"bindings":[{"worldbookId":"book-main","name":"Main"},{"worldbookId":"book-x","name":"X","contextKey":"x","mainRecall":false}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/open/v1/worldbook/bindings" {
			fmt.Fprint(w, bindings)
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL)
		w.WriteHeader(404)
	}))
	defer server.Close()
	client := api.New(server.URL, server.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	f, err := card.Init(t.TempDir(), "Synthetic", "en")
	if err != nil {
		t.Fatal(err)
	}
	f.State.RoleID = "role"
	f.State.Target = "owned"
	f.State.LorebookID = "book-main"
	opts := PushOptions{}
	got, err := checkOwnedBoundLorebook(context.Background(), client, f, "role", "book-main", true, &opts, &PushResult{})
	if err != nil {
		t.Fatalf("main Lorebook push blocked by a named binding: %v", err)
	}
	if got != "book-main" || f.State.LorebookID != "book-main" {
		t.Fatalf("main Lorebook retargeted: got %q state %q", got, f.State.LorebookID)
	}

	only := bindingsOutput{}
	key := "x"
	only.Bindings = append(only.Bindings, struct {
		WorldbookID string  `json:"worldbookId"`
		ContextKey  *string `json:"contextKey"`
		MainRecall  *bool   `json:"mainRecall"`
		Name        string  `json:"name"`
	}{WorldbookID: "book-x", ContextKey: &key, Name: "X"})
	if id, _, ok := latestBinding(only); ok {
		t.Fatalf("a card with only named Lorebooks reported %q as its main Lorebook", id)
	}
}
