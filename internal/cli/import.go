package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/importer"
	"github.com/hearthroom/cli/internal/output"
)

func (a *App) cardImport() *cobra.Command {
	var out, language string
	var force, mmd bool
	c := &cobra.Command{
		Use:   "import <file>...",
		Short: "Convert a SillyTavern PNG/JSON/CHARX card or an MMD file set into a card folder",
		Long: `Reads cards from other tools and writes a card folder. Nothing is sent to the
provider; review the folder, then run "hearthroom card push".

Formats are detected by content:
  - SillyTavern PNG (V2 "chara" or V3 "ccv3" text chunk), JSON (V1/V2/V3), CHARX (zip)
  - MMD three-file set: regex export JSON, World Info JSON and persona TXT, in any order;
    pass all files of the set in one call (a PNG among them is treated as a SillyTavern card)

Every source field that has no place in the folder is listed in the report.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var result *importer.Result
			var mmdFiles []*importer.MMDFile
			for _, path := range args {
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if importer.IsPNG(b) || looksLikeCard(b) {
					if mmd && !importer.IsPNG(b) {
						// explicit MMD mode: treat JSON as a set part
					} else {
						if result != nil {
							return output.Exitf(2, "more than one card file given (%s); import one card per call", path)
						}
						parsed, err := importer.ParseCardFile(b)
						if err != nil {
							return output.Exit(2, importError(path, err))
						}
						result = importer.FromTavern(parsed, language)
						continue
					}
				}
				f, err := importer.ClassifyMMDFile(filepath.Base(path), b)
				if err != nil {
					return output.Exit(2, importError(path, err))
				}
				mmdFiles = append(mmdFiles, f)
			}
			if result != nil && len(mmdFiles) > 0 {
				return output.Exitf(2, "mixed inputs: a card file and %d MMD part(s); import them separately", len(mmdFiles))
			}
			if result == nil {
				result = importer.MergeMMDFiles(mmdFiles, language)
			}
			dir := out
			if dir == "" {
				dir = folderNameFor(result.Name, args[0])
			}
			rep, err := importer.WriteFolder(result, dir, force)
			if err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(rep)
			}
			a.Out.Line("Imported %s card %q into %s", rep.Spec, rep.Name, rep.Dir)
			for _, f := range rep.Files {
				a.Out.Line("  %s", f)
			}
			for _, as := range rep.Assets {
				a.Out.Line("  %s", as)
			}
			if rep.Lorebook > 0 {
				a.Out.Line("  Lorebook entries: %d", rep.Lorebook)
			}
			if rep.Rules > 0 {
				a.Out.Line("  display rules: %d", rep.Rules)
			}
			if len(rep.Notes) > 0 {
				a.Out.Line("Not imported or changed:")
				for _, n := range rep.Notes {
					a.Out.Line("  - %s", n)
				}
			}
			a.Out.Line("Next: review the folder, then `hearthroom card push %s`", rep.Dir)
			return nil
		},
	}
	c.Flags().StringVar(&out, "out", "", "folder to write (default: derived from the card name)")
	c.Flags().StringVar(&language, "language", "en", "card language: picks the creator note and sets the Lorebook entry length limit (en, zh-Hant, zh-Hans, ja, ko)")
	c.Flags().BoolVar(&force, "force", false, "overwrite an existing card folder")
	c.Flags().BoolVar(&mmd, "mmd", false, "treat JSON inputs as MMD set parts even if they look like a card")
	return c
}

func looksLikeCard(b []byte) bool {
	t := strings.TrimSpace(string(b[:min(len(b), 4)]))
	if strings.HasPrefix(t, "PK") {
		return true
	}
	if !strings.HasPrefix(t, "{") {
		return false
	}
	s := string(b)
	return strings.Contains(s, `"spec"`) && strings.Contains(s, `"data"`) || strings.Contains(s, `"first_mes"`)
}

func importError(path string, err error) error {
	switch {
	case errors.Is(err, importer.ErrNoMetadata):
		return fmt.Errorf("%s: no character card data found in this file", path)
	case errors.Is(err, importer.ErrPNGTruncated):
		return fmt.Errorf("%s: the PNG is truncated or corrupt", path)
	case errors.Is(err, importer.ErrInvalid):
		return fmt.Errorf("%s: not a SillyTavern card (PNG, JSON or CHARX)", path)
	case errors.Is(err, importer.ErrMMDEmpty):
		return fmt.Errorf("%s: the file is empty", path)
	case errors.Is(err, importer.ErrMMDInvalidJSON):
		return fmt.Errorf("%s: looks like JSON but does not parse", path)
	case errors.Is(err, importer.ErrMMDUnknown):
		return fmt.Errorf("%s: not recognised as a regex export, a World Info file or a persona text", path)
	}
	return fmt.Errorf("%s: %w", path, err)
}

func folderNameFor(name, firstInput string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = importer.StemOf(firstInput)
	}
	var b strings.Builder
	for _, r := range base {
		switch {
		case r == ' ' || r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			b.WriteRune('-')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		out = "card"
	}
	return out
}
