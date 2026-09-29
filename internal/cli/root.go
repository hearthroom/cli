// Package cli wires the cobra command tree. Each verb group lives in its own
// file and receives the shared *App.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/auth"
	"github.com/hearthroom/cli/internal/config"
	"github.com/hearthroom/cli/internal/output"
	"github.com/hearthroom/cli/internal/update"
)

// BuildInfo is stamped by the build.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// App carries the per-invocation state shared by all commands.
type App struct {
	Info    BuildInfo
	Out     output.Printer
	Store   *config.Store
	Cfg     config.Config
	API     string
	Site    string
	jsonOut bool

	flagAPI, flagSite, flagConfigDir string
	client                           *api.Client
	updates                          <-chan update.Result
}

// NewRoot builds the full command tree without executing it. Documentation
// generators use it so the published manual is exactly what --help says.
func NewRoot(info BuildInfo) *cobra.Command {
	app := &App{Info: info, Out: output.Printer{Out: os.Stdout, Err: os.Stderr}}
	return app.rootCommand()
}

// Main runs the CLI and returns the process exit code.
func Main(ctx context.Context, info BuildInfo, args []string) int {
	app := &App{Info: info, Out: output.Printer{Out: os.Stdout, Err: os.Stderr}}
	root := app.rootCommand()
	root.SetArgs(args)
	code := 0
	if err := root.ExecuteContext(ctx); err != nil {
		code = app.Out.Fail(err)
	}
	app.printUpdateNotice()
	return code
}

// startUpdateCheck runs the daily release check in the background so it
// costs the command nothing; printUpdateNotice reads the answer at exit.
func (a *App) startUpdateCheck(ctx context.Context) {
	if !update.Enabled(a.Info.Version, a.Out.JSON, isTerminal(os.Stderr)) {
		return
	}
	a.updates = update.Start(ctx, a.Store.Dir, a.Info.Version, 2*time.Second)
}

func (a *App) printUpdateNotice() {
	if a.updates == nil {
		return
	}
	select {
	case r, ok := <-a.updates:
		if ok && r.Newer() {
			exe, _ := os.Executable()
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, update.Notice(r, exe))
		}
	case <-time.After(1500 * time.Millisecond):
	}
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func (a *App) rootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "hearthroom",
		Short: "Write, push, validate and import character cards for Hearthroom",
		Long: `hearthroom is the command-line client for Hearthroom.

Cards live in folders you can version and hand to an AI agent. Push a folder to
a private trial card on the connected provider, validate it, play it, pull it
back, or import cards from SillyTavern and MMD. Every command accepts --json.

Agents: start at https://sukisuki.ai/llms.txt for the card model, what needs
sign-in or spends credits, and the Markdown sources of the guide and API reference.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return a.init(cmd)
		},
	}
	pf := root.PersistentFlags()
	pf.BoolVar(&a.jsonOut, "json", false, "machine-readable JSON output")
	pf.StringVar(&a.flagAPI, "api", "", "provider API base (default from config or "+config.DefaultAPI+")")
	pf.StringVar(&a.flagSite, "site", "", "community site (default from config or "+config.DefaultSite+")")
	pf.StringVar(&a.flagConfigDir, "config-dir", "", "config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)")

	root.AddCommand(
		a.versionCommand(),
		a.authCommand(),
	)
	for _, reg := range extraCommands {
		root.AddCommand(reg(a)...)
	}
	// cobra adds `completion` lazily at execution; create it now so the
	// install/uninstall subcommands can hang off it.
	root.InitDefaultCompletionCmd()
	a.addCompletionInstall(root)
	return root
}

func (a *App) init(cmd *cobra.Command) error {
	a.Out.JSON = a.jsonOut
	store, err := config.Open(a.flagConfigDir)
	if err != nil {
		return err
	}
	a.Store = store
	cfg, err := store.LoadConfig()
	if err != nil {
		return err
	}
	a.Cfg = cfg
	a.API = config.NormalizeBase(firstNonEmpty(a.flagAPI, os.Getenv("HEARTHROOM_API"), cfg.API, config.DefaultAPI))
	a.Site = config.NormalizeBase(firstNonEmpty(a.flagSite, os.Getenv("HEARTHROOM_SITE"), cfg.Site, config.DefaultSite))
	if !strings.HasPrefix(a.API, "http://") && !strings.HasPrefix(a.API, "https://") {
		return fmt.Errorf("--api must be an http(s) URL, got %q", a.API)
	}
	if cmd.Name() != "upgrade" && cmd.Name() != "version" {
		a.startUpdateCheck(cmd.Context())
	}
	return nil
}

// Client returns the API client, built lazily.
func (a *App) Client() *api.Client {
	if a.client == nil {
		a.client = api.New(a.API, a.Site, a.userAgent(), nil)
		a.client.Token = auth.Source(a.Store, a.client)
	}
	return a.client
}

func (a *App) userAgent() string {
	return fmt.Sprintf("hearthroom-cli/%s (%s/%s)", a.Info.Version, runtime.GOOS, runtime.GOARCH)
}

// RequireAuth fails early with a clear message when no token is available.
func (a *App) RequireAuth() error {
	if !auth.HasCredential(a.Store, a.API) {
		return output.Exit(4, auth.ErrNotLoggedIn)
	}
	return nil
}

func (a *App) versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if a.Out.JSON {
				return a.Out.JSONValue(map[string]string{
					"version": a.Info.Version, "commit": a.Info.Commit, "date": a.Info.Date,
					"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
				})
			}
			a.Out.Line("hearthroom %s (%s, %s) %s/%s", a.Info.Version, a.Info.Commit, a.Info.Date, runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
}

// openBrowser launches the default browser without external dependencies.
func openBrowser(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		if _, err := exec.LookPath("xdg-open"); err != nil {
			return errors.New("xdg-open is not available")
		}
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
