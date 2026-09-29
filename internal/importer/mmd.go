package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// MMD ("Meimo Island") authors get three separate files instead of a card:
// a regex export (rules + statusbar + opening), a World Info JSON, and a
// persona TXT. Each file is classified by content, in any order.

// MMD part names.
const (
	PartRules      = "rules"
	PartBook       = "book"
	PartDefinition = "definition"
)

// MMD errors.
var (
	ErrMMDEmpty       = errors.New("mmd_empty")
	ErrMMDInvalidJSON = errors.New("mmd_invalid_json")
	ErrMMDUnknown     = errors.New("mmd_unknown")
)

// RuleSet is a regex export: rules plus page settings.
type RuleSet struct {
	Rules     []RegexRule
	Statusbar string
	Lowered   bool
	PageMode  string // "" | "sandbox"
	Format    string // "" (mmd) | "tavern"
}

// MMDFile is one classified file.
type MMDFile struct {
	Part     string
	FileName string
	// rules
	Set      *RuleSet
	Welcome  string
	RoleName string
	// book
	Book *Book
	// definition
	Text string
}

// ClassifyMMDFile recognises a file by content.
func ClassifyMMDFile(fileName string, content []byte) (*MMDFile, error) {
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return nil, ErrMMDEmpty
	}
	var raw any
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return nil, ErrMMDInvalidJSON
		}
		return &MMDFile{Part: PartDefinition, FileName: fileName, Text: trimmed}, nil
	}
	if s, ok := raw.(string); ok {
		if strings.TrimSpace(s) == "" {
			return nil, ErrMMDEmpty
		}
		return &MMDFile{Part: PartDefinition, FileName: fileName, Text: strings.TrimSpace(s)}, nil
	}
	obj, isObj := raw.(map[string]any)
	arr, isArr := raw.([]any)
	if !isObj && !isArr {
		return nil, ErrMMDUnknown
	}
	// "Export regex" list, or an API envelope {code, data: [...]}.
	var listItems []any
	if isArr {
		listItems = arr
	} else if d, ok := obj["data"].([]any); ok {
		listItems = d
	}
	if len(listItems) > 0 && allMeimoItems(listItems) {
		rules := RulesFromMeimoList(listItems)
		if len(rules) == 0 {
			return nil, ErrMMDEmpty
		}
		return &MMDFile{Part: PartRules, FileName: fileName, Set: &RuleSet{Rules: rules}}, nil
	}
	if isObj {
		if book := WorldInfoToBook(obj); book != nil {
			if len(book.Entries) == 0 {
				return nil, ErrMMDEmpty
			}
			return &MMDFile{Part: PartBook, FileName: fileName, Book: book}, nil
		}
	}
	set, welcome := RuleSetFromImport(raw)
	if set != nil {
		name := ""
		if isObj {
			name = text(obj["roleName"])
			if name == "" {
				name = text(obj["name"])
			}
		}
		return &MMDFile{Part: PartRules, FileName: fileName, Set: set, Welcome: welcome, RoleName: name}, nil
	}
	return nil, ErrMMDUnknown
}

func isMeimoItem(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	if _, ok := m["regex"].(string); !ok {
		return false
	}
	_, hasContent := m["content"]
	_, hasName := m["name"]
	return hasContent || hasName
}

func allMeimoItems(items []any) bool {
	for _, it := range items {
		if !isMeimoItem(it) {
			return false
		}
	}
	return true
}

// RulesFromMeimoList reads MMD's own {regex, content, name} list.
func RulesFromMeimoList(items []any) []RegexRule {
	var out []RegexRule
	for i, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		find := strText(m["regex"])
		if strings.TrimSpace(find) == "" {
			continue
		}
		name := strText(m["name"])
		if name == "" {
			name = fmt.Sprintf("#%d", i+1)
		}
		out = append(out, RegexRule{Name: name, Find: find, Replace: strText(m["content"]), Enabled: true})
	}
	return out
}

// RuleSetFromImport reads a regex export file, a native {rules:[...]}
// document, a SillyTavern card (its extensions.regex_scripts) or a bare
// scripts array. Returns nil when the value holds no rules.
func RuleSetFromImport(raw any) (*RuleSet, string) {
	obj, isObj := raw.(map[string]any)
	arr, isArr := raw.([]any)
	if !isObj && !isArr {
		return nil, ""
	}
	if isObj {
		if rules, ok := obj["rules"].([]any); ok {
			set := &RuleSet{}
			for _, r := range rules {
				m, ok := r.(map[string]any)
				if !ok || strText(m["find"]) == "" {
					continue
				}
				enabled := true
				if v, ok := m["enabled"].(bool); ok {
					enabled = v
				}
				set.Rules = append(set.Rules, RegexRule{Name: strText(m["name"]), Find: strText(m["find"]), Replace: strText(m["replace"]), Enabled: enabled})
			}
			if len(set.Rules) == 0 {
				return nil, ""
			}
			set.Statusbar = strText(obj["statusbar"])
			set.Lowered = boolOf(obj["lowered"])
			if _, ok := obj["pageDepth"]; ok {
				set.Lowered = LoweredFromPageDepth(obj["pageDepth"])
			}
			if pm := text(obj["pageMode"]); pm == "classic" || pm == "immersive" || pm == "sandbox" {
				set.PageMode = pm
			}
			if IsSandboxChatVersion(obj["chatVersion"]) {
				set.PageMode = "sandbox"
			}
			if f := strings.ToLower(text(obj["cardFormat"])); f == "mmd" || f == "tavern" {
				set.Format = f
			}
			return set, strText(obj["welcome"])
		}
	}
	var scripts any
	if isObj {
		scripts = obj["regex_scripts"]
		if scripts == nil {
			if data, ok := obj["data"].(map[string]any); ok {
				if e, ok := data["extensions"].(map[string]any); ok {
					scripts = e["regex_scripts"]
				}
			}
		}
	}
	if scripts == nil && isArr {
		scripts = arr
	}
	rules := RulesFromTavern(scripts)
	if len(rules) == 0 {
		return nil, ""
	}
	set := &RuleSet{Rules: rules}
	welcome := ""
	if isObj {
		set.Statusbar = strText(obj["statusbar"])
		set.Lowered = LoweredFromPageDepth(obj["pageDepth"])
		if IsSandboxChatVersion(obj["chatVersion"]) {
			set.PageMode = "sandbox"
		}
		spec := text(obj["spec"])
		if spec == "chara_card_v2" || spec == "chara_card_v3" {
			set.Format = "tavern"
		}
		welcome = strText(obj["beginning"])
	} else {
		set.Format = "tavern"
	}
	return set, welcome
}

// LoweredFromPageDepth maps MMD's pageDepth (1 = below the composer) to the
// provider's under/over layer.
func LoweredFromPageDepth(v any) bool {
	s := strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
	if v == nil {
		return false
	}
	return s == "0" || s == "1" || s == "under" || s == "below"
}

// IsSandboxChatVersion is true for chatVersion 1 (number or string).
func IsSandboxChatVersion(v any) bool {
	if v == nil {
		return false
	}
	return strings.TrimSpace(fmt.Sprint(v)) == "1"
}

var extRe = regexp.MustCompile(`\.[^.]+$`)

// StemOf strips the extension from a file name.
func StemOf(fileName string) string {
	return strings.TrimSpace(extRe.ReplaceAllString(filepath.Base(fileName), ""))
}

// MergeMMDFiles combines classified files into one Result. Missing parts
// are reported; the last file of each part wins.
func MergeMMDFiles(files []*MMDFile, language string) *Result {
	var rules, book, def *MMDFile
	for _, f := range files {
		switch f.Part {
		case PartRules:
			rules = f
		case PartBook:
			book = f
		case PartDefinition:
			def = f
		}
	}
	r := &Result{Spec: "mmd", Language: language}
	switch {
	case rules != nil && rules.RoleName != "":
		r.Name = rules.RoleName
	case def != nil:
		r.Name = StemOf(def.FileName)
	case book != nil && book.Book.Name != "":
		r.Name = book.Book.Name
	case len(files) > 0:
		r.Name = StemOf(files[0].FileName)
	}
	if def != nil {
		r.Definition = def.Text
	}
	if rules != nil {
		r.Welcome = rules.Welcome
		r.Rules = rules.Set
	}
	if book != nil {
		name := book.Book.Name
		if name == "" {
			name = r.Name
		}
		r.Lorebook = &LorebookDraft{Name: name, Description: book.Book.Description, Entries: BookEntriesToDrafts(book.Book.Entries)}
		r.Notes = append(r.Notes, BookEntryNotes(book.Book.Entries)...)
	}
	for _, part := range []struct {
		name string
		have bool
	}{{"regex export (rules, statusbar, opening)", rules != nil}, {"World Info JSON (Lorebook)", book != nil}, {"persona TXT (definition)", def != nil}} {
		if !part.have {
			r.Notes = append(r.Notes, "MMD set is missing the "+part.name+"; that part was left empty")
		}
	}
	return r
}
