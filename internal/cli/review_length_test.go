package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/auth"
	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/output"
)

func TestSubmitLengthRefusalExplainsCountsAndPreservesJSON(t *testing.T) {
	for _, jsonMode := range []bool{false, true} {
		t.Run(map[bool]string{false: "human", true: "json"}[jsonMode], func(t *testing.T) {
			cfg := authTestEnv(t)
			t.Setenv(auth.EnvToken, "tok")
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/cards" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				calls++
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"error":"review_length_insufficient","detail":{"reviewLength":{"persona":1800,"constantWorldbook":1000,"total":2800,"required":3001,"remaining":201,"eligible":false,"welcome":120}}}`))
			}))
			defer srv.Close()
			dir := ratingFolder(t, card.State{Target: "owned", RoleID: "role-1"})
			if err := card.WriteRating(dir, &card.Rating{Answers: card.RatingAnswers{Version: 1, Other: "other.none"}, Rating: "G"}); err != nil {
				t.Fatal(err)
			}
			args := []string{"card", "submit", dir, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg}
			if jsonMode {
				args = append(args, "--json")
			}
			_, _, err := runCLI(t, args...)
			if err == nil {
				t.Fatal("short card was reported as submitted")
			}
			var stdout, stderr bytes.Buffer
			printer := output.Printer{JSON: jsonMode, Out: &stdout, Err: &stderr}
			if code := printer.Fail(err); code != 2 {
				t.Fatalf("exit code = %d, want 2; error = %v", code, err)
			}
			if jsonMode {
				var got struct {
					Detail struct {
						Code   string `json:"code"`
						Detail struct {
							ReviewLength map[string]any `json:"reviewLength"`
						} `json:"detail"`
					} `json:"detail"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				length := got.Detail.Detail.ReviewLength
				if got.Detail.Code != "review_length_insufficient" || length["total"] != float64(2800) || length["remaining"] != float64(201) || length["eligible"] != false || length["welcome"] != float64(120) {
					t.Fatalf("lost server detail: %s", stdout.String())
				}
			} else {
				for _, want := range []string{"2800", "3001", "201", "character definition", "enabled always-on Lorebook", "push"} {
					if !strings.Contains(stderr.String(), want) {
						t.Errorf("human error missing %q: %s", want, stderr.String())
					}
				}
			}
			if calls != 1 {
				t.Fatalf("requests = %d, want one refusal without retry", calls)
			}
			f, err := card.Load(dir)
			if err != nil || f.State.Submission == nil {
				t.Fatalf("submission operation lost: %v", err)
			}
		})
	}
}
