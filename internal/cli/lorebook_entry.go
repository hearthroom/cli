package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/lorebook"
	"github.com/hearthroom/cli/internal/output"
	"github.com/hearthroom/cli/internal/sync"
)

func (a *App) lorebookEntry() *cobra.Command {
	c := &cobra.Command{
		Use:   "entry",
		Short: "List, read, add, update, or delete individual entries on a linked card's Lorebook",
		Long: `Reads and edits individual entries on the remote Lorebook bound to an owned card,
without touching other entries or book-level metadata (name, description, tags).
Use these commands when an author and an agent—or multiple sessions—edit the same
Lorebook concurrently.

<entry> accepts an entry id or a unique entry name; when multiple entries share
a name, pass the entry id from "lorebook entry list".

Writes send the Lorebook version from .hearthroom/state.json (or --expected-version)
so concurrent edits return a 409 conflict instead of overwriting each other. After
a write succeeds, the entry and version are updated in local lorebook.json and
.hearthroom/state.json so a later "card push" treats that entry as already synced.
Markdown source files under worldbook/*.md are not rewritten automatically.`,
	}
	c.AddCommand(
		a.lorebookEntryList(),
		a.lorebookEntryGet(),
		a.lorebookEntryAdd(),
		a.lorebookEntryUpdate(),
		a.lorebookEntryDelete(),
	)
	return c
}

func splitDirAndEntryArgs(args []string) (dir, entryRef string) {
	if len(args) == 2 {
		return args[0], args[1]
	}
	return ".", args[0]
}

func mapEntryErr(err error) error {
	if err == nil {
		return nil
	}
	var ue *sync.EntryUsageError
	if errors.As(err, &ue) {
		return output.Exit(2, err)
	}
	return err
}

func (a *App) lorebookEntryList() *cobra.Command {
	return &cobra.Command{
		Use:   "list [dir]",
		Short: "List entries in the linked card's remote Lorebook",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := loadFolder(args)
			if err != nil {
				return err
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			res, err := sync.ListLorebookEntries(cmd.Context(), a.Client(), f)
			if err != nil {
				return mapEntryErr(err)
			}
			if a.Out.JSON {
				return a.Out.JSONValue(res)
			}
			if res.LorebookID == "" {
				a.Out.Line("No Lorebook bound to card %s.", res.RoleID)
				return nil
			}
			a.Out.Line("Lorebook %s (%s) · version %d · %d entries", res.Name, res.LorebookID, res.Version, len(res.Entries))
			for _, e := range res.Entries {
				status := "enabled"
				if e.Disabled {
					status = "disabled"
				} else if e.Constant {
					status = "constant"
				}
				cat := e.Category
				if cat == "" {
					cat = "custom"
				}
				kw := strings.Join(e.Keywords, ", ")
				if kw != "" {
					a.Out.Line("  %-36s  %-9s  %-8s  %s  [%s]", e.ID, cat, status, e.Name, kw)
				} else {
					a.Out.Line("  %-36s  %-9s  %-8s  %s", e.ID, cat, status, e.Name)
				}
			}
			return nil
		},
	}
}

func (a *App) lorebookEntryGet() *cobra.Command {
	return &cobra.Command{
		Use:   "get [dir] <entry>",
		Short: "Read one entry from the linked card's remote Lorebook",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, entryRef := splitDirAndEntryArgs(args)
			f, err := loadFolder([]string{dir})
			if err != nil {
				return err
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			res, err := sync.GetLorebookEntry(cmd.Context(), a.Client(), f, entryRef)
			if err != nil {
				return mapEntryErr(err)
			}
			if a.Out.JSON {
				return a.Out.JSONValue(res)
			}
			e := res.Entry
			status := "enabled"
			if e.Disabled {
				status = "disabled"
			}
			a.Out.Line("ID:        %s", e.ID)
			a.Out.Line("Name:      %s", e.Name)
			a.Out.Line("Lorebook:  %s (version %d)", res.LorebookID, res.Version)
			a.Out.Line("Status:    %s (constant=%t)", status, e.Constant)
			if e.Category != "" {
				a.Out.Line("Category:  %s", e.Category)
			}
			if e.TriggerRegion != "" {
				a.Out.Line("Trigger:   %s", e.TriggerRegion)
			}
			a.Out.Line("Keywords:  %s", strings.Join(e.Keywords, ", "))
			if len(e.SecondaryKeywords) > 0 {
				a.Out.Line("Secondary: %s", strings.Join(e.SecondaryKeywords, ", "))
			}
			a.Out.Line("")
			a.Out.Line("%s", e.Content)
			return nil
		},
	}
}

type entryFlagValues struct {
	file              string
	name              string
	content           string
	contentFile       string
	keywords          []string
	secondaryKeywords []string
	category          string
	triggerRegion     string
	constant          bool
	disabled          bool
	semanticRecall    bool
	matchOptions      string
	expectedVersion   int32
	force             bool
}

func bindEntryWriteFlags(c *cobra.Command, v *entryFlagValues) {
	c.Flags().StringVar(&v.file, "file", "", "read entry from a Markdown source file or JSON object ('-' for stdin)")
	c.Flags().StringVar(&v.name, "name", "", "entry title (up to 120 characters)")
	c.Flags().StringVar(&v.content, "content", "", "entry body text")
	c.Flags().StringVar(&v.contentFile, "content-file", "", "read entry body text from a plain file ('-' for stdin)")
	c.Flags().StringSliceVar(&v.keywords, "keywords", nil, "primary trigger keywords (comma-separated or repeatable)")
	c.Flags().StringSliceVar(&v.secondaryKeywords, "secondary-keywords", nil, "secondary filter keywords (comma-separated or repeatable)")
	c.Flags().StringVar(&v.category, "category", "", "entry category: rule, character, location, item, event, or custom")
	c.Flags().StringVar(&v.triggerRegion, "trigger-region", "", "messages scanned for keywords: both, user_only, or ai_only")
	c.Flags().BoolVar(&v.constant, "constant", false, "always include the entry without keyword matching")
	c.Flags().BoolVar(&v.disabled, "disabled", false, "keep the entry saved but inactive")
	c.Flags().BoolVar(&v.semanticRecall, "semantic-recall", true, "allow semantic recall in addition to keyword matching")
	c.Flags().StringVar(&v.matchOptions, "match-options", "", "JSON object for advanced matchOptions")
	c.Flags().Int32Var(&v.expectedVersion, "expected-version", 0, "require this remote Lorebook version before writing")
	c.Flags().BoolVar(&v.force, "force", false, "write without checking the remote Lorebook version")
}

func cleanKeywordSlice(items []string) []string {
	out := make([]string, 0, len(items))
	for _, raw := range items {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' || r == '、' }) {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}

func readInputBytes(cmd *cobra.Command, path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(cmd.InOrStdin())
	}
	return os.ReadFile(path)
}

func parseEntryFile(cmd *cobra.Command, filePath string, isUpdate bool) (sync.EntryPatch, error) {
	data, err := readInputBytes(cmd, filePath)
	if err != nil {
		return sync.EntryPatch{}, output.Exit(2, err)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return sync.EntryPatch{}, output.Exitf(2, "%s is empty", filePath)
	}

	if strings.HasSuffix(strings.ToLower(filePath), ".json") || strings.HasPrefix(trimmed, "{") {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return sync.EntryPatch{}, output.Exitf(2, "%s: invalid JSON: %v", filePath, err)
		}
		// Accept wrapper {"entry": {...}} produced by `lorebook entry get --json` as well as direct entry objects.
		wrappedEntry := false
		if nested, ok := raw["entry"]; ok && len(nested) > 0 && strings.HasPrefix(strings.TrimSpace(string(nested)), "{") {
			var inner map[string]json.RawMessage
			if err := json.Unmarshal(nested, &inner); err != nil {
				return sync.EntryPatch{}, output.Exitf(2, "%s: invalid entry JSON: %v", filePath, err)
			}
			raw = inner
			wrappedEntry = true
		}
		var e card.LorebookEntry
		b, _ := json.Marshal(raw)
		if err := json.Unmarshal(b, &e); err != nil {
			return sync.EntryPatch{}, output.Exitf(2, "%s: %v", filePath, err)
		}
		patch := sync.EntryPatch{}
		if _, ok := raw["name"]; ok || !isUpdate {
			n := e.Name
			patch.Name = &n
		}
		if _, ok := raw["content"]; ok || !isUpdate {
			c := e.Content
			patch.Content = &c
		}
		if _, ok := raw["keywords"]; ok || !isUpdate {
			kw := e.Keywords
			if kw == nil {
				kw = []string{}
			}
			patch.Keywords = &kw
		}
		if _, ok := raw["secondaryKeywords"]; ok || wrappedEntry {
			skw := e.SecondaryKeywords
			if skw == nil {
				skw = []string{}
			}
			patch.SecondaryKeywords = &skw
		}
		if _, ok := raw["category"]; ok && e.Category != "" {
			cat := e.Category
			patch.Category = &cat
		}
		if _, ok := raw["triggerRegion"]; ok && e.TriggerRegion != "" {
			tr := e.TriggerRegion
			patch.TriggerRegion = &tr
		}
		if _, ok := raw["constant"]; ok || wrappedEntry {
			constant := e.Constant
			patch.Constant = &constant
		} else if rawConst, ok := raw["isConstant"]; ok {
			var ic bool
			if json.Unmarshal(rawConst, &ic) == nil {
				patch.Constant = &ic
			}
		}
		if _, ok := raw["disabled"]; ok || wrappedEntry {
			disabled := e.Disabled
			patch.Disabled = &disabled
		} else if rawEnabled, ok := raw["isEnabled"]; ok {
			var ie bool
			if json.Unmarshal(rawEnabled, &ie) == nil {
				dis := !ie
				patch.Disabled = &dis
			}
		}
		if e.SemanticRecall != nil {
			sr := *e.SemanticRecall
			patch.SemanticRecall = &sr
		}
		if len(e.MatchOptions) > 0 && string(e.MatchOptions) != "null" {
			mo := e.MatchOptions
			patch.MatchOptions = &mo
		} else if wrappedEntry && isUpdate {
			patch.ClearMatchOptions = true
		}
		return patch, nil
	}

	sourceName := filepath.Base(filePath)
	if filePath == "-" {
		sourceName = "stdin.md"
	}
	parsed, err := lorebook.ParseSource(sourceName, text)
	if err != nil {
		return sync.EntryPatch{}, output.Exitf(2, "%s: %v", filePath, err)
	}
	e := parsed.Entry
	if filePath == "-" && e.Name == "stdin" {
		e.Name = ""
	}
	kw := e.Keywords
	if kw == nil {
		kw = []string{}
	}
	skw := e.SecondaryKeywords
	if skw == nil {
		skw = []string{}
	}
	constant := e.Constant
	disabled := e.Disabled
	patch := sync.EntryPatch{
		Content:           &e.Content,
		Keywords:          &kw,
		SecondaryKeywords: &skw,
		Constant:          &constant,
		Disabled:          &disabled,
	}
	if e.Name != "" || !isUpdate {
		n := e.Name
		patch.Name = &n
	}
	if e.Category != "" {
		cat := e.Category
		patch.Category = &cat
	}
	if e.TriggerRegion != "" {
		tr := e.TriggerRegion
		patch.TriggerRegion = &tr
	}
	if e.SemanticRecall != nil {
		sr := *e.SemanticRecall
		patch.SemanticRecall = &sr
	}
	if len(e.MatchOptions) > 0 {
		mo := e.MatchOptions
		patch.MatchOptions = &mo
	} else if isUpdate {
		patch.ClearMatchOptions = true
	}
	return patch, nil
}

func buildEntryPatchFromFlags(cmd *cobra.Command, v entryFlagValues, isUpdate bool) (sync.EntryPatch, sync.EntryWriteOptions, error) {
	if cmd.Flags().Changed("content") && cmd.Flags().Changed("content-file") {
		return sync.EntryPatch{}, sync.EntryWriteOptions{}, output.Exitf(2, "--content and --content-file cannot be combined")
	}
	if v.file == "-" && v.contentFile == "-" {
		return sync.EntryPatch{}, sync.EntryWriteOptions{}, output.Exitf(2, "--file and --content-file cannot both read from stdin ('-')")
	}

	var patch sync.EntryPatch
	var err error
	if cmd.Flags().Changed("file") {
		patch, err = parseEntryFile(cmd, v.file, isUpdate)
		if err != nil {
			return sync.EntryPatch{}, sync.EntryWriteOptions{}, err
		}
	}

	if cmd.Flags().Changed("name") {
		n := v.name
		patch.Name = &n
	}
	if cmd.Flags().Changed("content") {
		c := v.content
		patch.Content = &c
	}
	if cmd.Flags().Changed("content-file") {
		b, err := readInputBytes(cmd, v.contentFile)
		if err != nil {
			return sync.EntryPatch{}, sync.EntryWriteOptions{}, output.Exit(2, err)
		}
		c := strings.TrimSpace(strings.ReplaceAll(string(b), "\r\n", "\n"))
		patch.Content = &c
	}
	if cmd.Flags().Changed("keywords") {
		kw := cleanKeywordSlice(v.keywords)
		patch.Keywords = &kw
	}
	if cmd.Flags().Changed("secondary-keywords") {
		skw := cleanKeywordSlice(v.secondaryKeywords)
		patch.SecondaryKeywords = &skw
	}
	if cmd.Flags().Changed("category") {
		cat := v.category
		patch.Category = &cat
	}
	if cmd.Flags().Changed("trigger-region") {
		tr := v.triggerRegion
		patch.TriggerRegion = &tr
	}
	if cmd.Flags().Changed("constant") {
		constant := v.constant
		patch.Constant = &constant
	}
	if cmd.Flags().Changed("disabled") {
		disabled := v.disabled
		patch.Disabled = &disabled
	}
	if cmd.Flags().Changed("semantic-recall") {
		sr := v.semanticRecall
		patch.SemanticRecall = &sr
	}
	if cmd.Flags().Changed("match-options") {
		trimmed := strings.TrimSpace(v.matchOptions)
		if trimmed == "" {
			return sync.EntryPatch{}, sync.EntryWriteOptions{}, output.Exitf(2, "--match-options must be a JSON object")
		}
		if trimmed == "null" || trimmed == "{}" {
			patch.MatchOptions = nil
			patch.ClearMatchOptions = isUpdate
		} else {
			raw := json.RawMessage(trimmed)
			patch.MatchOptions = &raw
			patch.ClearMatchOptions = false
		}
	}

	opts := sync.EntryWriteOptions{
		ExpectedVersion: v.expectedVersion,
		Force:           v.force,
	}
	return patch, opts, nil
}

func (a *App) lorebookEntryAdd() *cobra.Command {
	var flags entryFlagValues
	c := &cobra.Command{
		Use:   "add [dir]",
		Short: "Add one entry to the linked card's remote Lorebook",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := loadFolder(args)
			if err != nil {
				return err
			}
			patch, opts, err := buildEntryPatchFromFlags(cmd, flags, false)
			if err != nil {
				return err
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			res, err := sync.AddLorebookEntry(cmd.Context(), a.Client(), f, patch, opts)
			if err != nil {
				return mapEntryErr(err)
			}
			if a.Out.JSON {
				return a.Out.JSONValue(res)
			}
			a.Out.Line("Added Lorebook entry %s (%s) · version %d", res.Entry.Name, res.Entry.ID, res.Version)
			for _, w := range res.Warnings {
				a.Out.Note("warning: %s", w)
			}
			return nil
		},
	}
	bindEntryWriteFlags(c, &flags)
	return c
}

func (a *App) lorebookEntryUpdate() *cobra.Command {
	var flags entryFlagValues
	c := &cobra.Command{
		Use:   "update [dir] <entry>",
		Short: "Update one entry in the linked card's remote Lorebook",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, entryRef := splitDirAndEntryArgs(args)
			f, err := loadFolder([]string{dir})
			if err != nil {
				return err
			}
			patch, opts, err := buildEntryPatchFromFlags(cmd, flags, true)
			if err != nil {
				return err
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			res, err := sync.UpdateLorebookEntry(cmd.Context(), a.Client(), f, entryRef, patch, opts)
			if err != nil {
				return mapEntryErr(err)
			}
			if a.Out.JSON {
				return a.Out.JSONValue(res)
			}
			a.Out.Line("Updated Lorebook entry %s (%s) · version %d", res.Entry.Name, res.Entry.ID, res.Version)
			for _, w := range res.Warnings {
				a.Out.Note("warning: %s", w)
			}
			return nil
		},
	}
	bindEntryWriteFlags(c, &flags)
	return c
}

func (a *App) lorebookEntryDelete() *cobra.Command {
	var expectedVersion int32
	var force bool
	c := &cobra.Command{
		Use:   "delete [dir] <entry>",
		Short: "Delete one entry from the linked card's remote Lorebook",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, entryRef := splitDirAndEntryArgs(args)
			f, err := loadFolder([]string{dir})
			if err != nil {
				return err
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			res, err := sync.DeleteLorebookEntry(cmd.Context(), a.Client(), f, entryRef, sync.EntryWriteOptions{
				ExpectedVersion: expectedVersion,
				Force:           force,
			})
			if err != nil {
				return mapEntryErr(err)
			}
			if a.Out.JSON {
				return a.Out.JSONValue(res)
			}
			a.Out.Line("Deleted Lorebook entry %s (%s) · version %d", res.DeletedName, res.DeletedEntryID, res.Version)
			for _, w := range res.Warnings {
				a.Out.Note("warning: %s", w)
			}
			return nil
		},
	}
	c.Flags().Int32Var(&expectedVersion, "expected-version", 0, "require this remote Lorebook version before deleting")
	c.Flags().BoolVar(&force, "force", false, "delete without checking the remote Lorebook version")
	return c
}
