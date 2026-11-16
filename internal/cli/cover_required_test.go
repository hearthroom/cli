package cli

import (
	"github.com/hearthroom/cli/internal/auth"
	"github.com/hearthroom/cli/internal/card"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubmitOpeningPreflight(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"error":"publication_fields_required"}`))
	}))
	defer srv.Close()
	dir := ratingFolder(t, card.State{Target: "owned", RoleID: "opening-test"})
	if err := card.WriteRating(dir, &card.Rating{Answers: card.RatingAnswers{Version: 1, Other: "other.none"}, Rating: "G"}); err != nil {
		t.Fatal(err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	f.Welcome = "<!-- Write an opening -->"
	if err = f.Save(); err != nil {
		t.Fatal(err)
	}
	_, _, err = runCLI(t, "card", "submit", dir, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
	if err == nil || !strings.Contains(err.Error(), "opening") || calls != 0 {
		t.Fatalf("missing opening err=%v requests=%d", err, calls)
	}
}

func TestSubmitOpeningFormats(t *testing.T) {
	page := "<p>At the harbor</p>"
	css := ".canvas-root { color: inherit; }"
	cases := []struct {
		name   string
		folder card.Folder
		want   bool
	}{
		{"greeting", card.Folder{Welcome: "Welcome"}, true},
		{"alternate", card.Folder{Alternates: []card.Opening{{Text: "Welcome"}}}, true},
		{"legacy choices", card.Folder{Manifest: card.Manifest{Prologue: []string{"Enter"}}}, true},
		{"page", card.Folder{OpeningPage: &page, Rules: &card.Rules{}}, true},
		{"page with card.css", card.Folder{OpeningPage: &page, CardCSS: &css}, true},
		{"unsent page", card.Folder{OpeningPage: &page}, false},
		{"empty mode", card.Folder{Rules: &card.Rules{OpeningMode: "page"}}, false},
		{"placeholder", card.Folder{Welcome: "<!-- Write an opening -->"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasSubmitOpening(&tc.folder); got != tc.want {
				t.Fatalf("has opening=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestSubmitRejectsUnsentOpeningPage(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(422) }))
	defer srv.Close()
	dir := ratingFolder(t, card.State{Target: "owned", RoleID: "opening-page"})
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	page := "<p>At the harbor</p>"
	f.Welcome = ""
	f.OpeningPage = &page
	f.Rules = nil
	if err = f.Save(); err != nil {
		t.Fatal(err)
	}
	if err = card.WriteRating(dir, &card.Rating{Answers: card.RatingAnswers{Version: 1, Other: "other.none"}, Rating: "G"}); err != nil {
		t.Fatal(err)
	}
	_, _, err = runCLI(t, "card", "submit", dir, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
	if err == nil || calls != 0 || !strings.Contains(err.Error(), "rules.json") {
		t.Fatalf("unsent opening page err=%v requests=%d", err, calls)
	}
}

func TestSubmitCoverPreflightAndServerRefusal(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"error":"cover_required","detail":{"field":"roleBackground"}}`))
	}))
	defer srv.Close()
	dir := ratingFolder(t, card.State{Target: "owned", RoleID: "cover-test"})
	if err := card.WriteRating(dir, &card.Rating{Answers: card.RatingAnswers{Version: 1, Other: "other.none"}, Rating: "G"}); err != nil {
		t.Fatal(err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	f.Manifest.Media.Portrait = ""
	f.Manifest.Media.Background = ""
	f.Manifest.Media.BackgroundLandscape = "https://images.test/landscape.png"
	if err = f.Save(); err != nil {
		t.Fatal(err)
	}
	args := []string{"card", "submit", dir, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg}
	_, _, err = runCLI(t, args...)
	if err == nil || !strings.Contains(err.Error(), "media.portrait") || calls != 0 {
		t.Fatalf("local cover refusal err=%v requests=%d", err, calls)
	}
	for _, slot := range []string{"portrait", "background"} {
		f, err = card.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		f.Manifest.Media.Portrait = ""
		f.Manifest.Media.Background = ""
		if slot == "portrait" {
			f.Manifest.Media.Portrait = "https://images.test/cover.png"
		} else {
			f.Manifest.Media.Background = "https://images.test/cover.png"
		}
		if err = f.Save(); err != nil {
			t.Fatal(err)
		}
		before := calls
		_, _, err = runCLI(t, args...)
		if err == nil || !strings.Contains(err.Error(), "cover_required") || !strings.Contains(err.Error(), "media.portrait") || calls != before+1 {
			t.Fatalf("%s server refusal err=%v requests=%d", slot, err, calls-before)
		}
	}
}
