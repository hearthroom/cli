package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

func TestPushWelcomeClearAndAbsence(t *testing.T) {
	for _, target := range []string{"owned", "trial", "trial retry"} {
		for _, tc := range []struct {
			name string
			mask int
			mode string
			want []any
		}{
			{"retained remote page", 7, "", []any{"", []any{}, []any{}}},
			{"clear page", 7, "page", []any{"", []any{}, []any{}}},
			{"all absent", 0, "page", []any{"Old greeting", []any{"Old alternate"}, []any{"Old choice"}}},
			{"greeting only", 1, "page", []any{"", []any{"Old alternate"}, []any{"Old choice"}}},
			{"alternates only", 2, "page", []any{"Old greeting", []any{}, []any{"Old choice"}}},
			{"prologue only", 4, "page", []any{"Old greeting", []any{"Old alternate"}, []any{}}},
			{"protect greeting mode", 7, "greeting", []any{"Old greeting", []any{"Old alternate"}, []any{"Old choice"}}},
		} {
			if target != "owned" && tc.mode == "greeting" {
				continue
			} // owned-card guard; trials accept blank drafts.
			t.Run(target+"/"+tc.name, func(t *testing.T) {
				remote := map[string]any{"roleWelcome": "Old greeting", "alternates": []any{"Old alternate"}, "prologue": []any{"Old choice"}}
				mode := "page"
				patches := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.URL.Path == "/open/v1/trial-cards/k" && r.Method == http.MethodGet:
						json.NewEncoder(w).Encode(map[string]any{"roleId": "r"})
					case r.URL.Path == "/open/v1/role/detail":
						json.NewEncoder(w).Encode(map[string]any{"characterRoleId": "r", "language": "en", "roleWelcome": remote["roleWelcome"], "roleWelcomeAlternates": remote["alternates"], "rolePrologue": remote["prologue"]})
					case r.URL.Path == "/open/v1/role/author-asset":
						json.NewEncoder(w).Encode(map[string]any{"openingMode": mode, "rules": []any{}})
					case r.URL.Path == "/open/v1/role/r/author-asset" && r.Method == http.MethodPut:
						var body map[string]any
						json.NewDecoder(r.Body).Decode(&body)
						mode, _ = body["openingMode"].(string)
						json.NewEncoder(w).Encode(map[string]any{"version": 1})
					case r.URL.Path == "/open/v1/role/r/welcome":
						var body map[string]any
						json.NewDecoder(r.Body).Decode(&body)
						if body["roleWelcome"] == "" && mode != "page" {
							t.Error("welcome cleared before page mode was saved")
						}
						for k, v := range body {
							remote[k] = v
						}
						patches++
						w.Write([]byte(`{}`))
					case r.URL.Path == "/open/v1/trial-cards/k" && r.Method == http.MethodPut:
						var body map[string]any
						json.NewDecoder(r.Body).Decode(&body)
						// Trial PUT replaces the whole welcome, including omitted fields.
						remote = map[string]any{"roleWelcome": "", "alternates": []any{}, "prologue": []any{}}
						if welcome, ok := body["welcome"].(map[string]any); ok {
							for k, v := range welcome {
								remote[k] = v
							}
						}
						json.NewEncoder(w).Encode(map[string]any{"roleId": "r", "clientKey": "k", "sections": map[string]string{}})
					default:
						w.Write([]byte(`{}`))
					}
				}))
				defer srv.Close()
				dir := t.TempDir()
				manifest := `{"formatVersion":1,"name":"Synthetic","language":"en"`
				if tc.mask&4 != 0 {
					manifest += `,"prologue":[]`
				}
				os.WriteFile(filepath.Join(dir, card.ManifestFile), []byte(manifest+`}`), 0644)
				if tc.mode != "" {
					os.WriteFile(filepath.Join(dir, card.RulesFile), []byte(`{"rules":[],"openingMode":"`+tc.mode+`"}`), 0644)
				}
				if tc.mask&1 != 0 {
					os.WriteFile(filepath.Join(dir, card.WelcomeFile), nil, 0644)
				}
				if tc.mask&2 != 0 {
					os.Mkdir(filepath.Join(dir, card.OpeningsDir), 0755)
				}
				f, err := card.Load(dir)
				if err != nil {
					t.Fatal(err)
				}
				f.State = card.State{Target: target, RoleID: "r", TrialKey: "k"}
				if target == "trial retry" {
					f.State.Target = "trial"
					f.State.RoleID = ""
				}
				c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
				for attempt := 0; attempt < 2; attempt++ {
					if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
						t.Fatal(err)
					}
					got := []any{remote["roleWelcome"], remote["alternates"], remote["prologue"]}
					if !reflect.DeepEqual(got, tc.want) {
						t.Fatalf("remote = %#v, want %#v", got, tc.want)
					}
				}
				if target == "owned" && tc.mask != 0 && tc.mode == "page" && patches != 1 {
					t.Errorf("welcome patch count = %d, want 1", patches)
				}
			})
		}
	}
}

func TestTrialResumeRequiresAConfirmedRead(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantWrite bool
	}{
		{"new slot", 404, `{}`, true},
		{"unavailable", 503, `{"error":"temporarily_unavailable"}`, false},
		{"unidentified", 200, `{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/open/v1/trial-cards/k" {
					w.WriteHeader(tc.status)
					w.Write([]byte(tc.body))
					return
				}
				if r.Method != http.MethodGet {
					writes++
				}
				json.NewEncoder(w).Encode(map[string]any{"roleId": "r", "clientKey": "k", "sections": map[string]string{}})
			}))
			defer srv.Close()
			f, err := card.Init(t.TempDir(), "Synthetic retry", "en")
			if err != nil {
				t.Fatal(err)
			}
			f.State.TrialKey = "k"
			c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
			_, err = Push(context.Background(), c, f, PushOptions{SkipMedia: true})
			if tc.wantWrite {
				if err != nil || writes != 1 {
					t.Fatalf("new trial err=%v writes=%d", err, writes)
				}
			} else if err == nil || writes != 0 {
				t.Fatalf("uncertain read err=%v writes=%d", err, writes)
			}
		})
	}
}
