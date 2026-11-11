package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCardRevertNeedsExplicitSave(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv("HEARTHROOM_TOKEN", "fixture-token")
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open/v1/role/source/approved" {
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if r.Method == "POST" {
			writes++
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["approvalRevision"] != "approval" || body["policyRevision"] != "draft" || body["operationId"] != "operation" {
				t.Error("lost save preconditions")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"approvedRoleId":"frozen","approvalRevision":"approval","policyRevision":"draft","operationId":"operation"}`))
	}))
	defer srv.Close()
	for _, confirm := range []bool{false, true} {
		args := []string{"card", "revert", "source", "--json", "--api", srv.URL, "--config-dir", cfg}
		if confirm {
			args = append(args, "--confirm")
		}
		stdout, _, err := runCLI(t, args...)
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			Saved bool `json:"saved"`
		}
		if err = json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatal(err)
		}
		if out.Saved != confirm {
			t.Errorf("saved=%v want %v", out.Saved, confirm)
		}
	}
	if writes != 1 {
		t.Fatalf("preview wrote or confirmed save repeated: %d", writes)
	}
}

func TestCardRevertDoesNotClaimAnUnknownSaveLeftDraftUnchanged(t *testing.T) {
	for _, tc := range []struct {
		status  int
		confirm bool
		want    string
	}{{503, false, "temporarily unavailable"}, {503, true, "save result could not be confirmed"}, {409, true, "status has changed"}, {403, true, "permission"}} {
		t.Run(fmt.Sprintf("%d-%v", tc.status, tc.confirm), func(t *testing.T) {
			cfg := authTestEnv(t)
			t.Setenv("HEARTHROOM_TOKEN", "fixture-token")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "POST" || !tc.confirm {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(`{"error":"fixture"}`))
					return
				}
				_, _ = w.Write([]byte(`{"approvedRoleId":"frozen","approvalRevision":"approval","policyRevision":"draft","operationId":"operation"}`))
			}))
			defer srv.Close()
			args := []string{"card", "revert", "source", "--api", srv.URL, "--config-dir", cfg}
			if tc.confirm {
				args = append(args, "--confirm")
			}
			_, _, err := runCLI(t, args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wrong failure: %v", err)
			}
			if tc.confirm && tc.status >= 500 && strings.Contains(err.Error(), "unchanged") {
				t.Fatal("unknown save claimed unchanged draft")
			}
		})
	}
}
