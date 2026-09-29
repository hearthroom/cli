package cli

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/output"
	"github.com/hearthroom/cli/internal/sync"
)

func init() {
	extraCommands = append(extraCommands, func(a *App) []*cobra.Command { return []*cobra.Command{a.cardCommand()} })
}

func (a *App) cardCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "card",
		Short: "Work with card folders: init, push, validate, pull, import, list",
	}
	cmd.AddCommand(a.cardInit(), a.cardPush(), a.cardValidate(), a.cardPull(), a.cardList(), a.cardStatus())
	return cmd
}

func dirArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return "."
}

func loadFolder(args []string) (*card.Folder, error) {
	dir := dirArg(args)
	f, err := card.Load(dir)
	if err != nil {
		return nil, output.Exit(2, err)
	}
	return f, nil
}

func (a *App) cardInit() *cobra.Command {
	var name string
	c := &cobra.Command{
		Use:   "init <dir>",
		Short: "Create a new card folder",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := args[0]
			if name == "" {
				name = filepath.Base(absPath(dir))
			}
			f, err := card.Init(dir, name)
			if err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(map[string]any{"dir": dir, "name": f.Manifest.Name, "files": []string{card.ManifestFile, card.DefinitionFile, card.WelcomeFile}})
			}
			a.Out.Line("Created %s", dir)
			a.Out.Line("  %-14s name, tags and short fields", card.ManifestFile)
			a.Out.Line("  %-14s the private definition", card.DefinitionFile)
			a.Out.Line("  %-14s the opening message", card.WelcomeFile)
			a.Out.Line("Next: edit the files, then `hearthroom card push %s`", dir)
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "card name (default: folder name)")
	return c
}

func (a *App) cardPush() *cobra.Command {
	var opts sync.PushOptions
	var validate bool
	c := &cobra.Command{
		Use:   "push [dir]",
		Short: "Sync the folder to the provider (a private trial card by default)",
		Long: `Uploads referenced assets, then writes the card in sections. Without --to
the target is an auto-expiring private trial card keyed to this folder, which
is the fastest way to play what you just edited. With --to <roleId> (or
--create) the folder is written to a private card you own.

Only sections whose content changed since the last push are sent.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			f, err := loadFolder(args)
			if err != nil {
				return err
			}
			if problems := sync.LocalCheck(f); len(problems) > 0 && f.State.RoleID == "" && !opts.Force {
				// A first push with an empty definition or opening is almost
				// always a mistake; later pushes may be partial on purpose.
				for _, p := range problems {
					a.Out.Note("  - %s", p)
				}
				return output.Exitf(2, "the folder is not ready to push (use --force to push anyway)")
			}
			res, err := sync.Push(cmd.Context(), a.Client(), f, opts)
			if err != nil {
				return err
			}
			var report *sync.Report
			if validate && !opts.DryRun && res.RoleID != "" {
				report, err = sync.Validate(cmd.Context(), a.Client(), res.RoleID)
				if err != nil {
					return err
				}
			}
			if a.Out.JSON {
				out := map[string]any{"push": res}
				if report != nil {
					out["validation"] = report
				}
				if err := a.Out.JSONValue(out); err != nil {
					return err
				}
			} else {
				a.printPush(res)
				if report != nil {
					a.printReport(report)
				}
			}
			if res.AssetError {
				return output.Exitf(1, "some assets failed to upload")
			}
			if report != nil && report.Status == "blocker" {
				return output.Exitf(2, "validation found blockers")
			}
			return nil
		},
	}
	c.Flags().StringVar(&opts.To, "to", "", "write to an owned private card by id instead of a trial card")
	c.Flags().BoolVar(&opts.Create, "create", false, "create a new private card and write to it")
	c.Flags().BoolVar(&opts.Evict, "evict", false, "free the least recently used trial slot when all are in use")
	c.Flags().BoolVar(&opts.DryRun, "dry-run", false, "show what would be sent without sending")
	c.Flags().BoolVar(&opts.Force, "force", false, "send every section even if unchanged; also skips the readiness check")
	c.Flags().BoolVar(&opts.SkipMedia, "skip-media", false, "do not upload assets; keep previously uploaded URLs")
	c.Flags().BoolVar(&validate, "validate", false, "run the server validation after pushing")
	return c
}

func (a *App) printPush(res *sync.PushResult) {
	verb := "Pushed"
	if res.DryRun {
		verb = "Would push"
	}
	target := "trial card"
	if res.Target == "owned" {
		target = "card"
	}
	if res.Created {
		target = "new " + target
	}
	a.Out.Line("%s to %s %s", verb, target, res.RoleID)
	for _, o := range res.Assets {
		switch {
		case o.Err != "":
			a.Out.Line("  asset %-30s FAILED: %s", o.Path, o.Err)
		case o.Uploaded && res.DryRun:
			a.Out.Line("  asset %-30s would upload", o.Path)
		case o.Uploaded:
			a.Out.Line("  asset %-30s uploaded", o.Path)
		default:
			a.Out.Line("  asset %-30s unchanged", o.Path)
		}
	}
	if len(res.Changed) > 0 {
		a.Out.Line("  sections sent:      %s", strings.Join(res.Changed, ", "))
	}
	if len(res.Unchanged) > 0 {
		a.Out.Line("  sections unchanged: %s", strings.Join(res.Unchanged, ", "))
	}
	for _, s := range res.Skipped {
		a.Out.Line("  skipped: %s", s)
	}
	if res.ExpiresAt != "" {
		a.Out.Line("  trial expires:      %s", res.ExpiresAt)
	}
	if res.Slots != nil {
		a.Out.Line("  trial slots:        %v of %v used", res.Slots["used"], res.Slots["max"])
	}
	if res.RoleID != "" && !res.DryRun {
		a.Out.Line("Play: %s/play/%s", a.Site, res.RoleID)
	}
}

func (a *App) cardValidate() *cobra.Command {
	var push, strict bool
	c := &cobra.Command{
		Use:   "validate [dir]",
		Short: "Show the provider's pre-publish validation for the pushed card",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := loadFolder(args)
			if err != nil {
				return err
			}
			problems := sync.LocalCheck(f)
			if len(problems) > 0 && !push {
				if a.Out.JSON {
					_ = a.Out.JSONValue(map[string]any{"status": "blocker", "local": problems})
				} else {
					a.Out.Line("Local checks failed:")
					for _, p := range problems {
						a.Out.Line("  - %s", p)
					}
				}
				return output.Exitf(2, "fix the local problems first")
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			if push {
				if _, err := sync.Push(cmd.Context(), a.Client(), f, sync.PushOptions{}); err != nil {
					return err
				}
			}
			if f.State.RoleID == "" {
				return output.Exit(2, sync.ErrNoTarget)
			}
			report, err := sync.Validate(cmd.Context(), a.Client(), f.State.RoleID)
			if err != nil {
				return err
			}
			if a.Out.JSON {
				if err := a.Out.JSONValue(report); err != nil {
					return err
				}
			} else {
				a.printReport(report)
			}
			switch report.Status {
			case "blocker":
				return output.Exitf(2, "validation found blockers")
			case "warning":
				if strict {
					return output.Exitf(1, "validation produced warnings (--strict)")
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&push, "push", false, "push the folder first so the report reflects the current files")
	c.Flags().BoolVar(&strict, "strict", false, "exit non-zero on warnings as well as blockers")
	return c
}

func (a *App) printReport(r *sync.Report) {
	a.Out.Line("Validation: %s (score %.1f)", strings.ToUpper(r.Status), r.Score)
	if len(r.Blockers) > 0 {
		a.Out.Line("Blockers:")
		for _, b := range r.Blockers {
			a.Out.Line("  - %s", b)
		}
	}
	if len(r.Warnings) > 0 {
		a.Out.Line("Warnings:")
		for _, w := range r.Warnings {
			a.Out.Line("  - %s", w)
		}
	}
	if len(r.SuggestedFixes) > 0 {
		a.Out.Line("Suggested fixes:")
		for _, s := range r.SuggestedFixes {
			a.Out.Line("  - %s", s)
		}
	}
	if tb := r.TokenBudget; tb != nil {
		a.Out.Line("Budget (~%d tokens):", tb.EstimatedTokens)
		row := func(label string, n int64, limitKey string) {
			if lim, ok := tb.Limits[limitKey]; ok {
				a.Out.Line("  %-20s %6d / %d", label, n, lim)
			} else {
				a.Out.Line("  %-20s %6d", label, n)
			}
		}
		row("summary", tb.RoleDescChars, "roleDescMaxChars")
		row("definition", tb.RoleDetailDescChars, "roleDetailDescMaxChars")
		row("opening", tb.RoleWelcomeChars, "roleWelcomeMaxChars")
		row("output contract", tb.RoleOutputContractChars, "roleOutputContractMaxChars")
		row("custom instructions", tb.CustomInstructionsChars, "customInstructionsMaxChars")
		for _, g := range tb.Guidance {
			a.Out.Line("  note: %s", g)
		}
	}
}

func (a *App) cardPull() *cobra.Command {
	var opts sync.PullOptions
	c := &cobra.Command{
		Use:   "pull <roleId> [dir]",
		Short: "Write one of your cards to a folder",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			roleID := args[0]
			dir := roleID
			if len(args) > 1 {
				dir = args[1]
			}
			res, err := sync.Pull(cmd.Context(), a.Client(), roleID, dir, opts)
			if err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(res)
			}
			a.Out.Line("Pulled %s into %s", res.RoleID, res.Dir)
			for _, f := range res.Files {
				a.Out.Line("  %s", f)
			}
			for _, d := range res.Downloaded {
				a.Out.Line("  %s (downloaded)", d)
			}
			for _, w := range res.Warnings {
				a.Out.Note("warning: %s", w)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&opts.Download, "download", false, "download referenced media into assets/")
	c.Flags().BoolVar(&opts.Force, "force", false, "overwrite an existing card folder")
	return c
}

type mineOutput struct {
	HasNextPage bool `json:"hasNextPage"`
	Total       int  `json:"total"`
	RoleList    []struct {
		RoleID         string `json:"characterRoleId"`
		RoleName       string `json:"roleName"`
		RoleDesc       string `json:"roleDesc"`
		Visibility     string `json:"roleVisibility"`
		ReviewStatus   string `json:"reviewStatus"`
		CreationMethod string `json:"creationMethod"`
		Language       string `json:"language"`
		TalkNum        int64  `json:"talkNum"`
	} `json:"roleList"`
}

func (a *App) cardList() *cobra.Command {
	var q string
	var page, size int
	c := &cobra.Command{
		Use:   "list",
		Short: "List your cards on the provider",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			query := url.Values{"pageNum": {strconv.Itoa(page)}, "pageSize": {strconv.Itoa(size)}}
			if q != "" {
				query.Set("q", q)
			}
			var out mineOutput
			if err := a.Client().OpenGet(cmd.Context(), "/role/mine", query, &out); err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(out)
			}
			rows := make([][]string, 0, len(out.RoleList))
			for _, r := range out.RoleList {
				rows = append(rows, []string{r.RoleID, r.RoleName, r.Visibility, r.ReviewStatus, r.CreationMethod, r.Language, strconv.FormatInt(r.TalkNum, 10)})
			}
			a.Out.Table([]string{"ID", "NAME", "VISIBILITY", "REVIEW", "SOURCE", "LANG", "CHATS"}, rows)
			if out.HasNextPage {
				a.Out.Line("More: --page %d", page+1)
			}
			return nil
		},
	}
	c.Flags().StringVar(&q, "q", "", "filter by name")
	c.Flags().IntVar(&page, "page", 1, "page number")
	c.Flags().IntVar(&size, "limit", 50, "entries per page")
	return c
}

func (a *App) cardStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status [dir]",
		Short: "Show what a folder is linked to and which sections changed",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := loadFolder(args)
			if err != nil {
				return err
			}
			urls := map[string]string{}
			for p, as := range f.State.Assets {
				urls[p] = as.URL
			}
			p, err := f.Build(urls)
			if err != nil {
				return err
			}
			digests := p.Digests()
			changed := []string{}
			for _, s := range card.Sections {
				if d, ok := digests[s]; ok && f.State.LocalHashes[s] != d {
					changed = append(changed, s)
				}
			}
			present, missing := f.AssetRefs()
			if a.Out.JSON {
				return a.Out.JSONValue(map[string]any{"state": f.State, "changedSections": changed, "assets": present, "missingAssets": missing, "local": sync.LocalCheck(f)})
			}
			if f.State.RoleID == "" {
				a.Out.Line("Not pushed yet.")
			} else {
				a.Out.Line("Linked to %s card %s on %s", f.State.Target, f.State.RoleID, f.State.API)
			}
			if len(changed) > 0 {
				a.Out.Line("Changed since last push: %s", strings.Join(changed, ", "))
			} else if f.State.RoleID != "" {
				a.Out.Line("No changes since last push.")
			}
			for _, m := range missing {
				a.Out.Line("Missing asset: %s", m)
			}
			for _, pr := range sync.LocalCheck(f) {
				a.Out.Line("Check: %s", pr)
			}
			return nil
		},
	}
}

func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// exists is a small helper used by import.
func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

var _ = fmt.Sprintf
