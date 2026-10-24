package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/auth"
	"github.com/hearthroom/cli/internal/card"
)

func visibilityFixture(t *testing.T, post func(http.ResponseWriter, *http.Request), operation func(http.ResponseWriter, *http.Request)) (*httptest.Server, string, string) {
	t.Helper()
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /open/v1/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer other-token" {
			_, _ = w.Write([]byte(`{"accountNumId":43}`))
			return
		}
		_, _ = w.Write([]byte(`{"accountNumId":42}`))
	})
	mux.HandleFunc("GET /v1/me/cards/role-1/visibility", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Error("missing bearer")
		}
		_, _ = w.Write([]byte(`{"workVisibility":{"access":"private","syncState":"complete","policyRevision":"p1","approvalRevision":"a1","actions":["set_private","set_unlisted","restore_public"]}}`))
	})
	mux.HandleFunc("POST /v1/me/cards/role-1/visibility", post)
	mux.HandleFunc("GET /v1/me/cards/role-1/visibility/operations/{op}", operation)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	dir := ratingFolder(t, card.State{Target: "owned", RoleID: "role-1", API: srv.URL})
	return srv, dir, cfg
}
func visibilityRun(t *testing.T, srv *httptest.Server, dir, cfg string, args ...string) (string, error) {
	t.Helper()
	argv := append([]string{"card", "visibility"}, args...)
	argv = append(argv, dir, "--json", "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
	out, _, err := runCLI(t, argv...)
	return out, err
}
func TestVisibilityGetDoesNotWrite(t *testing.T) {
	fail := func(http.ResponseWriter, *http.Request) { t.Error("GET mutated") }
	srv, dir, cfg := visibilityFixture(t, fail, fail)
	out, err := visibilityRun(t, srv, dir, cfg, "get")
	if err != nil || !strings.Contains(out, `"access": "private"`) {
		t.Fatalf("%s %v", out, err)
	}
}
func TestVisibilitySetSendsCASAndCheckpointsBeforePost(t *testing.T) {
	for _, access := range []string{"private", "unlisted", "public"} {
		t.Run(access, func(t *testing.T) {
			var dir string
			srv, d, cfg := visibilityFixture(t, func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				f, e := card.Load(dir)
				if e != nil {
					t.Fatal(e)
				}
				encoded, _ := json.Marshal(f.State)
				if !strings.Contains(string(encoded), body["operationId"].(string)) {
					t.Error("operation was not checkpointed before POST")
				}
				if body["targetAccess"] != access || body["expectedPolicyRevision"] != "p1" || body["expectedApprovalRevision"] != "a1" {
					t.Errorf("body=%v", body)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"workVisibility": map[string]any{"access": access, "syncState": "complete"}, "operation": map[string]any{"operationId": body["operationId"], "state": "complete"}})
			}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected operation GET") })
			dir = d
			out, err := visibilityRun(t, srv, dir, cfg, "set", access)
			if err != nil || !strings.Contains(out, `"state": "complete"`) {
				t.Fatalf("%s %v", out, err)
			}
		})
	}
}
func TestVisibilityPendingReadsBackWithoutNewMutation(t *testing.T) {
	posts, reads := 0, 0
	var id string
	srv, dir, cfg := visibilityFixture(t, func(w http.ResponseWriter, r *http.Request) {
		posts++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		id = body["operationId"].(string)
		w.WriteHeader(202)
		_ = json.NewEncoder(w).Encode(map[string]any{"operation": map[string]any{"operationId": id, "state": "pending"}})
	}, func(w http.ResponseWriter, r *http.Request) {
		reads++
		if r.PathValue("op") != id {
			t.Error("changed key")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"workVisibility": map[string]any{"access": "unlisted", "syncState": "complete"}, "operation": map[string]any{"operationId": id, "state": "complete"}})
	})
	out, err := visibilityRun(t, srv, dir, cfg, "set", "unlisted")
	if err == nil || !strings.Contains(out, `"state": "pending"`) {
		t.Fatalf("pending claimed success: %s %v", out, err)
	}
	out, err = visibilityRun(t, srv, dir, cfg, "get")
	if err != nil || posts != 1 || reads != 1 || !strings.Contains(out, `"state": "complete"`) {
		t.Fatalf("posts %d reads %d: %s %v", posts, reads, out, err)
	}
}
func TestVisibilityUnknownResumesOriginalIntentAfterReadback404(t *testing.T) {
	posts := 0
	var id string
	srv, dir, cfg := visibilityFixture(t, func(w http.ResponseWriter, r *http.Request) {
		posts++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if posts == 1 {
			id = body["operationId"].(string)
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"error":"visibility_sync_pending"}`))
			return
		}
		if body["operationId"] != id || body["expectedPolicyRevision"] != "p1" {
			t.Errorf("replaced intent: %v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"workVisibility": map[string]any{"access": "unlisted", "syncState": "complete"}, "operation": map[string]any{"operationId": id, "state": "complete"}})
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"visibility_operation_not_found"}`))
	})
	_, err := visibilityRun(t, srv, dir, cfg, "set", "unlisted")
	if err == nil {
		t.Fatal("503 claimed success")
	}
	_, err = visibilityRun(t, srv, dir, cfg, "set", "public")
	if err == nil || posts != 1 {
		t.Fatal("unresolved intent was replaced")
	}
	_, err = visibilityRun(t, srv, dir, cfg, "set", "unlisted")
	if err != nil || posts != 2 {
		t.Fatalf("posts=%d %v", posts, err)
	}
}
func TestVisibilityPublicReviewRequiredDoesNotSubmit(t *testing.T) {
	srv, dir, cfg := visibilityFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":"review_required"}`))
	}, func(http.ResponseWriter, *http.Request) { t.Error("definitive rejection read as unknown") })
	_, err := visibilityRun(t, srv, dir, cfg, "set", "public")
	if err == nil || !strings.Contains(err.Error(), "review_required") {
		t.Fatalf("%v", err)
	}
}
func TestVisibilityRejectsInvalidTargetBeforeNetwork(t *testing.T) {
	srv, dir, cfg := visibilityFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid target sent") }, func(http.ResponseWriter, *http.Request) { t.Error("invalid target sent") })
	_, err := visibilityRun(t, srv, dir, cfg, "set", "friends")
	if err == nil {
		t.Fatal("invalid enum accepted")
	}
}

func TestVisibilityUncertainOperationCannotReplayAsAnotherAccount(t *testing.T) {
	posts, reads := 0, 0
	srv, dir, cfg := visibilityFixture(t, func(w http.ResponseWriter, r *http.Request) {
		posts++
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":"temporarily_unavailable"}`))
	}, func(w http.ResponseWriter, r *http.Request) { reads++; w.WriteHeader(404) })
	_, err := visibilityRun(t, srv, dir, cfg, "set", "unlisted")
	if err == nil {
		t.Fatal("unknown result claimed success")
	}
	t.Setenv(auth.EnvToken, "other-token")
	_, err = visibilityRun(t, srv, dir, cfg, "set", "unlisted")
	if err == nil || posts != 1 || reads != 0 {
		t.Fatalf("cross-account replay: posts=%d reads=%d err=%v", posts, reads, err)
	}
}

func TestVisibilityWrongReceiptDoesNotClearCheckpoint(t *testing.T) {
	srv, dir, cfg := visibilityFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"workVisibility":{"access":"unlisted","syncState":"complete"},"operation":{"operationId":"wrong","state":"complete"}}`))
	}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":"visibility_operation_conflict"}`))
	})
	for _, args := range [][]string{{"set", "unlisted"}, {"get"}} {
		out, err := visibilityRun(t, srv, dir, cfg, args...)
		if err == nil || !strings.Contains(out, "visibility_unconfirmed") {
			t.Fatalf("receipt accepted: %s %v", out, err)
		}
		f, e := card.Load(dir)
		if e != nil || f.State.VisibilityOperation == nil {
			t.Fatalf("checkpoint lost: %v", e)
		}
	}
}
