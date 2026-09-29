package sync_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/sync"
)

func TestRenderCallsTheProviderAndFillsDefaults(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"roleId":"r1","opening":{"index":1,"count":2},"names":{"player":"You","character":"Mira"},
		  "source":"hi **x**","rendered":"hi <b>x</b>","authorAsset":{"version":3,"pageMode":"sandbox","mountLayer":"under","mountTrigger":"","cardFormat":"","rules":1,"enabled":1},
		  "rules":[{"id":"bold","name":"","status":"applied"}],
		  "report":{"chars":11,"estimatedTokens":4,"tags":{"b":1},"components":[],"scripts":0,"styles":0,"inlineHandlers":0,"externalUrls":[],"unsupported":null,"crossLineRules":false},
		  "warnings":null,"capture":{"serverCaptureAvailable":false,"recommended":"open the play page"}}`))
	}))
	defer srv.Close()
	c := api.New(srv.URL, "https://site.test", "test", func(context.Context) (string, error) { return "tok", nil })

	r, err := sync.Render(context.Background(), c, "r1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.URL.Path != "/open/v1/role/render" || got.URL.Query().Get("roleId") != "r1" || got.URL.Query().Get("opening") != "1" || got.Header.Get("Authorization") != "Bearer tok" {
		t.Errorf("request = %s %s %q", got.Method, got.URL, got.Header.Get("Authorization"))
	}
	if r.Rendered != "hi <b>x</b>" || r.AuthorAsset == nil || r.AuthorAsset.PageMode != "sandbox" || r.Opening.Count != 2 || r.Rules[0].Status != "applied" {
		t.Errorf("report = %+v", r)
	}
	if r.Warnings == nil || r.Report.Unsupported == nil {
		t.Errorf("null arrays must decode to empty slices for --json: %+v", r)
	}

	// opening 0 is the default and is not sent.
	if _, err := sync.Render(context.Background(), c, "r1", 0); err != nil {
		t.Fatal(err)
	}
	if got.URL.Query().Has("opening") {
		t.Errorf("opening=0 should not be sent: %s", got.URL)
	}
	if u := sync.PreviewURL("https://site.test", "r 1"); u != "https://site.test/play/r%201" {
		t.Errorf("preview url = %q", u)
	}
}

func TestRenderSurfacesProviderErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid_param","message":"opening out of range"}`))
	}))
	defer srv.Close()
	c := api.New(srv.URL, "https://site.test", "test", func(context.Context) (string, error) { return "tok", nil })
	if _, err := sync.Render(context.Background(), c, "r1", 9); err == nil {
		t.Fatal("expected an error")
	}
}
