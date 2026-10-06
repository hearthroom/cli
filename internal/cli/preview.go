package cli

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/output"
	"github.com/hearthroom/cli/internal/preview"
)

func (a *App) cardPreview() *cobra.Command {
	var port int
	var open bool
	var shellDir string
	c := &cobra.Command{
		Use:   "preview [dir]",
		Short: "Open the folder in the real sandbox chat shell with a fake host, offline",
		Long: `Serves the card folder to the community site's own sandbox shell on a local port,
with this CLI playing the host: the opening, display rules and scripts run exactly as
on the play page (the real sdk, sanitizer, Markdown and event order), without a push
and without credits. The page streams sample replies from preview/replies.md (one per
"## " heading), switches conversation, re-runs scripts, offers phone, landscape,
unfolded, tablet and desktop sizes, both themes, and "rules off" (?rules=off) to read
the card as plain text. It is not the host's own render path, the model or a device.

The shell is fetched once from the card's sandbox origin on the configured site
(c<roleId>.<site host>/sandbox/) and cached by content hash; --shell <dir> uses a
local build of the shell instead. Nothing is sent anywhere else.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := loadFolder(args)
			if err != nil {
				return err
			}
			if shellDir == "" {
				origin, err := preview.SandboxOrigin(a.Site, f.State.RoleID)
				if err != nil {
					return output.Exit(2, err)
				}
				cacheRoot, err := os.UserCacheDir()
				if err != nil {
					cacheRoot = os.TempDir()
				}
				cacheRoot = filepath.Join(cacheRoot, "hearthroom", "sandbox")
				dir, fetched, err := preview.Fetch(&http.Client{Timeout: 30 * time.Second}, origin, cacheRoot)
				if err != nil {
					return output.Exitf(2, "could not fetch the sandbox shell from %s: %v (pass --shell <dist-sandbox dir>)", origin, err)
				}
				if fetched {
					a.Out.Note("fetched the sandbox shell from %s", origin)
				}
				shellDir = dir
			} else if _, err := os.Stat(filepath.Join(shellDir, "sandbox.js")); err != nil {
				return output.Exitf(2, "%s does not contain sandbox.js", shellDir)
			}
			if _, err := os.Stat(filepath.Join(f.Dir, card.RulesFile)); errors.Is(err, os.ErrNotExist) {
				a.Out.Note("%s has no rules.json; the preview shows plain text", f.Dir)
			}
			return preview.Serve(shellDir, f.Dir, port, func(url string) {
				if a.Out.JSON {
					_ = a.Out.JSONValue(map[string]any{"url": url, "shell": shellDir, "card": f.Dir})
				} else {
					a.Out.Line("card:    %s", f.Dir)
					a.Out.Line("preview: %s", url)
					a.Out.Line("Ctrl-C stops the server.")
				}
				if open {
					_ = openBrowser(url)
				}
			})
		},
	}
	c.Flags().IntVar(&port, "port", 4173, "local port (0 picks a free one)")
	c.Flags().BoolVar(&open, "open", false, "open the preview in the default browser")
	c.Flags().StringVar(&shellDir, "shell", "", "serve a local sandbox shell build (dist-sandbox) instead of fetching the site's")
	return c
}
