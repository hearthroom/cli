// Package e2e runs the CLI against a real provider. It is skipped unless
// HEARTHROOM_E2E_API points at one and HEARTHROOM_E2E_SESSION holds a signed-in
// browser session cookie for that provider (the value of its session cookie
// after signing in through its web UI). The session is used only to approve
// the CLI's OAuth request headlessly; everything else goes through the CLI's
// own code paths.
package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/auth"
	"github.com/hearthroom/cli/internal/config"
)

const sessionCookie = "hh_session"

type env struct {
	api     string
	session string
	store   *config.Store
	cfg     config.Config
	client  *api.Client
}

func setup(t *testing.T) *env {
	t.Helper()
	apiBase := strings.TrimRight(os.Getenv("HEARTHROOM_E2E_API"), "/")
	session := os.Getenv("HEARTHROOM_E2E_SESSION")
	if apiBase == "" || session == "" {
		t.Skip("set HEARTHROOM_E2E_API and HEARTHROOM_E2E_SESSION to run end-to-end tests")
	}
	dir := t.TempDir()
	t.Setenv("HEARTHROOM_CONFIG_DIR", dir)
	t.Setenv(auth.EnvToken, "")
	store := &config.Store{Dir: dir}
	e := &env{api: apiBase, session: session, store: store, cfg: config.Config{Clients: map[string]config.ClientReg{}}}
	e.client = api.New(apiBase, "http://site.invalid", "hearthroom-cli/e2e", nil)
	e.client.Token = auth.Source(store, e.client)
	return e
}

// approve plays the browser: it follows the authorize redirect to the consent
// page, approves with the session cookie, and hits the CLI's loopback callback.
func (e *env) approve(target string) error {
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest(http.MethodGet, target, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: e.session})
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || loc.Query().Get("oauth_request") == "" {
		return fmt.Errorf("authorize did not redirect to consent: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	body := strings.NewReader(fmt.Sprintf(`{"oauth_request":%q,"decision":"approve"}`, loc.Query().Get("oauth_request")))
	req, _ = http.NewRequest(http.MethodPost, e.api+"/oauth/authorize/complete", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: e.session})
	resp, err = hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var done struct {
		RedirectURL string `json:"redirect_url"`
		Error       string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&done)
	if resp.StatusCode != http.StatusOK || done.RedirectURL == "" {
		return fmt.Errorf("consent failed: %d %s (is the session cookie still valid?)", resp.StatusCode, done.Error)
	}
	cb, err := hc.Get(done.RedirectURL)
	if err != nil {
		return err
	}
	cb.Body.Close()
	if cb.StatusCode != http.StatusOK {
		return fmt.Errorf("loopback callback returned %d", cb.StatusCode)
	}
	return nil
}

func (e *env) login(t *testing.T) config.Credential {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// approve plays the loopback redirect, so keep login off the code flow.
	cred, err := auth.Login(ctx, e.client, e.store, &e.cfg, auth.LoginOptions{OpenBrowser: e.approve, Timeout: 20 * time.Second, LoopbackOnly: true})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return cred
}

func TestLoginAndWhoami(t *testing.T) {
	e := setup(t)
	cred := e.login(t)
	if !strings.Contains(cred.Scope, "role.write") {
		t.Fatalf("scope missing role.write: %q", cred.Scope)
	}
	var me struct {
		AccountNumID int64 `json:"accountNumId"`
	}
	if err := e.client.OpenGet(context.Background(), "/me", nil, &me); err != nil {
		t.Fatal(err)
	}
	if me.AccountNumID == 0 {
		t.Fatal("me returned no public id")
	}
	// A second login must reuse the registered client.
	first := e.cfg.Clients[e.api].ClientID
	e.login(t)
	if e.cfg.Clients[e.api].ClientID != first {
		t.Fatal("client re-registered on second login")
	}
	// Logout revokes and forgets.
	removed, err := auth.Logout(context.Background(), e.client, e.store)
	if err != nil || !removed {
		t.Fatalf("logout: %v %v", removed, err)
	}
	if err := e.client.OpenGet(context.Background(), "/me", nil, &me); !errors.Is(err, auth.ErrNotLoggedIn) {
		t.Fatalf("expected not logged in after logout, got %v", err)
	}
}
