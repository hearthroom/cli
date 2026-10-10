package sync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

// The server checks trial-card field limits against the card's language, the
// same way an owned card is checked later, so a trial push must say which
// language the folder declares.
func TestTrialPushSendsTheCardLanguage(t *testing.T) {
	for _, tc := range []struct{ manifest, header string }{{"en", "en"}, {"zh-Hans", "zh-Hans"}} {
		var got string
		var seen bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut {
				got, seen = r.Header.Get("language"), true
			}
			json.NewEncoder(w).Encode(map[string]any{"clientKey": "k", "roleId": "r", "created": true, "sections": map[string]string{}})
		}))
		f, err := card.Init(t.TempDir(), "Language", "en")
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
	f, err := card.Init(t.TempDir(), "Landscape", "en")
	if err != nil {
		t.Fatal(err)
	}
	f.Manifest.Media.Background = "https://cdn/tall.png"
	f.Manifest.Media.BackgroundLandscape = "https://cdn/wide.png"
	f.Manifest.Media.Share = "https://cdn/share.png"
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	if fields["roleBackground"] != "https://cdn/tall.png" || fields["roleBackgroundLandscape"] != "https://cdn/wide.png" || fields["roleShareImage"] != "https://cdn/share.png" {
		t.Fatalf("document fields = %v, want both backgrounds and the share image", fields)
	}
}

// The provider files an undeclared card as zh-Hant, so a Simplified card
// pushed without a language would be shown to its players unconverted in
// the wrong script. Push refuses before anything is uploaded.
func TestPushRefusesAMissingOrUnsupportedLanguage(t *testing.T) {
	for _, lang := range []string{"", "zh", "zh-TW"} {
		called := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
		f, err := card.Init(t.TempDir(), "NoLanguage", "en")
		if err != nil {
			t.Fatal(err)
		}
		f.Manifest.Language = lang
		c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
		_, err = Push(context.Background(), c, f, PushOptions{SkipMedia: true})
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), "card.json") || !strings.Contains(err.Error(), "zh-Hant, zh-Hans") {
			t.Fatalf("language %q: err = %v, want a card.json language error", lang, err)
		}
		if called {
			t.Fatalf("language %q: pushed to the server anyway", lang)
		}
	}
}

// A folder pushed as a trial card remembers the section hashes it sent. A new
// owned card holds none of it, so --create has to write every section.
func TestCreateAfterTrialWritesEverySection(t *testing.T) {
	var wrote []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/open/v1/role":
			json.NewEncoder(w).Encode(map[string]any{"roleId": "owned1"})
		case r.Method == http.MethodPost && r.URL.Path == "/open/v1/role/owned1/document":
			wrote = append(wrote, card.SectionCard)
			w.Write([]byte("{}"))
		case r.Method == http.MethodPatch && r.URL.Path == "/open/v1/role/owned1/welcome":
			wrote = append(wrote, card.SectionWelcome)
			w.Write([]byte("{}"))
		default:
			w.Write([]byte("{}"))
		}
	}))
	defer srv.Close()
	f, err := card.Init(t.TempDir(), "Trial then owned", "en")
	if err != nil {
		t.Fatal(err)
	}
	f.Welcome = "Hello there."
	payload, err := f.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	f.State.Target = "trial"
	f.State.RoleID = "trial1"
	f.State.TrialKey = "k"
	f.State.LocalHashes = payload.Digests()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	res, err := Push(context.Background(), c, f, PushOptions{Create: true, SkipMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.RoleID != "owned1" || len(wrote) < 2 || wrote[0] != card.SectionCard || wrote[1] != card.SectionWelcome {
		t.Fatalf("role %s wrote %v unchanged %v", res.RoleID, wrote, res.Unchanged)
	}
}

// A share image that points at a missing file is caught before any upload.
func TestLocalCheckReportsAMissingShareImage(t *testing.T) {
	f, err := card.Init(t.TempDir(), "Share", "en")
	if err != nil {
		t.Fatal(err)
	}
	f.Manifest.Media.Share = "assets/share.png"
	found := false
	for _, p := range LocalCheck(f) {
		if strings.Contains(p, "assets/share.png") {
			found = true
		}
	}
	if !found {
		t.Fatalf("LocalCheck = %v, want the missing share image reported", LocalCheck(f))
	}
}

// An owned Chinese card can switch between zh-Hant and zh-Hans; push sends the
// folder's language with the card fields, and a language change alone is
// enough to resend them. Other languages are never sent (fixed after creation).
func TestOwnedPushSendsChineseScript(t *testing.T) {
	var sent []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/open/v1/role/owned1/document" {
			var body struct{ Fields map[string]any }
			json.NewDecoder(r.Body).Decode(&body)
			sent = append(sent, body.Fields["language"])
		}
		w.Write([]byte("{}"))
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	f, err := card.Init(t.TempDir(), "Script", "zh-Hant")
	if err != nil {
		t.Fatal(err)
	}
	f.State.Target, f.State.RoleID = "owned", "owned1"
	push := func() {
		t.Helper()
		if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
			t.Fatal(err)
		}
	}
	push()
	push() // nothing changed: no second document write
	f.Manifest.Language = "zh-Hans"
	push()
	if len(sent) != 2 || sent[0] != "zh-Hant" || sent[1] != "zh-Hans" {
		t.Fatalf("document language sent = %v", sent)
	}
	// card status compares the same digest, so a pushed folder reads as unchanged.
	built, err := f.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.State.LocalHashes[card.SectionCard] != OwnedDigest(f, built.Digests(), card.SectionCard) {
		t.Fatal("status would report the card section as changed right after a push")
	}

	sent = nil
	en, _ := card.Init(t.TempDir(), "English", "en")
	en.State.Target, en.State.RoleID = "owned", "owned1"
	if _, err := Push(context.Background(), c, en, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0] != nil {
		t.Fatalf("english card sent language %v", sent)
	}
}

// rating.json is a community-site submission attribute: the provider never
// receives it, on a trial push or an owned one.
func TestPushDoesNotSendTheRatingFile(t *testing.T) {
	for _, create := range []bool{false, true} {
		var bodies []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			bodies = append(bodies, r.Method+" "+r.URL.Path+" "+string(b))
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/open/v1/role":
				json.NewEncoder(w).Encode(map[string]any{"roleId": "owned1"})
			default:
				json.NewEncoder(w).Encode(map[string]any{"clientKey": "k", "roleId": "r", "created": true, "sections": map[string]string{}})
			}
		}))
		f, err := card.Init(t.TempDir(), "Rated", "en")
		if err != nil {
			t.Fatal(err)
		}
		f.Definition = "A keeper of the lamp."
		f.Welcome = "Hello."
		if err := card.WriteRating(f.Dir, &card.Rating{Answers: card.RatingAnswers{Version: 1, Topics: map[string]string{"violence": "violence.bloody"}, Other: "other.none"}, Rating: "PG15", Descriptors: []string{"violence"}}); err != nil {
			t.Fatal(err)
		}
		f, err = card.Load(f.Dir)
		if err != nil {
			t.Fatal(err)
		}
		c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
		if _, err := Push(context.Background(), c, f, PushOptions{Create: create}); err != nil {
			t.Fatal(err)
		}
		srv.Close()
		if len(bodies) == 0 {
			t.Fatal("nothing pushed")
		}
		for _, b := range bodies {
			for _, leak := range []string{"violence.bloody", "other.none", "PG15", card.RatingFile, "ratingAnswers"} {
				if strings.Contains(b, leak) {
					t.Fatalf("create=%v: request carries %q: %s", create, leak, b)
				}
			}
		}
	}
}
