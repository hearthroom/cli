package auth

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/config"
)

func TestPKCEChallengeIsS256OfVerifier(t *testing.T) {
	pk, err := NewPKCE()
	if err != nil {
		t.Fatal(err)
	}
	if len(pk.Verifier) < 43 {
		t.Fatalf("verifier too short: %d", len(pk.Verifier))
	}
	if pk.Challenge != Challenge(pk.Verifier) {
		t.Fatal("challenge does not match verifier")
	}
	// RFC 7636 appendix B vector.
	if got := Challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("S256 vector mismatch: %s", got)
	}
}

func TestCallbackHandlerRejectsStateMismatch(t *testing.T) {
	result := make(chan callback, 1)
	h := callbackHandler("good", result)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/callback?code=abc&state=evil", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	cb := <-result
	if cb.err == nil || cb.code != "" {
		t.Fatalf("expected state error, got %+v", cb)
	}
}

func TestCallbackHandlerAcceptsCodeOnce(t *testing.T) {
	result := make(chan callback, 1)
	h := callbackHandler("s", result)
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/callback?code=abc&state=s", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	}
	cb := <-result
	if cb.code != "abc" || cb.err != nil {
		t.Fatalf("got %+v", cb)
	}
	select {
	case <-result:
		t.Fatal("second callback should be ignored")
	default:
	}
}

func TestBindLoopbackSkipsBusyPorts(t *testing.T) {
	tried := []int{}
	listen := func(port int) (net.Listener, error) {
		tried = append(tried, port)
		if port == LoopbackPorts[0] {
			return nil, &net.OpError{Op: "listen"}
		}
		return net.Listen("tcp", "127.0.0.1:0")
	}
	ln, port, err := bindLoopback(listen)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if port != LoopbackPorts[1] || len(tried) != 2 {
		t.Fatalf("port=%d tried=%v", port, tried)
	}
}

// fakeProvider serves discovery, registration, token and the consent redirect
// so Login and Source can be exercised end to end without a browser.
type fakeProvider struct {
	srv         *httptest.Server
	tokenCalls  atomic.Int32
	registered  atomic.Int32
	lastGrant   string
	lastRefresh string
}

func newFakeProvider(t *testing.T) *fakeProvider {
	fp := &fakeProvider{}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": base, "authorization_endpoint": base + "/oauth/authorize",
			"token_endpoint": base + "/oauth/token", "registration_endpoint": base + "/oauth/register",
			"revocation_endpoint": base + "/oauth/revoke",
		})
	})
	mux.HandleFunc("/oauth/register", func(w http.ResponseWriter, r *http.Request) {
		fp.registered.Add(1)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["token_endpoint_auth_method"] != "none" {
			t.Errorf("auth method = %v", body["token_endpoint_auth_method"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "hh_client_test", "scope": Scopes})
	})
	mux.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("code_challenge_method") != "S256" || q.Get("resource") == "" || q.Get("client_id") != "hh_client_test" {
			http.Error(w, "bad authorize", http.StatusBadRequest)
			return
		}
		// Approve immediately: redirect the "browser" back with a code.
		http.Redirect(w, r, q.Get("redirect_uri")+"?code=CODE1&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		fp.tokenCalls.Add(1)
		_ = r.ParseForm()
		fp.lastGrant = r.Form.Get("grant_type")
		fp.lastRefresh = r.Form.Get("refresh_token")
		if fp.lastGrant == "authorization_code" && r.Form.Get("code_verifier") == "" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at_" + fp.lastGrant, "refresh_token": "rt_new", "expires_in": 3600,
			"scope": Scopes, "resource": "http://" + r.Host + "/open/v1", "token_type": "Bearer",
		})
	})
	fp.srv = httptest.NewServer(mux)
	t.Cleanup(fp.srv.Close)
	return fp
}

func TestLoginStoresTokensAndRegistersOnce(t *testing.T) {
	fp := newFakeProvider(t)
	store := &config.Store{Dir: t.TempDir()}
	cfg := config.Config{Clients: map[string]config.ClientReg{}}
	client := api.New(fp.srv.URL, "http://site.invalid", "test", nil)

	browser := func(target string) error {
		// Follow the authorize redirect like a browser would.
		hc := &http.Client{}
		resp, err := hc.Get(target)
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	}
	opts := LoginOptions{OpenBrowser: browser, Timeout: 10 * time.Second, Listen: func(int) (net.Listener, error) {
		return net.Listen("tcp", "127.0.0.1:0")
	}}
	// Use one free ephemeral port for the whole test so the fixed-port list
	// does not have to be available on the machine running the tests.
	restore := useEphemeralLoopbackPort(t)
	defer restore()
	opts.Listen = nil
	cred, err := Login(context.Background(), client, store, &cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	if cred.AccessToken != "at_authorization_code" || cred.ClientID != "hh_client_test" {
		t.Fatalf("cred = %+v", cred)
	}
	creds, _ := store.LoadCredentials()
	if creds[fp.srv.URL].AccessToken != cred.AccessToken {
		t.Fatal("credential was not stored under the API base")
	}
	if cfg.Clients[fp.srv.URL].ClientID != "hh_client_test" {
		t.Fatal("client registration not cached")
	}
	// Second login reuses the client.
	if _, err := Login(context.Background(), client, store, &cfg, opts); err != nil {
		t.Fatal(err)
	}
	if fp.registered.Load() != 1 {
		t.Fatalf("registered %d times", fp.registered.Load())
	}
}

// useEphemeralLoopbackPort points the loopback list at one free port for the
// duration of a test and returns the restore function.
func useEphemeralLoopbackPort(t *testing.T) func() {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	saved := LoopbackPorts
	LoopbackPorts = []int{port}
	return func() { LoopbackPorts = saved }
}

func TestSourcePrefersEnvThenRefreshesNearExpiry(t *testing.T) {
	fp := newFakeProvider(t)
	store := &config.Store{Dir: t.TempDir()}
	client := api.New(fp.srv.URL, "http://site.invalid", "test", nil)

	t.Setenv(EnvToken, "hh_live_env")
	src := Source(store, client)
	if tok, _ := src(context.Background()); tok != "hh_live_env" {
		t.Fatalf("env token not preferred: %s", tok)
	}
	t.Setenv(EnvToken, "")

	if _, err := src(context.Background()); err == nil || !strings.Contains(err.Error(), "not signed in") {
		t.Fatalf("expected not signed in, got %v", err)
	}

	_ = store.SaveCredentials(map[string]config.Credential{fp.srv.URL: {
		AccessToken: "old", RefreshToken: "rt_old", ClientID: "hh_client_test",
		ExpiresAt: time.Now().Add(30 * time.Second),
	}})
	tok, err := src(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "at_refresh_token" || fp.lastGrant != "refresh_token" || fp.lastRefresh != "rt_old" {
		t.Fatalf("tok=%s grant=%s refresh=%s", tok, fp.lastGrant, fp.lastRefresh)
	}
	creds, _ := store.LoadCredentials()
	if creds[fp.srv.URL].RefreshToken != "rt_new" {
		t.Fatal("rotated refresh token not stored")
	}
	// Fresh token: no further network call.
	before := fp.tokenCalls.Load()
	if _, err := src(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fp.tokenCalls.Load() != before {
		t.Fatal("unexpected refresh of a fresh token")
	}
}
