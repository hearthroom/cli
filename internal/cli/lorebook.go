package cli

import (
	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/lorebook"
	"github.com/hearthroom/cli/internal/output"
)

func init() {
	extraCommands = append(extraCommands, func(a *App) []*cobra.Command { return []*cobra.Command{a.lorebookCommand()} })
}

func (a *App) lorebookCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lorebook",
		Short: "Build lorebook.json from one Markdown file per entry, and check the entries",
		Long: `A Lorebook is easier to write and review as one file per entry. Keep the sources
under <dir>/worldbook/, one Markdown file each: a frontmatter block (name, keywords,
secondaryKeywords, constant, disabled, order, scanDepth, matchWholeWords,
caseSensitive, selective, selectiveLogic; an optional stable id) followed by the
content. Files starting with "_" are notes and are skipped. "build" writes
lorebook.json from them (never the other way round); "check" compares the two and
looks for keywords that fire several entries at once, entries nothing can reach, and
too many constant entries.`,
	}
	cmd.AddCommand(a.lorebookBuild(), a.lorebookCheck())
	return cmd
}

func (a *App) lorebookBuild() *cobra.Command {
	return &cobra.Command{
		Use:   "build [dir]",
		Short: "Write lorebook.json from worldbook/*.md",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := dirArg(args)
			sources, err := lorebook.ReadSources(dir)
			if err != nil {
				return output.Exit(2, err)
			}
			book, err := lorebook.Build(dir, sources)
			if err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(map[string]any{"file": card.LorebookFile, "name": book.Name, "entries": len(book.Entries)})
			}
			a.Out.Line("Wrote %s: %d entries from %d source files", card.LorebookFile, len(book.Entries), len(sources))
			return nil
		},
	}
}

func (a *App) lorebookCheck() *cobra.Command {
	return &cobra.Command{
		Use:   "check [dir]",
		Short: "Compare worldbook/*.md with lorebook.json and look for keyword problems",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := dirArg(args)
			sources, err := lorebook.ReadSources(dir)
			if err != nil {
				return output.Exit(2, err)
			}
			res := lorebook.Check(dir, sources)
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
				a.Out.Line("%d source files, %d built entries", res.Sources, res.Built)
			}
			if res.Status == "error" {
				return output.Exitf(1, "lorebook check found errors")
			}
			return nil
		},
	}
}
