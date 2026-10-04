// Package auth signs the CLI in to the provider: sign-in with a one-time code
// (OAuth 2.0 device authorization grant, RFC 8628) when the provider offers
// it, the authorization-code flow with PKCE on a loopback port otherwise,
// plus dynamic client registration, token storage and silent refresh.
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
	// DeviceAuthorizationEndpoint is empty when the provider has no sign-in
	// with a code; login then uses the loopback flow.
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
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

// KnownClients maps an API base to a client id the provider's operator
// created for this CLI under the community's application, so tokens share
// the same data namespace as the web editor. A dynamically registered client
// lands in the provider's open tenant and cannot see cards made on the site.
// The entry is a public client (no secret): it signs in with a one-time code
// the person approves on the provider's own page, or with PKCE redirecting
// only to the fixed loopback ports, so publishing it here is safe.
var KnownClients = map[string]string{
	"https://api.harperharbor.com": "hh_client_Z40oH2vHYZrjNEvfrUm2DGEHyD-5WhubUj_cuxAXZCU",
}

// EnsureClient returns the client to use for the API base: a stored one that
// still matches, then a known first-party client, then a fresh dynamic
// registration.
func EnsureClient(ctx context.Context, c *api.Client, store *config.Store, cfg *config.Config, d Discovery) (config.ClientReg, error) {
	want := RedirectURIs()
	if cfg.Clients == nil {
		cfg.Clients = map[string]config.ClientReg{}
	}
	stored, hasStored := cfg.Clients[c.API]
	// A first-party client wins over anything the CLI registered itself, so
	// existing installs move to the community tenant on their next login.
	if id, ok := KnownClients[c.API]; ok && id != "" {
		if hasStored && stored.ClientID == id && stored.Scope == Scopes && equalStrings(stored.RedirectURIs, want) {
			return stored, nil
		}
		reg := config.ClientReg{ClientID: id, RedirectURIs: want, Scope: Scopes}
		cfg.Clients[c.API] = reg
		return reg, store.SaveConfig(*cfg)
	}
	if hasStored && stored.ClientID != "" && stored.Scope == Scopes && equalStrings(stored.RedirectURIs, want) {
		return stored, nil
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
	reg := config.ClientReg{ClientID: resp.ClientID, RedirectURIs: want, Scope: Scopes, Dynamic: true}
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
	// OpenBrowser opens the sign-in page; nil or NoBrowser prints the URL.
	// The device flow also leaves the browser closed in an SSH session.
	OpenBrowser func(url string) error
	NoBrowser   bool
	// Status receives one-line progress messages.
	Status func(format string, args ...any)
	// Timeout bounds the wait for the sign-in to complete.
	Timeout time.Duration
	// Listen overrides the loopback bind (tests). Default binds 127.0.0.1:<port>.
	Listen func(port int) (net.Listener, error)
	// LoopbackOnly skips sign-in with a code even when the provider offers it.
	LoopbackOnly bool
	// Now and Sleep drive the device-flow poll loop; tests replace them.
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration) error
}

func (o LoginOptions) withDefaults() LoginOptions {
	if o.Status == nil {
		o.Status = func(string, ...any) {}
	}
	if o.Timeout == 0 {
		o.Timeout = 5 * time.Minute
	}
	if o.Listen == nil {
		o.Listen = func(port int) (net.Listener, error) {
			return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Sleep == nil {
		o.Sleep = sleepContext
	}
	return o
}

// Login signs in and stores the resulting tokens. It uses sign-in with a
// one-time code when the provider offers it to this client, and the
// authorization-code flow with a loopback redirect otherwise.
func Login(ctx context.Context, c *api.Client, store *config.Store, cfg *config.Config, opts LoginOptions) (config.Credential, error) {
	opts = opts.withDefaults()
	d, err := Discover(ctx, c)
	if err != nil {
		return config.Credential{}, err
	}
	reg, err := EnsureClient(ctx, c, store, cfg, d)
	if err != nil {
		return config.Credential{}, err
	}
	if !opts.LoopbackOnly {
		dc, err := requestDeviceCode(ctx, c, d, reg.ClientID)
		var unavailable noDeviceError
		switch {
		case err == nil:
			return loginDevice(ctx, c, store, d, reg.ClientID, dc, opts)
		case !errors.As(err, &unavailable):
			return config.Credential{}, err
		}
		opts.Status("Sign-in with a code is not available (%s); signing in through a browser on this machine instead.", unavailable.why)
	}
	return loginLoopback(ctx, c, store, d, reg, opts)
}

// loginLoopback runs the authorization-code flow with PKCE, receiving the
// result on a loopback port.
func loginLoopback(ctx context.Context, c *api.Client, store *config.Store, d Discovery, reg config.ClientReg, opts LoginOptions) (config.Credential, error) {
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
var ErrNotLoggedIn = fmt.Errorf("%w; run `hearthroom auth login` (from an agent: `hearthroom auth login --no-wait`, show the person the code and address, then `hearthroom auth login --resume`) or set %s", api.ErrNoToken, EnvToken)

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
