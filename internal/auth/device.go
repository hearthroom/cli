package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/config"
)

// DeviceGrantType is the token grant for sign-in with a one-time code.
const DeviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"

// DeviceCode is the provider's answer to a device authorization request
// (RFC 8628 §3.2): the code the person enters and where they enter it.
type DeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int64  `json:"interval"`
}

// noDeviceError means the provider will not sign this client in with a code;
// the interactive login falls back to the loopback flow.
type noDeviceError struct{ why string }

func (e noDeviceError) Error() string { return "sign-in with a code is not available: " + e.why }

var (
	errDeviceDenied  = errors.New("Sign-in was denied in the browser.")
	errDeviceExpired = errors.New("The code expired. Run `hearthroom auth login` again.")
	errDeviceTimeout = errors.New("timed out waiting for the sign-in to be approved")

	errResumeNothing = errors.New("No sign-in is waiting to be resumed. Run `hearthroom auth login --no-wait` first.")
	errResumeExpired = errors.New("The code expired. Run `hearthroom auth login --no-wait` again.")
	errResumeTimeout = errors.New("Still waiting for the sign-in to be approved. Run `hearthroom auth login --resume` again to keep waiting.")
)

// requestDeviceCode asks the provider for a one-time code (RFC 8628 §3.1).
// A provider without the endpoint, or one that does not allow this client to
// use it, yields a noDeviceError.
func requestDeviceCode(ctx context.Context, c *api.Client, d Discovery, clientID string) (DeviceCode, error) {
	if d.DeviceAuthorizationEndpoint == "" {
		return DeviceCode{}, noDeviceError{"this provider does not offer it"}
	}
	form := url.Values{"client_id": {clientID}, "scope": {Scopes}, "resource": {Resource(c.API)}}
	var dc DeviceCode
	if err := c.Form(ctx, pathOf(d.DeviceAuthorizationEndpoint), form, &dc); err != nil {
		var e *api.Error
		if errors.As(err, &e) {
			switch {
			case e.Code == "unauthorized_client":
				return DeviceCode{}, noDeviceError{"this provider does not allow it for this CLI"}
			case e.Code == "unsupported_grant_type" || e.Status == http.StatusNotFound:
				return DeviceCode{}, noDeviceError{"this provider does not offer it"}
			case e.Status == http.StatusTooManyRequests && e.RetryAfter > 0:
				return DeviceCode{}, fmt.Errorf("too many sign-in requests; try again in %d seconds", e.RetryAfter)
			case e.Status == http.StatusTooManyRequests:
				return DeviceCode{}, errors.New("too many sign-in requests; try again in a minute")
			}
		}
		return DeviceCode{}, fmt.Errorf("request a sign-in code: %w", err)
	}
	if dc.DeviceCode == "" || dc.UserCode == "" || dc.VerificationURI == "" {
		return DeviceCode{}, errors.New("request a sign-in code: the response is incomplete")
	}
	// RFC 8628 defaults the interval to 5 s; expires_in is required, so a
	// provider that omits it gets the usual 15 minutes.
	if dc.Interval <= 0 {
		dc.Interval = 5
	}
	if dc.ExpiresIn <= 0 {
		dc.ExpiresIn = 900
	}
	return dc, nil
}

// loginDevice shows the code and the page, then waits for the approval.
func loginDevice(ctx context.Context, c *api.Client, store *config.Store, d Discovery, clientID string, dc DeviceCode, opts LoginOptions) (config.Credential, error) {
	opts.Status("One-time code: %s", dc.UserCode)
	opts.Status("Open %s on any device, sign in, and enter the code.", dc.VerificationURI)
	page := dc.VerificationURIComplete
	if page == "" {
		page = dc.VerificationURI
	}
	if !opts.NoBrowser && opts.OpenBrowser != nil && !sshSession() {
		if err := opts.OpenBrowser(page); err != nil {
			opts.Status("Could not open a browser (%v); open the address above.", err)
		} else {
			opts.Status("Opened %s in your browser.", page)
		}
	}
	opts.Status("Waiting for the sign-in to be approved…")
	now := opts.Now()
	cred, err := pollDevice(ctx, c, opts, pathOf(d.TokenEndpoint), clientID, dc.DeviceCode,
		seconds(dc.Interval), now.Add(seconds(dc.ExpiresIn)), now.Add(opts.Timeout), true)
	if err != nil {
		return config.Credential{}, err
	}
	cred.ClientID = clientID
	if err := saveCredential(store, c.API, cred); err != nil {
		return cred, err
	}
	return cred, nil
}

// pollDevice asks the token endpoint for the tokens until the person approves
// or denies, the code expires, or the timeout passes (RFC 8628 §3.4–3.5).
// waitFirst waits one interval before the first request.
func pollDevice(ctx context.Context, c *api.Client, opts LoginOptions, tokenPath, clientID, deviceCode string, interval time.Duration, expiresAt, timeoutAt time.Time, waitFirst bool) (config.Credential, error) {
	form := url.Values{"grant_type": {DeviceGrantType}, "device_code": {deviceCode}, "client_id": {clientID}}
	deadline, deadlineErr := expiresAt, errDeviceExpired
	if timeoutAt.Before(expiresAt) {
		deadline, deadlineErr = timeoutAt, errDeviceTimeout
	}
	for wait := waitFirst; ; wait = true {
		if wait {
			pause := interval
			if left := deadline.Sub(opts.Now()); left < pause {
				pause = left
			}
			if pause > 0 {
				if err := opts.Sleep(ctx, pause); err != nil {
					return config.Credential{}, err
				}
			}
		}
		if !opts.Now().Before(deadline) {
			return config.Credential{}, deadlineErr
		}
		cred, err := exchange(ctx, c, tokenPath, form)
		if err == nil {
			return cred, nil
		}
		var e *api.Error
		if !errors.As(err, &e) {
			return config.Credential{}, err
		}
		switch e.Code {
		case "authorization_pending":
		case "slow_down":
			interval += 5 * time.Second
		case "access_denied":
			return config.Credential{}, errDeviceDenied
		case "expired_token":
			return config.Credential{}, errDeviceExpired
		default:
			return config.Credential{}, err
		}
	}
}

// StartPendingLogin requests a one-time code and stores the request so a
// later ResumeLogin, in another process, can finish the sign-in. It never
// falls back to the loopback flow, which cannot outlive the process.
func StartPendingLogin(ctx context.Context, c *api.Client, store *config.Store, cfg *config.Config, opts LoginOptions) (DeviceCode, error) {
	opts = opts.withDefaults()
	d, err := Discover(ctx, c)
	if err != nil {
		return DeviceCode{}, err
	}
	reg, err := EnsureClient(ctx, c, store, cfg, d)
	if err != nil {
		return DeviceCode{}, err
	}
	dc, err := requestDeviceCode(ctx, c, d, reg.ClientID)
	var unavailable noDeviceError
	if errors.As(err, &unavailable) {
		return DeviceCode{}, fmt.Errorf("--no-wait needs sign-in with a code, which is not available here (%s). Run `hearthroom auth login` on a machine with a browser, or set %s.", unavailable.why, EnvToken)
	}
	if err != nil {
		return DeviceCode{}, err
	}
	p := config.PendingLogin{
		API: c.API, ClientID: reg.ClientID, DeviceCode: dc.DeviceCode,
		UserCode: dc.UserCode, VerificationURI: dc.VerificationURI,
		Interval: dc.Interval, ExpiresAt: opts.Now().Add(seconds(dc.ExpiresIn)),
	}
	if err := store.SavePendingLogin(p); err != nil {
		return DeviceCode{}, err
	}
	return dc, nil
}

// ResumeLogin finishes the sign-in StartPendingLogin began: it polls at once,
// then at the interval, and stores the credential. The request is forgotten
// once it is used, denied or expired; a timeout keeps it for the next resume.
func ResumeLogin(ctx context.Context, c *api.Client, store *config.Store, opts LoginOptions) (config.Credential, error) {
	opts = opts.withDefaults()
	p, ok, err := store.LoadPendingLogin()
	if err != nil {
		return config.Credential{}, err
	}
	if !ok {
		return config.Credential{}, errResumeNothing
	}
	if p.API != c.API {
		return config.Credential{}, fmt.Errorf("The waiting sign-in is for %s, not %s. Run `hearthroom auth login --no-wait` again, or resume with --api %s.", p.API, c.API, p.API)
	}
	now := opts.Now()
	if !now.Before(p.ExpiresAt) {
		return config.Credential{}, forgetPending(store, errResumeExpired)
	}
	d, err := Discover(ctx, c)
	if err != nil {
		return config.Credential{}, err
	}
	if p.UserCode != "" && p.VerificationURI != "" {
		opts.Status("Waiting for code %s to be approved at %s", p.UserCode, p.VerificationURI)
	} else {
		opts.Status("Waiting for the sign-in to be approved…")
	}
	cred, err := pollDevice(ctx, c, opts, pathOf(d.TokenEndpoint), p.ClientID, p.DeviceCode,
		seconds(p.Interval), p.ExpiresAt, now.Add(opts.Timeout), false)
	var e *api.Error
	switch {
	case err == nil:
	case errors.Is(err, errDeviceTimeout):
		return config.Credential{}, errResumeTimeout
	case errors.Is(err, errDeviceExpired):
		return config.Credential{}, forgetPending(store, errResumeExpired)
	case errors.Is(err, errDeviceDenied):
		return config.Credential{}, forgetPending(store, err)
	case errors.As(err, &e) && e.Status >= 400 && e.Status < 500 && e.Status != http.StatusTooManyRequests:
		// The provider rejected the device code itself; it will not work again.
		return config.Credential{}, forgetPending(store, fmt.Errorf("%w; run `hearthroom auth login --no-wait` again", err))
	default:
		return config.Credential{}, err
	}
	cred.ClientID = p.ClientID
	if err := saveCredential(store, c.API, cred); err != nil {
		return cred, err
	}
	// The device code is spent; polling with it again would count as a replay
	// and revoke the tokens just issued.
	if err := store.DeletePendingLogin(); err != nil {
		return cred, err
	}
	return cred, nil
}

func forgetPending(store *config.Store, err error) error {
	if derr := store.DeletePendingLogin(); derr != nil {
		return errors.Join(err, derr)
	}
	return err
}

// sshSession reports whether the CLI runs over SSH, where a browser would
// open on the remote machine instead of in front of the person.
func sshSession() bool {
	for _, k := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func seconds(n int64) time.Duration { return time.Duration(n) * time.Second }
