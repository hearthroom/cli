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

// card state：作者看自己卡的共用狀態、開關、重設一個 key、清空草稿；都打社群站、都要登入。
func TestCardStateShowToggleReset(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	var calls []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/me/cards/role-1/shared-state", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "show")
		_, _ = w.Write([]byte(`{"platformEnabled":true,"authorOff":false,"platformOff":false,"declaration":{"polls":{"p1":{"options":["a","b"]}},"counters":{"boss":{"maxStep":5,"perMember":50}}},"live":{"p1":{"kind":"poll","counts":{"a":3,"b":1},"total":4},"boss":{"kind":"counter","total":120}},"draft":{}}`))
	})
	mux.HandleFunc("PUT /v1/me/cards/role-1/shared-state", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		calls = append(calls, "put:"+map[bool]string{true: "on", false: "off"}[body["enabled"] == true])
		_, _ = w.Write([]byte(`{"authorOff":` + map[bool]string{true: "false", false: "true"}[body["enabled"] == true] + `}`))
	})
	mux.HandleFunc("POST /v1/me/cards/role-1/shared-state/{key}/reset", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		calls = append(calls, "reset:"+r.PathValue("key")+":"+body["ns"].(string))
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("POST /v1/me/cards/role-1/shared-state/draft/clear", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "clear")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	dir := ratingFolder(t, card.State{Target: "owned", RoleID: "role-1", API: srv.URL})
	run := func(args ...string) string {
		t.Helper()
		argv := append([]string{"card", "state"}, args...)
		argv = append(argv, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
		out, _, err := runCLI(t, argv...)
		if err != nil {
			t.Fatalf("%v: %s %v", args, out, err)
		}
		return out
	}
	if out := run("show", dir); !strings.Contains(out, "p1") || !strings.Contains(out, "a 3") || !strings.Contains(out, "boss") || !strings.Contains(out, "120") {
		t.Fatalf("show = %s", out)
	}
	run("off", dir)
	run("on", dir)
	run("reset", "boss", dir)
	run("reset", "p1", dir, "--draft")
	run("clear-draft", dir)
	if got := strings.Join(calls, ","); got != "show,put:off,put:on,reset:boss:live,reset:p1:draft,clear" {
		t.Fatalf("calls = %s", got)
	}
}
