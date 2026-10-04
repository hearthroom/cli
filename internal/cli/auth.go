package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/auth"
	"github.com/hearthroom/cli/internal/config"
	"github.com/hearthroom/cli/internal/output"
)

// extraCommands lets later files register verb groups without touching root.
var extraCommands []func(a *App) []*cobra.Command

func (a *App) authCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Sign in to the connected card provider",
	}
	var noBrowser, noWait, resume bool
	var timeout time.Duration
	login := &cobra.Command{
		Use:   "login",
		Short: "Sign in with a one-time code (works over SSH)",
		Long: `Signs in to the provider behind --api. The CLI prints a one-time code and a
web address. Open the address on any device (this computer, a laptop, a
phone), sign in, and enter the code; the CLI picks up the sign-in as soon as
you approve it. On a desktop the page opens in your browser; type the code
there. Over SSH, or with --no-browser, nothing is opened.

If the provider does not offer sign-in with a code, the CLI says so and signs
in through a browser on this machine instead (OAuth with PKCE, receiving the
result on a loopback port).

Agents that only see a command's output after it exits can split the wait:
--no-wait prints the code and address (one JSON object with --json) and exits;
after the person has approved, --resume finishes the sign-in.

Tokens are stored under the config directory with owner-only permissions and
refreshed automatically. For scripts and CI, set HEARTHROOM_TOKEN instead of
signing in.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := auth.LoginOptions{
				OpenBrowser: openBrowser,
				NoBrowser:   noBrowser,
				Status:      a.Out.Note,
				Timeout:     timeout,
			}
			if noWait {
				dc, err := auth.StartPendingLogin(cmd.Context(), a.Client(), a.Store, &a.Cfg, opts)
				if err != nil {
					return err
				}
				return a.printPendingLogin(dc)
			}
			var cred config.Credential
			var err error
			switch {
			case resume:
				cred, err = auth.ResumeLogin(cmd.Context(), a.Client(), a.Store, opts)
			default:
				cred, err = auth.Login(cmd.Context(), a.Client(), a.Store, &a.Cfg, opts)
			}
			if err != nil {
				return err
			}
			me, err := a.whoami(cmd.Context())
			if err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(map[string]any{"api": a.API, "scope": cred.Scope, "expiresAt": cred.ExpiresAt, "me": me})
			}
			a.Out.Line("Signed in to %s as %s.", a.API, displayName(me))
			return nil
		},
	}
	login.Flags().BoolVar(&noBrowser, "no-browser", false, "do not open a browser; just print the code and address")
	login.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "how long to wait for the sign-in to be approved")
	login.Flags().BoolVar(&noWait, "no-wait", false, "print the code and address, then exit; finish later with --resume")
	login.Flags().BoolVar(&resume, "resume", false, "finish the sign-in started with --no-wait")
	login.MarkFlagsMutuallyExclusive("no-wait", "resume")

	logout := &cobra.Command{
		Use:   "logout",
		Short: "Forget the stored sign-in for this provider",
		RunE: func(cmd *cobra.Command, _ []string) error {
			removed, err := auth.Logout(cmd.Context(), a.Client(), a.Store)
			if err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(map[string]any{"api": a.API, "removed": removed})
			}
			if removed {
				a.Out.Line("Signed out of %s.", a.API)
			} else {
				a.Out.Line("No stored sign-in for %s.", a.API)
			}
			return nil
		},
	}

	status := &cobra.Command{
		Use:   "status",
		Short: "Show who you are signed in as",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			me, err := a.whoami(cmd.Context())
			if err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(map[string]any{"api": a.API, "site": a.Site, "me": me})
			}
			a.Out.Line("Provider: %s", a.API)
			a.Out.Line("Site:     %s", a.Site)
			a.Out.Line("Account:  %s (%s)", displayName(me), me.AccountType)
			return nil
		},
	}
	cmd.AddCommand(login, logout, status)
	return cmd
}

// printPendingLogin shows what `auth login --no-wait` started: the code and
// address are the result here, so they go to stdout.
func (a *App) printPendingLogin(dc auth.DeviceCode) error {
	if a.Out.JSON {
		return a.Out.JSONValue(struct {
			UserCode        string `json:"user_code"`
			VerificationURI string `json:"verification_uri"`
			ExpiresIn       int64  `json:"expires_in"`
			Interval        int64  `json:"interval"`
		}{dc.UserCode, dc.VerificationURI, dc.ExpiresIn, dc.Interval})
	}
	a.Out.Line("First copy your one-time code: %s", dc.UserCode)
	a.Out.Line("Then open %s on any device, sign in, and enter the code.", dc.VerificationURI)
	a.Out.Line("After approving, run `hearthroom auth login --resume`. The code expires in %s.", minutes(dc.ExpiresIn))
	return nil
}

func minutes(secs int64) string {
	if m := (secs + 59) / 60; m != 1 {
		return fmt.Sprintf("%d minutes", m)
	}
	return "1 minute"
}

// Me is GET /open/v1/me. accountNumId is kept only for --json output; the
// human-readable lines identify the account by email, then nickname.
type Me struct {
	AccountNumID int64  `json:"accountNumId"`
	NickName     string `json:"nickName"`
	Email        string `json:"email,omitempty"`
	Avatar       string `json:"avatar"`
	AccountType  string `json:"accountType"`
}

func (a *App) whoami(ctx context.Context) (Me, error) {
	var me Me
	if err := a.Client().OpenGet(ctx, "/me", nil, &me); err != nil {
		return me, output.Exit(4, err)
	}
	if me.AccountType == "" {
		me.AccountType = "user"
	}
	return me, nil
}

// displayName prefers the email (stable, always present when the email.read
// scope was granted), then the nickname; it never shows numeric ids.
func displayName(me Me) string {
	switch {
	case me.Email != "" && me.NickName != "":
		return me.Email + " (" + me.NickName + ")"
	case me.Email != "":
		return me.Email
	case me.NickName != "":
		return me.NickName
	}
	return "this account (sign in again to see the email)"
}
