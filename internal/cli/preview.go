package cli

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/check"
	"github.com/hearthroom/cli/internal/output"
	"github.com/hearthroom/cli/internal/preview"
)

func (a *App) cardPreview() *cobra.Command {
	var port int
	var open bool
	var shellDir string
	var runCheck bool
	var fromHistory []string
	var sizes string
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

--check gives an agent without a browser the same evidence: it drives headless Chrome
through the harness, streams the sample replies, and for a fixed set of states (phone
and desktop, dark and light, rules on and off, the first and the last sample) saves a
screenshot under preview/shots/, reads the facts a picture does not show (when the
status panel was drawn after the reply finished, whether the phone width scrolls
sideways, choice buttons, text under 12px, the card scripts' console errors) and writes
preview/shots/findings.json plus a contact sheet, preview/shots/contact.png, with every
shot and its numbers. Errors (a block written but never drawn as a panel, sideways
overflow at phone width, a script error) exit 1; warnings and info exit 0. Chrome,
Chromium or Edge is found on PATH or in the usual install locations, or named with
HEARTHROOM_CHROME. --from-history streams real replies from a "hearthroom play
--history" file instead of preview/replies.md; --sizes overrides the viewports.
Nothing under preview/ is ever pushed: card push sends the card files and the assets the
card references, so the shots stay on this machine.

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
			if runCheck {
				return a.previewCheck(f, shellDir, port, fromHistory, sizes)
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
	c.Flags().BoolVar(&runCheck, "check", false, "drive headless Chrome: screenshots, facts and findings under preview/shots/, then exit")
	c.Flags().StringArrayVar(&fromHistory, "from-history", nil, "with --check: stream the replies in this play --history file instead of preview/replies.md (repeatable)")
	c.Flags().StringVar(&sizes, "sizes", "", "with --check: viewports as WxH,WxH (default 390x844,1280x800)")
	return c
}

// previewCheck is --check: serve, drive, derive, report.
func (a *App) previewCheck(f *card.Folder, shellDir string, port int, fromHistory []string, sizesSpec string) error {
	chrome, err := preview.FindChrome()
	if err != nil {
		return output.Exitf(2, "%v; install Chrome, Chromium or Edge, set HEARTHROOM_CHROME to its executable, or use \"card preview --open\" and \"card render --json\" instead", err)
	}
	sz, err := preview.ParseSizes(sizesSpec)
	if err != nil {
		return output.Exit(2, err)
	}
	report := &preview.Report{Card: f.Dir, ShotsDir: filepath.Join(f.Dir, "preview", "shots")}
	var samples []string
	if len(fromHistory) > 0 {
		for _, path := range fromHistory {
			b, err := os.ReadFile(path)
			if err != nil {
				return output.Exit(2, err)
			}
			samples = append(samples, check.RepliesFrom(string(b))...)
		}
		if len(samples) == 0 {
			return output.Exitf(2, "no replies found in %s", strings.Join(fromHistory, ", "))
		}
		report.Notes = append(report.Notes, "samples are the replies from "+strings.Join(fromHistory, ", "))
	} else if b, err := os.ReadFile(filepath.Join(f.Dir, "preview", "replies.md")); err == nil {
		samples = preview.SplitSamples(string(b))
	}
	if len(samples) == 0 {
		one, note := preview.Synthesize(f.Manifest.OutputContract)
		samples = []string{one}
		report.Notes = append(report.Notes, note)
	}
	states := preview.Plan(sz, samples)
	var results []preview.StateResult
	srv, err := preview.Listen(shellDir, f.Dir, port, preview.Options{
		ShotsDir: report.ShotsDir,
		Samples:  samples,
		Contact:  func() string { return preview.ContactHTML(results) },
	})
	if err != nil {
		return output.Exit(2, err)
	}
	defer srv.Close()
	ctx := context.Background()
	results, err = preview.Run(ctx, preview.DriveOptions{Chrome: chrome, Origin: srv.Origin, States: states, ShotsDir: report.ShotsDir, Progress: func(name string) {
		if !a.Out.JSON {
			a.Out.Note("%s", name)
		}
	}})
	if err != nil {
		return output.Exit(2, err)
	}
	report.States = results
	decl := check.ReadDeclarations(readFileOr(filepath.Join(f.Dir, card.ReadmeFile)))
	report.Findings = preview.Derive(preview.Declared{UIRole: decl.UIRole}, results)
	for _, n := range report.Notes {
		report.Findings = append(report.Findings, preview.Finding{Level: "info", Where: "preview", Msg: n})
	}
	contact := filepath.Join(report.ShotsDir, "contact.png")
	if err := preview.Contact(ctx, chrome, srv.Origin, contact); err != nil {
		report.Findings = append(report.Findings, preview.Finding{Level: "warning", Where: "preview", Msg: "contact sheet not written: " + err.Error()})
	} else {
		report.Contact = contact
	}
	report.Status = "ok"
	if report.Errors() > 0 {
		report.Status = "error"
	}
	if b, err := preview.MarshalReport(report); err == nil {
		_ = os.WriteFile(filepath.Join(report.ShotsDir, "findings.json"), append(b, '\n'), 0o644)
	}
	if a.Out.JSON {
		if err := a.Out.JSONValue(report); err != nil {
			return err
		}
	} else {
		for _, fd := range report.Findings {
			mark := "·"
			switch fd.Level {
			case "error":
				mark = "✖"
			case "warning":
				mark = "△"
			}
			a.Out.Line("%s %s: %s", mark, fd.Where, fd.Msg)
		}
		for _, st := range report.States {
			if st.Shot != "" {
				a.Out.Line("shot:    %s", filepath.Join(report.ShotsDir, st.Shot))
			}
		}
		if report.Contact != "" {
			a.Out.Line("contact: %s", report.Contact)
		}
		a.Out.Line("report:  %s", filepath.Join(report.ShotsDir, "findings.json"))
		if n := report.Errors(); n > 0 {
			a.Out.Line("✖ %d error(s)", n)
		} else {
			a.Out.Line("✔ no errors")
		}
	}
	if report.Errors() > 0 {
		return output.ExitQuiet(1, "preview check found errors")
	}
	return nil
}

func readFileOr(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
