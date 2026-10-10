package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hearthroom/cli/internal/auth"
	"github.com/hearthroom/cli/internal/config"
	"github.com/hearthroom/cli/internal/output"
)

// codeProvider is a fake provider that offers sign-in with a code and
// approves on the first poll, plus /open/v1/me for the signed-in account.
func codeProvider(t *testing.T, offerDevice bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		meta := map[string]any{"issuer": base, "authorization_endpoint": base + "/oauth/authorize",
			"token_endpoint": base + "/oauth/token", "registration_endpoint": base + "/oauth/register"}
		if offerDevice {
			meta["device_authorization_endpoint"] = base + "/oauth/device_authorization"
		}
		_ = json.NewEncoder(w).Encode(meta)
	})
	mux.HandleFunc("/oauth/register", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "hh_client_cli"})
	})
	mux.HandleFunc("/oauth/device_authorization", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code": "hh_dc_secret", "user_code": "BCDF-GHJK",
			"verification_uri":          "https://console.example.test/device",
			"verification_uri_complete": "https://console.example.test/device?user_code=BCDF-GHJK",
			"expires_in":                900, "interval": 5,
		})
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("grant_type") != auth.DeviceGrantType || r.PostForm.Get("device_code") != "hh_dc_secret" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at_device", "refresh_token": "rt_device",
			"expires_in": 3600, "scope": auth.Scopes, "token_type": "Bearer"})
	})
	mux.HandleFunc("/open/v1/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at_device" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"nickName": "Tester", "email": "tester@example.test"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// runCLI executes the command tree with captured output.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	app, out, errOut := testApp()
	root := app.rootCommand()
	root.SetArgs(args)
	err = root.ExecuteContext(context.Background())
	return out.String(), errOut.String(), err
}

// testApp builds an App with captured output and no terminal on stdin.
func testApp() (*App, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	app := &App{Info: BuildInfo{Version: "dev"}, Out: output.Printer{Out: &out, Err: &errOut}, In: strings.NewReader("")}
	return app, &out, &errOut
}

func authTestEnv(t *testing.T) string {
	t.Helper()
	t.Setenv(auth.EnvToken, "")
	for _, k := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		t.Setenv(k, "")
	}
	return t.TempDir()
}

func TestLoginNoWaitThenResume(t *testing.T) {
	dir := authTestEnv(t)
	srv := codeProvider(t, true)

	stdout, _, err := runCLI(t, "auth", "login", "--no-wait", "--json", "--api", srv.URL, "--config-dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	var started map[string]any
	if err := json.Unmarshal([]byte(stdout), &started); err != nil {
		t.Fatalf("--no-wait --json is not one JSON object: %v\n%s", err, stdout)
	}
	want := map[string]any{
		"user_code": "BCDF-GHJK", "verification_uri": "https://console.example.test/device",
		"expires_in": float64(900),
		"interval":   float64(5),
	}
	if len(started) != len(want) {
		t.Fatalf("keys = %v", started)
	}
	for k, v := range want {
		if started[k] != v {
			t.Fatalf("%s = %v, want %v", k, started[k], v)
		}
	}
	pendingPath := filepath.Join(dir, "pending-login.json")
	info, err := os.Stat(pendingPath)
	if err != nil {
		t.Fatalf("pending request not stored: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("pending mode = %o", info.Mode().Perm())
	}

	stdout, _, err = runCLI(t, "auth", "login", "--resume", "--json", "--api", srv.URL, "--config-dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	var done struct {
		API   string `json:"api"`
		Scope string `json:"scope"`
		Me    struct {
			Email string `json:"email"`
		} `json:"me"`
	}
	if err := json.Unmarshal([]byte(stdout), &done); err != nil {
		t.Fatalf("--resume --json: %v\n%s", err, stdout)
	}
	if done.API != srv.URL || done.Me.Email != "tester@example.test" || done.Scope != auth.Scopes {
		t.Fatalf("resume result = %+v", done)
	}
	if _, err := os.Stat(pendingPath); !os.IsNotExist(err) {
		t.Fatalf("pending request still present: %v", err)
	}
	creds, _ := (&config.Store{Dir: dir}).LoadCredentials()
	if creds[srv.URL].AccessToken != "at_device" || creds[srv.URL].ClientID != "hh_client_cli" {
		t.Fatalf("credential = %+v", creds[srv.URL])
	}
}

func TestLoginNoWaitHumanOutput(t *testing.T) {
	dir := authTestEnv(t)
	srv := codeProvider(t, true)
	stdout, _, err := runCLI(t, "auth", "login", "--no-wait", "--api", srv.URL, "--config-dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"BCDF-GHJK", "https://console.example.test/device", "hearthroom auth login --resume"} {
		if !strings.Contains(stdout, s) {
			t.Fatalf("missing %q in:\n%s", s, stdout)
		}
	}
}

func TestLoginNoWaitWithoutDeviceFlow(t *testing.T) {
	dir := authTestEnv(t)
	srv := codeProvider(t, false)
	_, _, err := runCLI(t, "auth", "login", "--no-wait", "--api", srv.URL, "--config-dir", dir)
	if err == nil || !strings.Contains(err.Error(), "--no-wait") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoginResumeErrors(t *testing.T) {
	srv := codeProvider(t, true)

	t.Run("nothing pending", func(t *testing.T) {
		dir := authTestEnv(t)
		_, _, err := runCLI(t, "auth", "login", "--resume", "--api", srv.URL, "--config-dir", dir)
		if err == nil || !strings.Contains(err.Error(), "hearthroom auth login --no-wait") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("another API", func(t *testing.T) {
		dir := authTestEnv(t)
		store := &config.Store{Dir: dir}
		_ = store.SavePendingLogin(config.PendingLogin{API: "https://api.other.test", ClientID: "c", DeviceCode: "d",
			Interval: 5, ExpiresAt: time.Now().Add(10 * time.Minute)})
		_, _, err := runCLI(t, "auth", "login", "--resume", "--api", srv.URL, "--config-dir", dir)
		if err == nil || !strings.Contains(err.Error(), "https://api.other.test") || !strings.Contains(err.Error(), "hearthroom auth login --no-wait") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("expired", func(t *testing.T) {
		dir := authTestEnv(t)
		store := &config.Store{Dir: dir}
		_ = store.SavePendingLogin(config.PendingLogin{API: srv.URL, ClientID: "c", DeviceCode: "d",
			Interval: 5, ExpiresAt: time.Now().Add(-time.Minute)})
		_, _, err := runCLI(t, "auth", "login", "--resume", "--api", srv.URL, "--config-dir", dir)
		if err == nil || !strings.Contains(err.Error(), "expired") || !strings.Contains(err.Error(), "hearthroom auth login --no-wait") {
			t.Fatalf("err = %v", err)
		}
		if _, ok, _ := store.LoadPendingLogin(); ok {
			t.Fatal("expired request kept")
		}
	})

	t.Run("flags are exclusive", func(t *testing.T) {
		dir := authTestEnv(t)
		if _, _, err := runCLI(t, "auth", "login", "--resume", "--no-wait", "--api", srv.URL, "--config-dir", dir); err == nil {
			t.Fatal("--resume with --no-wait accepted")
		}
	})
}
