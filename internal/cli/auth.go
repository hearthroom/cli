package cli

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/auth"
	"github.com/hearthroom/cli/internal/output"
)

// extraCommands lets later files register verb groups without touching root.
var extraCommands []func(a *App) []*cobra.Command

func (a *App) authCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Sign in to the connected card provider",
	}
	var noBrowser bool
	var timeout time.Duration
	login := &cobra.Command{
		Use:   "login",
		Short: "Sign in with your browser (OAuth with PKCE)",
		Long: `Signs in to the provider behind --api. The CLI registers itself as an OAuth
client once, opens the provider's sign-in page in your browser, and receives
the result on a loopback port. Tokens are stored under the config directory
with owner-only permissions and refreshed automatically.

For scripts and CI, set HEARTHROOM_TOKEN instead of signing in.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := auth.LoginOptions{
				OpenBrowser: openBrowser,
				NoBrowser:   noBrowser,
				Status:      a.Out.Note,
				Timeout:     timeout,
			}
			cred, err := auth.Login(cmd.Context(), a.Client(), a.Store, &a.Cfg, opts)
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
	login.Flags().BoolVar(&noBrowser, "no-browser", false, "print the sign-in URL instead of opening a browser")
	login.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "how long to wait for the browser")

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
