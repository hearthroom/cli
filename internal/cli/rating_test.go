package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hearthroom/cli/internal/auth"
	"github.com/hearthroom/cli/internal/card"
)

// ratingSite fakes the community site's rating endpoints and POST /v1/cards.
// cardReplies is consumed in order, one status per submit; the last repeats.
type ratingSite struct {
	mu          sync.Mutex
	evaluated   int
	drafts      []map[string]any
	submits     []map[string]any
	auths       []string
	cardReplies []int
}

func (s *ratingSite) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/rating/questionnaire", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": 1, "locale": r.URL.Query().Get("locale"), "intro": "Answer for the card.", "pickRule": "Pick the closest option that is not milder.",
			"ratings": []map[string]string{{"id": "G", "name": "General"}, {"id": "PG15", "name": "15+"}},
			"topics": []map[string]any{
				{"id": "sex", "title": "Sexual content", "options": []map[string]string{{"id": "sex.kiss", "rating": "P", "text": "Kissing"}, {"id": "sex.explicit", "rating": "R", "text": "Explicit"}}},
				{"id": "violence", "title": "Violence", "options": []map[string]string{{"id": "violence.mild", "rating": "P", "text": "Mild"}, {"id": "violence.bloody", "rating": "PG15", "text": "Bloody"}}},
			},
			"other": map[string]any{"title": "Anything else?", "options": []map[string]string{{"id": "other.none", "rating": "G", "text": "No"}}},
		})
	})
	evaluate := func(body map[string]any) (map[string]any, bool) {
		if v, _ := body["version"].(float64); v != 1 {
			return nil, false
		}
		if o, _ := body["other"].(string); o == "" {
			return nil, false
		}
		topics, _ := body["topics"].(map[string]any)
		rating, desc := "G", []string{}
		for k, v := range topics {
			desc = append(desc, k)
			if v == "violence.bloody" {
				rating = "PG15"
			}
		}
		return map[string]any{"rating": rating, "descriptors": desc, "answers": body}, true
	}
	mux.HandleFunc("POST /v1/rating/evaluate", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		s.evaluated++
		s.mu.Unlock()
		out, ok := evaluate(body)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"rating_version_outdated"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("PUT /v1/cards/{roleId}/rating-draft", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		s.drafts = append(s.drafts, body)
		s.mu.Unlock()
		out, _ := evaluate(body)
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("POST /v1/cards", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		s.submits = append(s.submits, body)
		s.auths = append(s.auths, r.Header.Get("Authorization"))
		status := http.StatusCreated
		if n := len(s.submits) - 1; len(s.cardReplies) > 0 {
			status = s.cardReplies[min(n, len(s.cardReplies)-1)]
		}
		s.mu.Unlock()
		w.WriteHeader(status)
		if status >= 300 {
			_, _ = w.Write([]byte(`{"error":"upstream_error"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": body["roleId"], "status": "pending", "versionId": "ver-1"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func ratingFolder(t *testing.T, state card.State) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "card")
	f, err := card.Init(dir, "Mira", "en")
	if err != nil {
		t.Fatal(err)
	}
	f.State = state
	if err := f.SaveState(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRateAnswersWritesRatingFile(t *testing.T) {
	cfg := authTestEnv(t)
	site := &ratingSite{}
	srv := site.server(t)
	dir := ratingFolder(t, card.State{})
	answers := filepath.Join(t.TempDir(), "answers.json")
	writeFile(t, answers, `{"version":1,"topics":{"violence":"violence.bloody"},"other":"other.none"}`)

	stdout, _, err := runCLI(t, "card", "rate", dir, "--answers", answers, "--json", "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if site.evaluated != 1 {
		t.Fatalf("evaluate called %d times", site.evaluated)
	}
	var out struct {
		Rating      string             `json:"rating"`
		Descriptors []string           `json:"descriptors"`
		Answers     card.RatingAnswers `json:"answers"`
		File        string             `json:"file"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout)
	}
	if out.Rating != "PG15" || out.File != filepath.Join(dir, card.RatingFile) || out.Answers.Topics["violence"] != "violence.bloody" {
		t.Fatalf("output = %+v", out)
	}
	r, err := card.ReadRating(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Rating != "PG15" || r.Answers.Version != 1 || r.Answers.Other != "other.none" || len(r.Descriptors) != 1 {
		t.Fatalf("rating.json = %+v", r)
	}
	if len(site.drafts) != 0 {
		t.Fatalf("an unlinked folder saved a draft: %v", site.drafts)
	}
}

// A folder linked to an owned card also keeps the answers as the site's draft,
// so the website's submit form starts from them.
func TestRateSavesTheDraftForAnOwnedCard(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	site := &ratingSite{}
	srv := site.server(t)
	dir := ratingFolder(t, card.State{Target: "owned", RoleID: "role-1"})
	answers := filepath.Join(t.TempDir(), "answers.json")
	writeFile(t, answers, `{"version":1,"topics":{},"other":"other.none"}`)
	if _, _, err := runCLI(t, "card", "rate", dir, "--answers", answers, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg); err != nil {
		t.Fatal(err)
	}
	if len(site.drafts) != 1 || site.drafts[0]["other"] != "other.none" {
		t.Fatalf("drafts = %v", site.drafts)
	}
}

func TestRateRejectedAnswersWriteNothing(t *testing.T) {
	cfg := authTestEnv(t)
	srv := (&ratingSite{}).server(t)
	dir := ratingFolder(t, card.State{})
	answers := filepath.Join(t.TempDir(), "answers.json")
	writeFile(t, answers, `{"version":0,"topics":{},"other":"other.none"}`)
	_, _, err := runCLI(t, "card", "rate", dir, "--answers", answers, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
	if err == nil || !strings.Contains(err.Error(), "--questions") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, card.RatingFile)); !os.IsNotExist(err) {
		t.Fatalf("rating.json written for rejected answers: %v", err)
	}
}

func TestRateWithoutTerminalNeedsFlags(t *testing.T) {
	cfg := authTestEnv(t)
	srv := (&ratingSite{}).server(t)
	dir := ratingFolder(t, card.State{})
	_, _, err := runCLI(t, "card", "rate", dir, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
	if err == nil || !strings.Contains(err.Error(), "--questions --json") || !strings.Contains(err.Error(), "--answers") {
		t.Fatalf("err = %v", err)
	}
}

func TestRateQuestionsJSON(t *testing.T) {
	cfg := authTestEnv(t)
	srv := (&ratingSite{}).server(t)
	stdout, _, err := runCLI(t, "card", "rate", "--questions", "--locale", "ja", "--json", "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var q map[string]any
	if err := json.Unmarshal([]byte(stdout), &q); err != nil || q["locale"] != "ja" || q["topics"] == nil {
		t.Fatalf("questions = %v (%v)", stdout, err)
	}
}

func TestRateInteractive(t *testing.T) {
	cfg := authTestEnv(t)
	site := &ratingSite{}
	srv := site.server(t)
	dir := ratingFolder(t, card.State{})
	app, out, _ := testApp()
	app.In = strings.NewReader("2\n2\n1\n")
	app.interactive = func() bool { return true }
	root := app.rootCommand()
	root.SetArgs([]string{"card", "rate", dir, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	r, err := card.ReadRating(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Rating != "PG15" || r.Answers.Topics["violence"] != "violence.bloody" || len(r.Answers.Topics) != 1 {
		t.Fatalf("rating.json = %+v", r)
	}
	if !strings.Contains(out.String(), "PG15") {
		t.Fatalf("output: %s", out.String())
	}
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestSubmitRetriesWithTheSameOperationID(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	site := &ratingSite{cardReplies: []int{http.StatusBadGateway, http.StatusCreated}}
	srv := site.server(t)
	dir := ratingFolder(t, card.State{Target: "owned", RoleID: "role-1"})
	if err := card.WriteRating(dir, &card.Rating{Answers: card.RatingAnswers{Version: 1, Topics: map[string]string{"sex": "sex.kiss"}, Other: "other.none"}, Rating: "P"}); err != nil {
		t.Fatal(err)
	}
	args := []string{"card", "submit", dir, "--fandom", "Original", "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg}
	if _, _, err := runCLI(t, args...); err == nil {
		t.Fatal("first submit should fail")
	}
	f, _ := card.Load(dir)
	if f.State.Submission == nil || f.State.Submission.RoleID != "role-1" {
		t.Fatalf("operation not kept after a failure: %+v", f.State.Submission)
	}
	stdout, _, err := runCLI(t, append(args, "--json")...)
	if err != nil {
		t.Fatal(err)
	}
	if len(site.submits) != 2 {
		t.Fatalf("submits = %d", len(site.submits))
	}
	first, second := site.submits[0], site.submits[1]
	op, _ := first["operationId"].(string)
	if !uuidPattern.MatchString(op) || second["operationId"] != op {
		t.Fatalf("operation ids %v then %v", first["operationId"], second["operationId"])
	}
	if first["roleId"] != "role-1" || first["fandom"] != "Original" || site.auths[0] != "Bearer tok" {
		t.Fatalf("body = %v, auth %q", first, site.auths[0])
	}
	answers, _ := first["ratingAnswers"].(map[string]any)
	topics, _ := answers["topics"].(map[string]any)
	if answers["version"] != float64(1) || answers["other"] != "other.none" || topics["sex"] != "sex.kiss" {
		t.Fatalf("ratingAnswers = %v", first["ratingAnswers"])
	}
	if !strings.Contains(stdout, `"versionId": "ver-1"`) {
		t.Fatalf("output: %s", stdout)
	}
	f, _ = card.Load(dir)
	if f.State.Submission != nil {
		t.Fatalf("operation kept after success: %+v", f.State.Submission)
	}
	// A later submission is a new operation.
	if _, _, err := runCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	if site.submits[2]["operationId"] == op {
		t.Fatal("a new submission reused the confirmed operation id")
	}
}

func TestSubmitWithoutRatingFile(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	site := &ratingSite{}
	srv := site.server(t)
	dir := ratingFolder(t, card.State{Target: "owned", RoleID: "role-1"})
	_, _, err := runCLI(t, "card", "submit", dir, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
	if err == nil || !strings.Contains(err.Error(), "hearthroom card rate") {
		t.Fatalf("err = %v", err)
	}
	if len(site.submits) != 0 {
		t.Fatal("submitted without a rating")
	}
}

func TestSubmitTrialCard(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	site := &ratingSite{}
	srv := site.server(t)
	dir := ratingFolder(t, card.State{Target: "trial", RoleID: "trial-1"})
	_ = card.WriteRating(dir, &card.Rating{Answers: card.RatingAnswers{Version: 1, Other: "other.none"}})
	_, _, err := runCLI(t, "card", "submit", dir, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
	if err == nil || !strings.Contains(err.Error(), "card push --create") {
		t.Fatalf("err = %v", err)
	}
	if len(site.submits) != 0 {
		t.Fatal("submitted a trial card")
	}
}
