package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/config"
)

// deviceProvider is a fake provider for the device flow. The token endpoint
// answers device_code polls from a script of error codes ("" issues tokens);
// the authorization-code grant and the loopback authorize redirect stay
// available so fallback can be observed.
type deviceProvider struct {
	srv *httptest.Server

	offerDevice   bool   // advertise device_authorization_endpoint
	deviceStatus  int    // non-zero: device endpoint answers this status...
	deviceError   string // ...with this error code
	retryAfter    int
	expiresIn     int
	pollScript    []string
	deviceCalls   atomic.Int32
	authorizeHits atomic.Int32

	mu         sync.Mutex
	polls      int
	deviceForm url.Values
	pollForms  []url.Values
}

func newDeviceProvider(t *testing.T, offerDevice bool, script ...string) *deviceProvider {
	t.Helper()
	dp := &deviceProvider{offerDevice: offerDevice, pollScript: script, expiresIn: 900}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		meta := map[string]any{
			"issuer": base, "authorization_endpoint": base + "/oauth/authorize",
			"token_endpoint": base + "/oauth/token", "registration_endpoint": base + "/oauth/register",
			"revocation_endpoint": base + "/oauth/revoke",
		}
		if dp.offerDevice {
			meta["device_authorization_endpoint"] = base + "/oauth/device_authorization"
			meta["grant_types_supported"] = []string{"authorization_code", "refresh_token", "urn:ietf:params:oauth:grant-type:device_code"}
		}
		_ = json.NewEncoder(w).Encode(meta)
	})
	mux.HandleFunc("/oauth/register", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "hh_client_test", "scope": Scopes})
	})
	mux.HandleFunc("/oauth/device_authorization", func(w http.ResponseWriter, r *http.Request) {
		dp.deviceCalls.Add(1)
		_ = r.ParseForm()
		dp.mu.Lock()
		dp.deviceForm = r.PostForm
		dp.mu.Unlock()
		if dp.deviceStatus != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(dp.deviceStatus)
			body := map[string]any{"error": dp.deviceError}
			if dp.retryAfter > 0 {
				body["retry_after"] = dp.retryAfter
			}
			_ = json.NewEncoder(w).Encode(body)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code": "hh_dc_secret", "user_code": "BCDF-GHJK",
			"verification_uri":          "https://console.example.test/device",
			"verification_uri_complete": "https://console.example.test/device?user_code=BCDF-GHJK",
			"expires_in":                dp.expiresIn, "interval": 5,
		})
	})
	mux.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		dp.authorizeHits.Add(1)
		q := r.URL.Query()
		http.Redirect(w, r, q.Get("redirect_uri")+"?code=CODE1&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		grant := r.PostForm.Get("grant_type")
		if grant == DeviceGrantType {
			dp.mu.Lock()
			dp.pollForms = append(dp.pollForms, r.PostForm)
			step := ""
			if dp.polls < len(dp.pollScript) {
				step = dp.pollScript[dp.polls]
			}
			dp.polls++
			dp.mu.Unlock()
			if step != "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, `{"error":%q}`, step)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at_" + grant, "refresh_token": "rt_" + grant, "expires_in": 3600,
			"scope": Scopes, "resource": "http://" + r.Host + "/open/v1", "token_type": "Bearer",
		})
	})
	dp.srv = httptest.NewServer(mux)
	t.Cleanup(dp.srv.Close)
	return dp
}

func (dp *deviceProvider) pollCount() int {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	return dp.polls
}

// fakeClock replaces the poll loop's clock; Sleep advances time instantly and
// records how long the loop asked to wait.
type fakeClock struct {
	now    time.Time
	sleeps []time.Duration
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)} }

func (f *fakeClock) Now() time.Time { return f.now }

func (f *fakeClock) Sleep(_ context.Context, d time.Duration) error {
	f.sleeps = append(f.sleeps, d)
	f.now = f.now.Add(d)
	return nil
}

// lines collects Status output, one entry per call.
type lines struct{ got []string }

func (l *lines) Status(format string, args ...any) {
	l.got = append(l.got, fmt.Sprintf(format, args...))
}
func (l *lines) String() string { return strings.Join(l.got, "\n") }

// clearSSH makes sure the machine running the tests does not look like an
// SSH session unless a test says so.
func clearSSH(t *testing.T) {
	t.Helper()
	for _, k := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		t.Setenv(k, "")
	}
}

func deviceSetup(t *testing.T, dp *deviceProvider) (*api.Client, *config.Store, *config.Config) {
	t.Helper()
	t.Setenv(EnvToken, "")
	store := &config.Store{Dir: t.TempDir()}
	cfg := &config.Config{Clients: map[string]config.ClientReg{}}
	return api.New(dp.srv.URL, "http://site.invalid", "test", nil), store, cfg
}

func TestDiscoverReadsDeviceEndpoint(t *testing.T) {
	with := newDeviceProvider(t, true)
	d, err := Discover(context.Background(), api.New(with.srv.URL, "", "test", nil))
	if err != nil {
		t.Fatal(err)
	}
	if d.DeviceAuthorizationEndpoint != with.srv.URL+"/oauth/device_authorization" {
		t.Fatalf("device endpoint = %q", d.DeviceAuthorizationEndpoint)
	}
	without := newDeviceProvider(t, false)
	d, err = Discover(context.Background(), api.New(without.srv.URL, "", "test", nil))
	if err != nil {
		t.Fatal(err)
	}
	if d.DeviceAuthorizationEndpoint != "" {
		t.Fatalf("device endpoint should be empty, got %q", d.DeviceAuthorizationEndpoint)
	}
}

// The default login prints the code, then the URL, then the opened page;
// polls at the advertised interval; grows it by 5 s on slow_down; and stores
// the credential the same way the loopback flow does.
func TestLoginUsesDeviceFlowAndHonoursSlowDown(t *testing.T) {
	clearSSH(t)
	dp := newDeviceProvider(t, true, "authorization_pending", "slow_down", "authorization_pending", "")
	client, store, cfg := deviceSetup(t, dp)
	clock := newFakeClock()
	var out lines
	var opened []string
	opts := LoginOptions{
		OpenBrowser: func(u string) error { opened = append(opened, u); return nil },
		Status:      out.Status, Timeout: 10 * time.Minute, Now: clock.Now, Sleep: clock.Sleep,
	}
	cred, err := Login(context.Background(), client, store, cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	if cred.AccessToken != "at_"+DeviceGrantType || cred.ClientID != "hh_client_test" {
		t.Fatalf("cred = %+v", cred)
	}
	want := []time.Duration{5 * time.Second, 5 * time.Second, 10 * time.Second, 10 * time.Second}
	if fmt.Sprint(clock.sleeps) != fmt.Sprint(want) {
		t.Fatalf("waits = %v, want %v", clock.sleeps, want)
	}
	if dp.pollCount() != 4 {
		t.Fatalf("polls = %d", dp.pollCount())
	}
	creds, _ := store.LoadCredentials()
	if stored := creds[dp.srv.URL]; stored.AccessToken != cred.AccessToken || stored.ClientID != "hh_client_test" || stored.RefreshToken == "" {
		t.Fatalf("stored credential = %+v", stored)
	}
	if len(opened) != 1 || opened[0] != "https://console.example.test/device?user_code=BCDF-GHJK" {
		t.Fatalf("opened = %v", opened)
	}
	if dp.authorizeHits.Load() != 0 {
		t.Fatal("loopback authorize was used")
	}
	text := out.String()
	iCode := strings.Index(text, "BCDF-GHJK")
	iURL := strings.Index(text, "https://console.example.test/device ")
	iOpened := strings.Index(text, "https://console.example.test/device?user_code=BCDF-GHJK")
	if iCode < 0 || iURL < 0 || iOpened < 0 || !(iCode < iURL && iURL < iOpened) {
		t.Fatalf("expected code, URL, opened page in that order:\n%s", text)
	}
	dp.mu.Lock()
	df, pf := dp.deviceForm, dp.pollForms[0]
	dp.mu.Unlock()
	if df.Get("client_id") != "hh_client_test" || df.Get("scope") != Scopes || df.Get("resource") != Resource(dp.srv.URL) {
		t.Fatalf("device request form = %v", df)
	}
	if pf.Get("device_code") != "hh_dc_secret" || pf.Get("client_id") != "hh_client_test" {
		t.Fatalf("poll form = %v", pf)
	}
}

func TestLoginDeviceDenied(t *testing.T) {
	clearSSH(t)
	dp := newDeviceProvider(t, true, "authorization_pending", "access_denied")
	client, store, cfg := deviceSetup(t, dp)
	clock := newFakeClock()
	_, err := Login(context.Background(), client, store, cfg, LoginOptions{NoBrowser: true, Now: clock.Now, Sleep: clock.Sleep})
	if err == nil || err.Error() != "Sign-in was denied in the browser." {
		t.Fatalf("err = %v", err)
	}
	if creds, _ := store.LoadCredentials(); len(creds) != 0 {
		t.Fatalf("credential stored after denial: %+v", creds)
	}
}

func TestLoginDeviceExpired(t *testing.T) {
	clearSSH(t)
	dp := newDeviceProvider(t, true, "expired_token")
	client, store, cfg := deviceSetup(t, dp)
	clock := newFakeClock()
	_, err := Login(context.Background(), client, store, cfg, LoginOptions{NoBrowser: true, Now: clock.Now, Sleep: clock.Sleep})
	if err == nil || err.Error() != "The code expired. Run `hearthroom auth login` again." {
		t.Fatalf("err = %v", err)
	}
}

// The wait never outlives the code, and --timeout can make it shorter.
func TestLoginDeviceWaitIsBounded(t *testing.T) {
	clearSSH(t)
	pending := make([]string, 100)
	for i := range pending {
		pending[i] = "authorization_pending"
	}

	dp := newDeviceProvider(t, true, pending...)
	dp.expiresIn = 12
	client, store, cfg := deviceSetup(t, dp)
	clock := newFakeClock()
	start := clock.now
	_, err := Login(context.Background(), client, store, cfg, LoginOptions{NoBrowser: true, Timeout: time.Hour, Now: clock.Now, Sleep: clock.Sleep})
	if err == nil || !strings.Contains(err.Error(), "The code expired") {
		t.Fatalf("err = %v", err)
	}
	if waited := clock.now.Sub(start); waited > 12*time.Second {
		t.Fatalf("waited %v past a 12 s code", waited)
	}

	dp2 := newDeviceProvider(t, true, pending...)
	client, store, cfg = deviceSetup(t, dp2)
	clock = newFakeClock()
	start = clock.now
	_, err = Login(context.Background(), client, store, cfg, LoginOptions{NoBrowser: true, Timeout: 7 * time.Second, Now: clock.Now, Sleep: clock.Sleep})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
	if waited := clock.now.Sub(start); waited > 7*time.Second {
		t.Fatalf("waited %v past a 7 s timeout", waited)
	}
}

func TestLoginDeviceSkipsBrowserOverSSH(t *testing.T) {
	clearSSH(t)
	t.Setenv("SSH_CONNECTION", "203.0.113.1 50000 198.51.100.2 22")
	dp := newDeviceProvider(t, true, "")
	client, store, cfg := deviceSetup(t, dp)
	clock := newFakeClock()
	var out lines
	var opened []string
	_, err := Login(context.Background(), client, store, cfg, LoginOptions{
		OpenBrowser: func(u string) error { opened = append(opened, u); return nil },
		Status:      out.Status, Now: clock.Now, Sleep: clock.Sleep,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(opened) != 0 {
		t.Fatalf("browser opened over SSH: %v", opened)
	}
	if !strings.Contains(out.String(), "BCDF-GHJK") || !strings.Contains(out.String(), "https://console.example.test/device") {
		t.Fatalf("code and URL not printed:\n%s", out.String())
	}
}

// Providers without the device endpoint, and clients the provider does not
// allow to use it, sign in through the unchanged loopback flow.
func TestLoginFallsBackToLoopback(t *testing.T) {
	cases := []struct {
		name   string
		offer  bool
		status int
		code   string
	}{
		{"no device endpoint", false, 0, ""},
		{"unauthorized_client", true, http.StatusBadRequest, "unauthorized_client"},
		{"unsupported_grant_type", true, http.StatusBadRequest, "unsupported_grant_type"},
		{"not found", true, http.StatusNotFound, "not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearSSH(t)
			dp := newDeviceProvider(t, tc.offer)
			dp.deviceStatus, dp.deviceError = tc.status, tc.code
			client, store, cfg := deviceSetup(t, dp)
			restore := useEphemeralLoopbackPort(t)
			defer restore()
			var out lines
			browser := func(target string) error {
				resp, err := http.Get(target)
				if err != nil {
					return err
				}
				resp.Body.Close()
				return nil
			}
			cred, err := Login(context.Background(), client, store, cfg, LoginOptions{OpenBrowser: browser, Status: out.Status, Timeout: 10 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			if cred.AccessToken != "at_authorization_code" {
				t.Fatalf("cred = %+v", cred)
			}
			if dp.authorizeHits.Load() != 1 || dp.pollCount() != 0 {
				t.Fatalf("authorize=%d polls=%d", dp.authorizeHits.Load(), dp.pollCount())
			}
			if len(out.got) == 0 || !strings.Contains(out.got[0], "code is not available") {
				t.Fatalf("first status line should explain the fallback:\n%s", out.String())
			}
		})
	}
}

// Other device endpoint errors are real failures, not a reason to fall back.
func TestLoginDeviceRequestErrors(t *testing.T) {
	clearSSH(t)
	dp := newDeviceProvider(t, true)
	dp.deviceStatus, dp.deviceError = http.StatusBadRequest, "invalid_scope"
	client, store, cfg := deviceSetup(t, dp)
	_, err := Login(context.Background(), client, store, cfg, LoginOptions{NoBrowser: true})
	var ae *api.Error
	if err == nil || !errors.As(err, &ae) || ae.Code != "invalid_scope" {
		t.Fatalf("err = %v", err)
	}
	if dp.authorizeHits.Load() != 0 {
		t.Fatal("fell back to loopback on invalid_scope")
	}

	dp.deviceStatus, dp.deviceError, dp.retryAfter = http.StatusTooManyRequests, "rate_limited", 42
	_, err = Login(context.Background(), client, store, cfg, LoginOptions{NoBrowser: true})
	if err == nil || !strings.Contains(err.Error(), "42 seconds") {
		t.Fatalf("err = %v", err)
	}
}

// LoopbackOnly keeps the browser-redirect flow even when the provider offers
// codes (the end-to-end suite approves through the loopback redirect).
func TestLoginLoopbackOnly(t *testing.T) {
	clearSSH(t)
	dp := newDeviceProvider(t, true)
	client, store, cfg := deviceSetup(t, dp)
	restore := useEphemeralLoopbackPort(t)
	defer restore()
	browser := func(target string) error {
		resp, err := http.Get(target)
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	}
	if _, err := Login(context.Background(), client, store, cfg, LoginOptions{OpenBrowser: browser, LoopbackOnly: true, Timeout: 10 * time.Second}); err != nil {
		t.Fatal(err)
	}
	if dp.deviceCalls.Load() != 0 || dp.authorizeHits.Load() != 1 {
		t.Fatalf("device=%d authorize=%d", dp.deviceCalls.Load(), dp.authorizeHits.Load())
	}
}

// StartPendingLogin and ResumeLogin split the device flow across two
// processes; resume polls at once, then at the stored interval.
func TestPendingLoginStartAndResume(t *testing.T) {
	clearSSH(t)
	dp := newDeviceProvider(t, true, "authorization_pending", "")
	client, store, cfg := deviceSetup(t, dp)
	clock := newFakeClock()
	opts := LoginOptions{Now: clock.Now, Sleep: clock.Sleep}
	dc, err := StartPendingLogin(context.Background(), client, store, cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	if dc.UserCode != "BCDF-GHJK" || dc.VerificationURIComplete == "" || dc.ExpiresIn != 900 || dc.Interval != 5 {
		t.Fatalf("device code = %+v", dc)
	}
	p, ok, err := store.LoadPendingLogin()
	if err != nil || !ok {
		t.Fatalf("pending not stored: %v", err)
	}
	if p.API != dp.srv.URL || p.ClientID != "hh_client_test" || p.DeviceCode != "hh_dc_secret" || p.Interval != 5 || !p.ExpiresAt.Equal(clock.now.Add(900*time.Second)) {
		t.Fatalf("pending = %+v", p)
	}
	if dp.pollCount() != 0 {
		t.Fatal("--no-wait must not poll")
	}

	cred, err := ResumeLogin(context.Background(), client, store, opts)
	if err != nil {
		t.Fatal(err)
	}
	if cred.ClientID != "hh_client_test" || cred.AccessToken == "" {
		t.Fatalf("cred = %+v", cred)
	}
	if fmt.Sprint(clock.sleeps) != fmt.Sprint([]time.Duration{5 * time.Second}) {
		t.Fatalf("resume waits = %v, want one 5s wait after an immediate poll", clock.sleeps)
	}
	if _, ok, _ := store.LoadPendingLogin(); ok {
		t.Fatal("pending login not deleted after success")
	}
}

func TestStartPendingLoginNeedsDeviceFlow(t *testing.T) {
	dp := newDeviceProvider(t, false)
	client, store, cfg := deviceSetup(t, dp)
	_, err := StartPendingLogin(context.Background(), client, store, cfg, LoginOptions{})
	if err == nil || !strings.Contains(err.Error(), "--no-wait") {
		t.Fatalf("err = %v", err)
	}
	if _, ok, _ := store.LoadPendingLogin(); ok {
		t.Fatal("pending stored without a device code")
	}
}

// A timed-out resume keeps the request so the next --resume can continue;
// denial removes it.
func TestResumeLoginTimeoutKeepsDenialDeletes(t *testing.T) {
	clearSSH(t)
	pending := make([]string, 50)
	for i := range pending {
		pending[i] = "authorization_pending"
	}
	dp := newDeviceProvider(t, true, append(pending, "access_denied")...)
	client, store, cfg := deviceSetup(t, dp)
	clock := newFakeClock()
	opts := LoginOptions{Now: clock.Now, Sleep: clock.Sleep, Timeout: 20 * time.Second}
	if _, err := StartPendingLogin(context.Background(), client, store, cfg, opts); err != nil {
		t.Fatal(err)
	}
	_, err := ResumeLogin(context.Background(), client, store, opts)
	if err == nil || !strings.Contains(err.Error(), "--resume") {
		t.Fatalf("err = %v", err)
	}
	if _, ok, _ := store.LoadPendingLogin(); !ok {
		t.Fatal("timed-out resume dropped the pending request")
	}
	opts.Timeout = time.Hour
	_, err = ResumeLogin(context.Background(), client, store, opts)
	if err == nil || err.Error() != "Sign-in was denied in the browser." {
		t.Fatalf("err = %v", err)
	}
	if _, ok, _ := store.LoadPendingLogin(); ok {
		t.Fatal("denied request still pending")
	}
}
