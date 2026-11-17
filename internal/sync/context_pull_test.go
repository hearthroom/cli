package sync

import (
	"context"
	"fmt"
	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContextPullReadsEveryNamedBookAndRemovesAbsentDeclarations(t *testing.T) {
	absent := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open/v1/role/detail":
			if absent {
				fmt.Fprint(w, `{"characterRoleId":"role","roleName":"Synthetic","language":"en"}`)
			} else {
				fmt.Fprint(w, `{"characterRoleId":"role","roleName":"Synthetic","language":"en","contexts":[{"id":"settle","name":"Settle","preset":{"schemaVersion":"hearthroom.st-preset.v1","st":{"prompts":[],"prompt_order":[]},"selection":{"profileId":0}}}]}`)
			}
		case "/open/v1/worldbook/bindings":
			if absent {
				fmt.Fprint(w, `{"bindings":[]}`)
			} else {
				fmt.Fprint(w, `{"bindings":[{"worldbookId":"one","name":"One","contextKey":"a","mainRecall":false},{"worldbookId":"two","name":"Two","contextKey":"b","mainRecall":true}]}`)
			}
		case "/open/v1/worldbook/detail":
			fmt.Fprint(w, `{"name":"Synthetic","version":3}`)
		case "/open/v1/worldbook/entry/list":
			fmt.Fprintf(w, `{"entries":[{"entryId":"%s-entry","name":"Synthetic","content":"Synthetic content","isEnabled":true,"semanticRecall":false,"keywords":[]}]}`, r.URL.Query().Get("worldbookId"))
		default:
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":"not found"}`)
		}
	}))
	defer server.Close()
	c := api.New(server.URL, server.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "role", dir, PullOptions{}); err != nil {
		t.Fatal(err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Lorebooks) != 2 || len(f.Contexts) != 1 || f.Lorebook != nil || f.State.Lorebooks["a"].ID != "one" || f.State.Lorebooks["b"].ID != "two" {
		t.Fatalf("incomplete pull: %+v", f)
	}
	if f.Lorebooks["a"].MainRecall == nil || *f.Lorebooks["a"].MainRecall || f.Lorebooks["a"].Entries[0].SemanticRecall == nil || *f.Lorebooks["a"].Entries[0].SemanticRecall {
		t.Fatal("binding or entry options lost")
	}
	absent = true
	if _, err := Pull(context.Background(), c, "role", dir, PullOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Contexts) != 0 || f.ContextsPresent || len(f.Lorebooks) != 0 || p.Card["contexts"] != nil {
		t.Fatal("absent declaration or book resurrected from stale files")
	}
}
