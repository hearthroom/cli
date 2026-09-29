package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/hearthroom/cli/internal/card"
)

// LorebookDraft is the imported Lorebook.
type LorebookDraft struct {
	Name        string
	Description string
	Entries     []EntryDraft
}

// Result is what an import produced, independent of the source format.
type Result struct {
	Spec           string
	Language       string
	Name           string
	Summary        string
	Definition     string
	Welcome        string
	Alternates     []string
	Prologue       []string
	Tags           []string
	Nickname       string
	OutputContract string
	Custom         string
	TalkExample    []TalkLine
	CardMeta       map[string]any
	Lorebook       *LorebookDraft
	Rules          *RuleSet
	CreatorNotes   string
	// Portrait/Background bytes and extensions, when the source bundled them.
	Portrait      []byte
	PortraitExt   string
	Background    []byte
	BackgroundExt string
	// Extra CHARX files to copy under assets/, by archive path.
	Files map[string][]byte
	// Notes lists everything that could not be placed, in plain English.
	Notes []string
}

// FromTavern maps a parsed SillyTavern card to a Result.
func FromTavern(p *Parsed, language string) *Result {
	d := p.Card.Data
	r := &Result{Spec: p.Card.Spec, Language: language}
	r.Name = d.Name
	r.Nickname = d.Nickname
	r.CardMeta = CardMetaFromTavern(d)
	r.Summary = MultilingualNote(d.CreatorNotesMultilingual, language)
	if r.Summary == "" {
		r.Summary = d.CreatorNotes
	}
	r.CreatorNotes = d.CreatorNotes
	r.Definition = JoinPersona(d)
	r.Welcome = d.FirstMes
	r.Alternates = append(r.Alternates, d.AlternateGreetings...)
	if n := len(d.GroupOnlyGreetings); n > 0 {
		r.Alternates = append(r.Alternates, d.GroupOnlyGreetings...)
		r.Notes = append(r.Notes, fmt.Sprintf("%d group-only greetings were added as alternate openings (the provider has no group chat)", n))
	}
	r.OutputContract = d.SystemPrompt
	if d.PostHistoryInstructions != "" {
		r.Custom = d.PostHistoryInstructions
		r.Notes = append(r.Notes, "post_history_instructions was placed in customInstructions; review whether it belongs there")
	}
	if len(d.Tags) > TagsMax {
		r.Tags = d.Tags[:TagsMax]
		r.Notes = append(r.Notes, fmt.Sprintf("%d tags beyond the limit of %d were dropped", len(d.Tags)-TagsMax, TagsMax))
	} else {
		r.Tags = d.Tags
	}
	if ex := strings.TrimSpace(d.MesExample); ex != "" {
		lines := ParseMesExample(ex)
		switch {
		case len(lines) > 0:
			r.TalkExample = lines
		case r.OutputContract == "" && utf8.RuneCountInString(ex) <= 2000:
			// MMD-style cards keep their output rules in mes_example.
			r.OutputContract = ex
			r.Notes = append(r.Notes, "mes_example was not dialogue and was placed in outputContract")
		default:
			r.Notes = append(r.Notes, "mes_example was not in the {{user}}:/{{char}}: format and was not imported")
		}
	}
	icon, bg := mainAsset(d.Assets, "icon"), mainAsset(d.Assets, "background")
	other := 0
	for i := range d.Assets {
		a := &d.Assets[i]
		if strings.TrimSpace(a.URI) == "" || strings.TrimSpace(a.URI) == "ccdefault:" {
			continue
		}
		if a != icon && a != bg {
			other++
		}
	}
	if other > 0 {
		if len(p.Files) > 0 {
			r.Notes = append(r.Notes, fmt.Sprintf("%d additional assets (expressions, sounds…) were copied under assets/ but are not referenced by the card", other))
		} else {
			r.Notes = append(r.Notes, fmt.Sprintf("%d additional assets (expressions, sounds…) have no destination and were not imported", other))
		}
	}
	if len(d.Extensions) > 0 {
		rules := RulesFromTavern(d.Extensions["regex_scripts"])
		if len(rules) > 0 {
			r.Rules = &RuleSet{Rules: rules, Format: "tavern"}
		}
		rest := 0
		for k := range d.Extensions {
			if k != "regex_scripts" {
				rest++
			}
		}
		if rest > 0 {
			r.Notes = append(r.Notes, fmt.Sprintf("%d extension keys other than regex_scripts were not imported", rest))
		}
	}
	if book := d.CharacterBook; book != nil && len(book.Entries) > 0 {
		name := book.Name
		if name == "" {
			name = r.Name
		}
		r.Lorebook = &LorebookDraft{Name: name, Description: book.Description, Entries: BookEntriesToDrafts(book.Entries)}
		r.Notes = append(r.Notes, BookEntryNotes(book.Entries)...)
		if book.ScanDepth != 0 && book.ScanDepth != 2 || book.TokenBudget > 0 || book.RecursiveScanning {
			r.Notes = append(r.Notes, "Lorebook scan depth, token budget and recursive scanning are provider settings and were not imported")
		}
	}
	r.Portrait, r.PortraitExt = p.Image, p.ImageExt
	r.Background, r.BackgroundExt = p.Background, p.BackgroundExt
	r.Files = p.Files
	return r
}

// Report is what WriteFolder tells the caller.
type Report struct {
	Dir      string   `json:"dir"`
	Spec     string   `json:"spec"`
	Name     string   `json:"name"`
	Files    []string `json:"files"`
	Assets   []string `json:"assets"`
	Notes    []string `json:"notes"`
	Lorebook int      `json:"lorebookEntries"`
	Rules    int      `json:"rules"`
}

// WriteFolder materialises a Result as a card folder. It refuses to write
// into an existing card folder unless force is set.
func WriteFolder(r *Result, dir string, force bool) (*Report, error) {
	if _, err := os.Stat(filepath.Join(dir, card.ManifestFile)); err == nil && !force {
		return nil, fmt.Errorf("%s already contains a card; pass --force to overwrite", dir)
	}
	if err := os.MkdirAll(filepath.Join(dir, card.AssetsDir), 0o755); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(r.Name)
	if name == "" {
		name = filepath.Base(dir)
	}
	f := &card.Folder{Dir: dir}
	f.Manifest = card.Manifest{
		FormatVersion:      card.FormatVersion,
		Name:               name,
		Summary:            r.Summary,
		Tags:               r.Tags,
		Type:               "story",
		Nickname:           r.Nickname,
		OutputContract:     r.OutputContract,
		CustomInstructions: r.Custom,
		Prologue:           r.Prologue,
		Language:           r.Language,
	}
	for _, t := range r.TalkExample {
		f.Manifest.TalkExample = append(f.Manifest.TalkExample, card.TalkExample{RoleType: t.RoleType, Content: t.Content})
	}
	if len(r.CardMeta) > 0 {
		raw, _ := json.Marshal(r.CardMeta)
		f.Manifest.CardMeta = raw
	}
	f.Definition = r.Definition
	f.Welcome = r.Welcome
	for _, a := range r.Alternates {
		if strings.TrimSpace(a) != "" {
			f.Alternates = append(f.Alternates, card.Opening{Text: a})
		}
	}
	rep := &Report{Dir: dir, Spec: r.Spec, Name: name, Notes: append([]string{}, r.Notes...)}

	if len(r.Portrait) > 0 {
		ext := r.PortraitExt
		if ext == "" {
			ext = "png"
		}
		rel := card.AssetsDir + "/portrait." + ext
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), r.Portrait, 0o644); err != nil {
			return nil, err
		}
		f.Manifest.Media.Portrait = rel
		rep.Assets = append(rep.Assets, rel)
	}
	if len(r.Background) > 0 {
		ext := r.BackgroundExt
		if ext == "" {
			ext = "png"
		}
		rel := card.AssetsDir + "/background." + ext
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), r.Background, 0o644); err != nil {
			return nil, err
		}
		f.Manifest.Media.Background = rel
		rep.Assets = append(rep.Assets, rel)
	}
	for archivePath, data := range r.Files {
		if archivePath == "card.json" || strings.HasSuffix(archivePath, "/") {
			continue
		}
		clean := filepath.ToSlash(filepath.Clean("/" + archivePath))[1:]
		if clean == "" || strings.HasPrefix(clean, "..") {
			continue
		}
		rel := card.AssetsDir + "/" + strings.TrimPrefix(clean, "assets/")
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			return nil, err
		}
		rep.Assets = append(rep.Assets, rel)
	}

	if r.Lorebook != nil && len(r.Lorebook.Entries) > 0 {
		lb := &card.Lorebook{Name: r.Lorebook.Name, Description: r.Lorebook.Description, Format: "tavern"}
		for _, e := range r.Lorebook.Entries {
			var mo json.RawMessage
			if len(e.MatchOptions) > 0 {
				mo, _ = json.Marshal(e.MatchOptions)
			}
			lb.Entries = append(lb.Entries, card.LorebookEntry{
				Name: e.Name, Content: e.Content, Keywords: nonNil(e.Keywords), SecondaryKeywords: e.SecondaryKeywords,
				Constant: e.Constant, Disabled: !e.Enabled, MatchOptions: mo,
			})
		}
		f.Lorebook = lb
		rep.Lorebook = len(lb.Entries)
	}
	if r.Rules != nil && len(r.Rules.Rules) > 0 {
		rules := &card.Rules{MountTrigger: r.Rules.Statusbar, MountLayer: "over", PageMode: r.Rules.PageMode, CardFormat: r.Rules.Format}
		if r.Rules.Lowered {
			rules.MountLayer = "under"
		}
		for i, rr := range r.Rules.Rules {
			rules.Rules = append(rules.Rules, card.DisplayRule{ID: fmt.Sprintf("rule-%02d", i+1), Name: rr.Name, Find: rr.Find, Replace: rr.Replace, Enabled: rr.Enabled})
		}
		f.Rules = rules
		rep.Rules = len(rules.Rules)
	}
	if err := f.Save(); err != nil {
		return nil, err
	}
	rep.Files = []string{card.ManifestFile, card.DefinitionFile, card.WelcomeFile}
	for _, o := range f.Alternates {
		rep.Files = append(rep.Files, o.File)
	}
	if f.Lorebook != nil {
		rep.Files = append(rep.Files, card.LorebookFile)
	}
	if f.Rules != nil {
		rep.Files = append(rep.Files, card.RulesFile)
	}
	if wrote, err := card.WriteAgentsGuide(dir, name); err != nil {
		return nil, err
	} else if wrote {
		rep.Files = append(rep.Files, card.AgentsFile)
	}
	if strings.TrimSpace(r.CreatorNotes) != "" {
		readme := "# " + name + "\n\nCreator notes from the imported card (not sent to the provider):\n\n" + strings.TrimSpace(r.CreatorNotes) + "\n"
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
			return nil, err
		}
		rep.Files = append(rep.Files, "README.md")
	}
	if rep.Notes == nil {
		rep.Notes = []string{}
	}
	if rep.Assets == nil {
		rep.Assets = []string{}
	}
	return rep, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
