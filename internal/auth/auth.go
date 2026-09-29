// Package auth implements the OAuth 2.1 authorization-code flow with PKCE
// against the provider, dynamic client registration, token storage and
// silent refresh.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/config"
)

// Scopes requested at registration and login. email.read is included only so
// the CLI can say which account is signed in (providers have no mandatory
// nickname); referral and model.invoke are deliberately excluded.
const Scopes = "profile.read email.read role.read role.write chat.play"

// LoopbackPorts are the fixed redirect ports registered with the provider.
// The provider matches loopback redirect URIs exactly, port included, so the
// client registers this list once and binds the first free port at login.
var LoopbackPorts = []int{41777, 41778, 41779, 41780, 41781}

// EnvToken names the environment variable that bypasses stored credentials.
const EnvToken = "HEARTHROOM_TOKEN"

// Discovery is the subset of RFC 8414 metadata the CLI uses.
type Discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
	RevocationEndpoint    string `json:"revocation_endpoint"`
}

// Discover fetches the authorization server metadata of the provider.
func Discover(ctx context.Context, c *api.Client) (Discovery, error) {
	var d Discovery
	if err := c.Provider(ctx, http.MethodGet, "/.well-known/oauth-authorization-server", nil, nil, &d, false); err != nil {
		return d, fmt.Errorf("discover authorization server: %w", err)
	}
	if d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" {
		return d, errors.New("discover authorization server: metadata is incomplete")
	}
	return d, nil
}

// Resource is the OAuth resource indicator for the Open API of an API base.
func Resource(apiBase string) string { return strings.TrimRight(apiBase, "/") + "/open/v1" }

// RedirectURIs lists the loopback redirect URIs for the fixed ports.
func RedirectURIs() []string {
	out := make([]string, 0, len(LoopbackPorts))
	for _, p := range LoopbackPorts {
		out = append(out, fmt.Sprintf("http://127.0.0.1:%d/callback", p))
	}
	return out
}

// EnsureClient returns the registered client for the API base, registering
// one when the config has none or the stored one requests different scopes
// or redirect URIs.
func EnsureClient(ctx context.Context, c *api.Client, store *config.Store, cfg *config.Config, d Discovery) (config.ClientReg, error) {
	want := RedirectURIs()
	if reg, ok := cfg.Clients[c.API]; ok && reg.ClientID != "" && reg.Scope == Scopes && equalStrings(reg.RedirectURIs, want) {
		return reg, nil
	}
	if d.RegistrationEndpoint == "" {
		return config.ClientReg{}, errors.New("provider does not offer dynamic client registration; set the client id in config.json")
	}
	body := map[string]any{
		"client_name":                "Hearthroom CLI",
		"redirect_uris":              want,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_method": "none",
		"scope":                      Scopes,
	}
	var resp struct {
		ClientID string `json:"client_id"`
		Scope    string `json:"scope"`
	}
	if err := c.Provider(ctx, http.MethodPost, pathOf(d.RegistrationEndpoint), nil, body, &resp, false); err != nil {
		return config.ClientReg{}, fmt.Errorf("register client: %w", err)
	}
	if resp.ClientID == "" {
		return config.ClientReg{}, errors.New("register client: no client_id in response")
	}
	reg := config.ClientReg{ClientID: resp.ClientID, RedirectURIs: want, Scope: Scopes}
	if cfg.Clients == nil {
		cfg.Clients = map[string]config.ClientReg{}
	}
	cfg.Clients[c.API] = reg
	if err := store.SaveConfig(*cfg); err != nil {
		return reg, err
	}
	return reg, nil
}

// PKCE holds a verifier and its S256 challenge.
type PKCE struct {
	Verifier  string
	Challenge string
}

// NewPKCE generates a 32-byte verifier and its challenge.
func NewPKCE() (PKCE, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return PKCE{}, err
	}
	v := base64.RawURLEncoding.EncodeToString(buf[:])
	return PKCE{Verifier: v, Challenge: Challenge(v)}, nil
}

// Challenge computes the S256 code challenge for a verifier.
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomState() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

// LoginOptions steer the interactive flow.
type LoginOptions struct {
	// OpenBrowser opens the authorization URL; nil or NoBrowser prints it.
	OpenBrowser func(url string) error
	NoBrowser   bool
	// Status receives one-line progress messages.
	Status func(format string, args ...any)
	// Timeout bounds the wait for the browser callback.
	Timeout time.Duration
	// Listen overrides the loopback bind (tests). Default binds 127.0.0.1:<port>.
	Listen func(port int) (net.Listener, error)
}

// Login runs the authorization-code flow and stores the resulting tokens.
func Login(ctx context.Context, c *api.Client, store *config.Store, cfg *config.Config, opts LoginOptions) (config.Credential, error) {
	if opts.Status == nil {
		opts.Status = func(string, ...any) {}
	}
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}
	if opts.Listen == nil {
		opts.Listen = func(port int) (net.Listener, error) {
			return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		}
	}
	d, err := Discover(ctx, c)
	if err != nil {
		return config.Credential{}, err
	}
	reg, err := EnsureClient(ctx, c, store, cfg, d)
	if err != nil {
		return config.Credential{}, err
	}
	ln, port, err := bindLoopback(opts.Listen)
	if err != nil {
		return config.Credential{}, err
	}
	defer ln.Close()
	redirect := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	pk, err := NewPKCE()
	if err != nil {
		return config.Credential{}, err
	}
	state, err := randomState()
	if err != nil {
		return config.Credential{}, err
	}
	resource := Resource(c.API)
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {reg.ClientID},
		"redirect_uri":          {redirect},
		"scope":                 {Scopes},
		"resource":              {resource},
		"code_challenge":        {pk.Challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	authURL := d.AuthorizationEndpoint + "?" + q.Encode()

	result := make(chan callback, 1)
	srv := &http.Server{Handler: callbackHandler(state, result), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if opts.NoBrowser || opts.OpenBrowser == nil {
		opts.Status("Open this URL in your browser to sign in:\n\n  %s\n", authURL)
	} else if err := opts.OpenBrowser(authURL); err != nil {
		opts.Status("Could not open a browser (%v). Open this URL instead:\n\n  %s\n", err, authURL)
	} else {
		opts.Status("Your browser has been opened to sign in. If it did not open, visit:\n\n  %s\n", authURL)
	}
	opts.Status("Waiting for the sign-in to complete on %s…", redirect)

	timer := time.NewTimer(opts.Timeout)
	defer timer.Stop()
	var cb callback
	select {
	case cb = <-result:
	case <-timer.C:
		return config.Credential{}, errors.New("timed out waiting for the browser sign-in")
	case <-ctx.Done():
		return config.Credential{}, ctx.Err()
	}
	if cb.err != nil {
		return config.Credential{}, cb.err
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {cb.code},
		"client_id":     {reg.ClientID},
		"redirect_uri":  {redirect},
		"code_verifier": {pk.Verifier},
		"resource":      {resource},
	}
	cred, err := exchange(ctx, c, pathOf(d.TokenEndpoint), form)
	if err != nil {
		return config.Credential{}, err
	}
	cred.ClientID = reg.ClientID
	if err := saveCredential(store, c.API, cred); err != nil {
		return cred, err
	}
	return cred, nil
}

type callback struct {
	code string
	err  error
}

func callbackHandler(state string, result chan<- callback) http.Handler {
	var once sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var cb callback
		switch {
		case q.Get("error") != "":
			cb.err = fmt.Errorf("authorization was not granted: %s %s", q.Get("error"), q.Get("error_description"))
		case q.Get("state") != state:
			cb.err = errors.New("authorization response did not match this login attempt (state mismatch)")
		case q.Get("code") == "":
			cb.err = errors.New("authorization response carried no code")
		default:
			cb.code = q.Get("code")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if cb.err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, page("Sign-in failed", html.EscapeString(cb.err.Error())))
		} else {
			_, _ = io.WriteString(w, page("Signed in", "You can close this window and return to the terminal."))
		}
		once.Do(func() { result <- cb })
	})
	return mux
}

func page(title, body string) string {
	return "<!doctype html><meta charset=utf-8><title>Hearthroom CLI · " + title + "</title>" +
		"<body style=\"font-family:system-ui;max-width:32em;margin:4em auto;padding:0 1em\">" +
		"<h1 style=\"font-size:1.4em\">" + title + "</h1><p>" + body + "</p></body>"
}

func bindLoopback(listen func(int) (net.Listener, error)) (net.Listener, int, error) {
	var lastErr error
	for _, p := range LoopbackPorts {
		ln, err := listen(p)
		if err == nil {
			return ln, p, nil
		}
		lastErr = err
	}
	return nil, 0, fmt.Errorf("no loopback port available for the sign-in callback (tried %v): %w", LoopbackPorts, lastErr)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	Resource     string `json:"resource"`
	TokenType    string `json:"token_type"`
}

func exchange(ctx context.Context, c *api.Client, tokenPath string, form url.Values) (config.Credential, error) {
	var tr tokenResponse
	if err := c.Form(ctx, tokenPath, form, &tr); err != nil {
		return config.Credential{}, fmt.Errorf("token exchange: %w", err)
	}
	if tr.AccessToken == "" {
		return config.Credential{}, errors.New("token exchange: no access token in response")
	}
	exp := time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	if tr.ExpiresIn <= 0 {
		exp = time.Now().Add(time.Hour)
	}
	return config.Credential{
		AccessToken: tr.AccessToken, RefreshToken: tr.RefreshToken,
		ExpiresAt: exp, Scope: tr.Scope, Resource: tr.Resource,
	}, nil
}

func saveCredential(store *config.Store, apiBase string, cred config.Credential) error {
	creds, err := store.LoadCredentials()
	if err != nil {
		return err
	}
	creds[apiBase] = cred
	return store.SaveCredentials(creds)
}

// Logout revokes the stored token (best effort) and forgets it.
func Logout(ctx context.Context, c *api.Client, store *config.Store) (bool, error) {
	creds, err := store.LoadCredentials()
	if err != nil {
		return false, err
	}
	cred, ok := creds[c.API]
	if !ok {
		return false, nil
	}
	if d, err := Discover(ctx, c); err == nil && d.RevocationEndpoint != "" {
		form := url.Values{"token": {cred.RefreshToken}, "client_id": {cred.ClientID}}
		if cred.RefreshToken == "" {
			form.Set("token", cred.AccessToken)
		}
		_ = c.Form(ctx, pathOf(d.RevocationEndpoint), form, nil)
	}
	delete(creds, c.API)
	return true, store.SaveCredentials(creds)
}

// ErrNotLoggedIn is returned by the token source when nothing is stored.
var ErrNotLoggedIn = fmt.Errorf("%w: not signed in; run `hearthroom auth login` or set %s", api.ErrNoToken, EnvToken)

// Source returns a TokenSource that prefers HEARTHROOM_TOKEN, then the stored
// credential for the client's API base, refreshing it when it is about to
// expire. It is safe for concurrent use.
func Source(store *config.Store, c *api.Client) api.TokenSource {
	var mu sync.Mutex
	return func(ctx context.Context) (string, error) {
		if tok := strings.TrimSpace(os.Getenv(EnvToken)); tok != "" {
			return tok, nil
		}
		mu.Lock()
		defer mu.Unlock()
		creds, err := store.LoadCredentials()
		if err != nil {
			return "", err
		}
		cred, ok := creds[c.API]
		if !ok || cred.AccessToken == "" {
			return "", ErrNotLoggedIn
		}
		if time.Until(cred.ExpiresAt) > 2*time.Minute {
			return cred.AccessToken, nil
		}
		if cred.RefreshToken == "" {
			return "", errors.New("the stored sign-in has expired; run `hearthroom auth login`")
		}
		d, err := Discover(ctx, c)
		if err != nil {
			return "", err
		}
		form := url.Values{
			"grant_type":    {"refresh_token"},
			"refresh_token": {cred.RefreshToken},
			"client_id":     {cred.ClientID},
			"resource":      {Resource(c.API)},
		}
		fresh, err := exchange(ctx, c, pathOf(d.TokenEndpoint), form)
		if err != nil {
			return "", fmt.Errorf("the stored sign-in could not be refreshed; run `hearthroom auth login` (%w)", err)
		}
		fresh.ClientID = cred.ClientID
		if fresh.RefreshToken == "" {
			fresh.RefreshToken = cred.RefreshToken
		}
		creds[c.API] = fresh
		if err := store.SaveCredentials(creds); err != nil {
			return "", err
		}
		return fresh.AccessToken, nil
	}
}

// HasCredential reports whether a token is available without touching the network.
func HasCredential(store *config.Store, apiBase string) bool {
	if strings.TrimSpace(os.Getenv(EnvToken)) != "" {
		return true
	}
	creds, err := store.LoadCredentials()
	if err != nil {
		return false
	}
	cred, ok := creds[apiBase]
	return ok && cred.AccessToken != ""
}

// pathOf strips the scheme and host from an absolute endpoint URL so it can
// be sent through the client, which prefixes the API base. Endpoints on a
// different host are returned unchanged and will fail loudly.
func pathOf(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	p := u.Path
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	return p
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ReadAll is a small helper for tests.
func ReadAll(r io.Reader) string {
	b, _ := io.ReadAll(r)
	return string(b)
}
