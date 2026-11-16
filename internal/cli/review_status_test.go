package cli

import (
	"encoding/json"
	"github.com/hearthroom/cli/internal/auth"
	"github.com/hearthroom/cli/internal/card"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReviewStatusAuthorRead(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/me/cards/role-1/review-status" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(403)
			return
		}
		_, _ = w.Write([]byte(`{"publicVersionId":"public-1","latestSubmission":{"id":"sub-1","versionId":"ver-1","submittedAt":1700000000000,"kind":"re","status":"rejected","rejectionReason":"Please add an opening","updateKind":"minor","silentUpdate":true},"reviewer":"private-reviewer","privateInstructions":"secret"}`))
	}))
	defer srv.Close()
	dir := ratingFolder(t, card.State{Target: "owned", RoleID: "role-1", API: srv.URL})
	for _, target := range [][]string{{dir}, {"--role", "role-1"}} {
		args := append([]string{"card", "review-status"}, target...)
		args = append(args, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
		out, _, err := runCLI(t, append(args, "--json")...)
		if err != nil {
			t.Fatal(err)
		}
		var result map[string]any
		if err = json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatal(err)
		}
		sub, _ := result["latestSubmission"].(map[string]any)
		if result["publicVersionId"] != "public-1" || sub["id"] != "sub-1" || sub["status"] != "rejected" || strings.Contains(out, "private-reviewer") || strings.Contains(out, "secret") {
			t.Fatalf("output: %s", out)
		}
		out, _, err = runCLI(t, args...)
		if err != nil || !strings.Contains(out, "Please add an opening") || !strings.Contains(out, "public-1") || !strings.Contains(out, "Silent update: true") {
			t.Fatalf("human output %s: %v", out, err)
		}
	}
}

func TestReviewStatusRejectsInvalidTargets(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	trial := ratingFolder(t, card.State{Target: "trial", RoleID: "trial-1"})
	wrong := ratingFolder(t, card.State{Target: "owned", RoleID: "role-1", API: "https://different.invalid"})
	for _, target := range [][]string{{trial}, {wrong}, {trial, "--role", "role-1"}, {"--role", ""}} {
		_, _, err := runCLI(t, append(append([]string{"card", "review-status"}, target...), "--config-dir", cfg)...)
		if err == nil {
			t.Fatalf("accepted invalid target %v", target)
		}
	}
}

func TestReviewStatusEmptyAndDenied(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	for _, status := range []int{200, 401, 403, 404} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				if status == 200 {
					_, _ = w.Write([]byte(`{"publicVersionId":null,"latestSubmission":null}`))
				} else {
					_, _ = w.Write([]byte(`{"error":"card_not_found"}`))
				}
			}))
			defer srv.Close()
			out, _, err := runCLI(t, "card", "review-status", "--role", "role-1", "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
			if status == 200 {
				if err != nil || !strings.Contains(out, "No review submissions") {
					t.Fatalf("%s %v", out, err)
				}
			} else if err == nil {
				t.Fatal("denial accepted")
			}
		})
	}
}
