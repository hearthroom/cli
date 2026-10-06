package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/check"
	"github.com/hearthroom/cli/internal/output"
)

func (a *App) cardCheck() *cobra.Command {
	var replay []string
	c := &cobra.Command{
		Use:   "check [dir] [--replay file...]",
		Short: "Local checks of the folder before a push: rules, markers, sandbox API use, protocol health",
		Long: `Reads the folder and reports what the provider's validate and render cannot see,
without signing in or sending anything:

  - rules.json: invalid or empty-matching patterns, flags outside gimsuy, blank find,
    duplicate ids, a replacement over 128 KiB (UTF-8 bytes) or a set over 32 MiB,
    sdk capabilities and event names the sandbox page does not have, sdk.off/once,
    sdk.vars, module syntax, an await before sdk.message.send, invalid save keys,
    attributes the sanitizer strips (data-*, aria-*, role; on* inside <svg>), tags with
    Chinese characters, hc-* components, {{random:a|b}}, asset paths built at runtime
    or pointing at missing files, and the sandbox API on a classic-page card;
  - render rules are not generation rules: a marker a rule consumes must be something
    the definition, the output contract, a constant Lorebook entry or an opening tells
    the model to write, or the panel appears once and never updates;
  - README.md (never sent) declarations: uiRole: assist | core and, for a core card,
    statusOverheadThreshold.

--replay reads replies ("hearthroom play --history" text or --json output,
preview/replies.md with "## " headings, or paragraphs) and reports protocol health:
how often the [status] block is written, missing closers, full-width punctuation,
the status overhead ratio against the declared threshold (15% by default), and the
keys the model forgets. Every fact comes from the sandbox contract embedded in this
build. Exit code 1 when there are errors; warnings do not fail.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := dirArg(args)
			if _, err := os.Stat(filepath.Join(dir, card.ManifestFile)); err != nil {
				return output.Exit(2, card.ErrNotCardFolder)
			}
			res, err := check.Card(dir)
			if err != nil {
				return err
			}
			if len(replay) > 0 {
				var replies []string
				for _, f := range replay {
					b, err := os.ReadFile(f)
					if err != nil {
						return output.Exit(2, err)
					}
					replies = append(replies, check.RepliesFrom(string(b))...)
				}
				opts := check.ReplayOptions{Threshold: res.Declared.Threshold, Markers: res.Markers}
				if b, err := os.ReadFile(filepath.Join(dir, "kit.config.json")); err == nil {
					opts.RequiredKeys, opts.VolatileKeys = check.KitFields(b)
				}
				res.Replay = check.ReplayHealth(replies, opts)
			}
			if a.Out.JSON {
				if err := a.Out.JSONValue(res); err != nil {
					return err
				}
			} else {
				for _, f := range res.Findings {
					mark := "·"
					switch f.Level {
					case "error":
						mark = "✖"
					case "warning":
						mark = "△"
					}
					a.Out.Line("%s %s: %s", mark, f.Where, f.Msg)
				}
				if h := res.Replay; h != nil {
					a.Out.Line("Replay: %d replies, block in %d, missing closer %d, skipped lines %d, full-width lines %d, choices blocks %d",
						h.Replies, h.WithBlock, h.MissingClose, h.SkippedLines, h.FullWidthLines, h.ChoicesBlocks)
					over := ""
					if h.OverThreshold {
						over = "  OVER"
					}
					a.Out.Line("  marked-up share %.1f%% (threshold %.0f%%, worst reply %.0f%%)%s", h.Overhead*100, h.Threshold*100, h.WorstReply*100, over)
					if len(h.RequiredKeysBelow) > 0 {
						a.Out.Line("  keys written in fewer than 90%% of replies: %s", strings.Join(h.RequiredKeysBelow, ", "))
					}
					if len(h.Markers) > 0 {
						names := make([]string, 0, len(h.Markers))
						for k := range h.Markers {
							names = append(names, k)
						}
						sort.Strings(names)
						parts := make([]string, 0, len(names))
						for _, k := range names {
							v := h.Markers[k]
							parts = append(parts, fmt.Sprintf("%s in %.0f%% of replies (%.1f/reply, %.0f%% of characters)", k, v.Rate*100, v.PerReply, v.Share*100))
						}
						a.Out.Line("  markers the rules consume: %s", strings.Join(parts, ", "))
					}
					var per []string
					for k, v := range h.Keys {
						per = append(per, k+" "+strconvPercent(v.Rate))
					}
					if len(per) > 0 {
						a.Out.Line("  per key: %s", strings.Join(per, ", "))
					}
				}
				if n := res.Errors(); n > 0 {
					a.Out.Line("✖ %d error(s)", n)
				} else {
					a.Out.Line("✔ no errors")
				}
			}
			if res.Errors() > 0 {
				return output.Exitf(1, "check found errors")
			}
			return nil
		},
	}
	c.Flags().StringArrayVar(&replay, "replay", nil, "a transcript of replies to measure the status protocol against (repeatable)")
	return c
}

func strconvPercent(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }
