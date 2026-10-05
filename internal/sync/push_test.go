package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

// The server checks trial-card field limits against the card's language, the
// same way an owned card is checked later, so a trial push must say which
// language the folder declares.
func TestTrialPushSendsTheCardLanguage(t *testing.T) {
	for _, tc := range []struct{ manifest, header string }{{"en", "en"}, {"", ""}} {
		var got string
		var seen bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut {
				got, seen = r.Header.Get("language"), true
			}
			json.NewEncoder(w).Encode(map[string]any{"clientKey": "k", "roleId": "r", "created": true, "sections": map[string]string{}})
		}))
		f, err := card.Init(t.TempDir(), "Language")
		if err != nil {
			t.Fatal(err)
		}
		f.Manifest.Language = tc.manifest
		c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
		if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
			t.Fatal(err)
		}
		srv.Close()
		if !seen || got != tc.header {
			t.Fatalf("manifest language %q: sent %q (seen=%v), want %q", tc.manifest, got, seen, tc.header)
		}
	}
}

// The trial contract ignores image fields, so push sets them on the trial role
// through the document route afterwards. The landscape background has to go
// with the portrait ones, or wide screens fall back to the portrait image.
func TestTrialPushSetsTheLandscapeBackground(t *testing.T) {
	var fields map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/open/v1/role/r/document" {
			var body struct{ Fields map[string]any }
			json.NewDecoder(r.Body).Decode(&body)
			fields = body.Fields
			w.WriteHeader(http.StatusOK)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"clientKey": "k", "roleId": "r", "created": true, "sections": map[string]string{}})
	}))
	defer srv.Close()
	f, err := card.Init(t.TempDir(), "Landscape")
	if err != nil {
		t.Fatal(err)
	}
	f.Manifest.Media.Background = "https://cdn/tall.png"
	f.Manifest.Media.BackgroundLandscape = "https://cdn/wide.png"
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	if fields["roleBackground"] != "https://cdn/tall.png" || fields["roleBackgroundLandscape"] != "https://cdn/wide.png" {
		t.Fatalf("document fields = %v, want both backgrounds", fields)
	}
}
