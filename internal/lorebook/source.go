// Package lorebook builds lorebook.json from one Markdown file per entry under
// worldbook/, and checks the sources against the built file and against each other.
//
// A source file is a YAML-style frontmatter block followed by the entry's content:
//
//	---
//	name: Harbor curfew
//	keywords: curfew, harbour, 宵禁
//	secondaryKeywords: []
//	constant: false
//	order: 10
//	scanDepth: 0
//	matchWholeWords: true
//	---
//	The harbour closes at dusk …
//
// `id` is the stable identity (the file stem when absent); `uid`/`order` are build
// outputs, never hand-edited. A file whose first line is a Markdown heading and whose
// second line is a `> key: value · key: value` blockquote is read the same way (the
// convention of some authors' worldbook notes), so existing folders need no rewrite.
// Files whose name starts with `_` (a manifest, notes) are skipped.
package lorebook

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/hearthroom/cli/internal/card"
)

// SourceDir is the folder of entry files inside a card folder.
const SourceDir = "worldbook"

// UnorderedLast is the order of an entry whose source gives none: after every explicit one.
const UnorderedLast = 1 << 30

// Source is one parsed entry file.
type Source struct {
	File  string
	ID    string
	Order int
	Entry card.LorebookEntry
}

var (
	frontmatterRe = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n?`)
	headingRe     = regexp.MustCompile(`^#\s+(.+?)\s*$`)
	metaLineRe    = regexp.MustCompile(`^>\s*(.+)$`)
)

// ReadSources parses every entry file under <dir>/worldbook.
func ReadSources(dir string) ([]Source, error) {
	root := filepath.Join(dir, SourceDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s has no %s/ folder", dir, SourceDir)
		}
		return nil, err
	}
	var out []Source
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, "_") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return nil, err
		}
		s, err := ParseSource(name, strings.ReplaceAll(string(b), "\r\n", "\n"))
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", SourceDir, name, err)
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].File < out[j].File
	})
	return out, nil
}

// ParseSource reads one file's text.
func ParseSource(fileName, text string) (Source, error) {
	stem := strings.TrimSuffix(fileName, ".md")
	s := Source{File: fileName, ID: stem, Order: UnorderedLast}
	fields := map[string]string{}
	body := text
	if m := frontmatterRe.FindStringSubmatch(text); m != nil {
		for _, line := range strings.Split(m[1], "\n") {
			k, v, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
		body = text[len(m[0]):]
	} else {
		// Heading + `> key: value · key: value` meta line.
		lines := strings.SplitN(text, "\n", 3)
		if len(lines) >= 2 {
			if h := headingRe.FindStringSubmatch(lines[0]); h != nil {
				title := h[1]
				if i := strings.Index(title, " — "); i > 0 {
					title = title[:i]
				}
				fields["name"] = strings.TrimSpace(title)
				rest := strings.Join(lines[1:], "\n")
				if ml := metaLineRe.FindStringSubmatch(strings.TrimSpace(strings.SplitN(strings.TrimSpace(rest), "\n", 2)[0])); ml != nil {
					for _, part := range strings.Split(ml[1], "·") {
						k, v, ok := strings.Cut(part, ":")
						if !ok {
							continue
						}
						k = strings.TrimSpace(k)
						v = strings.TrimSpace(v)
						switch {
						case strings.HasPrefix(k, "isConstant"):
							fields["constant"] = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(v, "建議:"), "建議："))
						default:
							fields[k] = v
						}
					}
					body = strings.TrimSpace(rest)
					if i := strings.Index(body, "\n"); i >= 0 {
						body = body[i+1:]
					} else {
						body = ""
					}
				} else {
					body = rest
				}
			}
		}
	}
	if v := fields["id"]; v != "" {
		s.ID = v
	}
	s.Entry.Name = fields["name"]
	if s.Entry.Name == "" {
		s.Entry.Name = stem
	}
	s.Entry.Content = strings.TrimSpace(body)
	s.Entry.Keywords = list(fields["keywords"])
	if s.Entry.Keywords == nil {
		s.Entry.Keywords = []string{}
	}
	s.Entry.SecondaryKeywords = list(fields["secondaryKeywords"])
	s.Entry.Constant = truthy(fields["constant"])
	s.Entry.Disabled = truthy(fields["disabled"])
	if v := fields["category"]; v != "" {
		s.Entry.Category = v
	}
	if v := fields["order"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return s, fmt.Errorf("order %q is not a number", v)
		}
		s.Order = n
	}
	opts := map[string]any{}
	for _, k := range []string{"scanDepth", "selectiveLogic"} {
		if v := fields[k]; v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return s, fmt.Errorf("%s %q is not a number", k, v)
			}
			opts[k] = n
		}
	}
	for _, k := range []string{"matchWholeWords", "caseSensitive", "selective"} {
		if v := fields[k]; v != "" {
			opts[k] = truthy(v)
		}
	}
	if len(opts) > 0 {
		raw, _ := json.Marshal(opts)
		s.Entry.MatchOptions = raw
	}
	return s, nil
}

func list(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" || v == "[]" {
		return nil
	}
	v = strings.TrimPrefix(strings.TrimSuffix(v, "]"), "[")
	var out []string
	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '，' || r == '、' }) {
		p := strings.Trim(strings.TrimSpace(part), `"'`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "yes", "1", "on", "是":
		return true
	}
	return false
}

// Build writes lorebook.json from the sources. The book keeps its existing name,
// description, format and remote ids where an entry's name matches.
func Build(dir string, sources []Source) (*card.Lorebook, error) {
	book := &card.Lorebook{Entries: []card.LorebookEntry{}}
	existing := map[string]card.LorebookEntry{}
	if b, err := os.ReadFile(filepath.Join(dir, card.LorebookFile)); err == nil {
		var old card.Lorebook
		if json.Unmarshal(b, &old) == nil {
			book.ID, book.Name, book.Description, book.Format = old.ID, old.Name, old.Description, old.Format
			for _, e := range old.Entries {
				existing[e.Name] = e
			}
		}
	}
	if book.Name == "" {
		if m, err := loadManifestName(dir); err == nil && m != "" {
			book.Name = m
		} else {
			book.Name = filepath.Base(dir)
		}
	}
	for _, s := range sources {
		e := s.Entry
		if old, ok := existing[e.Name]; ok {
			e.ID = old.ID
			if e.TriggerRegion == "" {
				e.TriggerRegion = old.TriggerRegion
			}
		}
		book.Entries = append(book.Entries, e)
	}
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(book); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, card.LorebookFile), []byte(buf.String()), 0o644); err != nil {
		return nil, err
	}
	return book, nil
}

func loadManifestName(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, card.ManifestFile))
	if err != nil {
		return "", err
	}
	var m struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return "", err
	}
	return m.Name, nil
}

// Finding is one check observation.
type Finding struct {
	Level string `json:"level"` // error | warning | info
	Where string `json:"where"`
	Msg   string `json:"msg"`
}

// CheckResult is the report of `lorebook check`.
type CheckResult struct {
	Status   string    `json:"status"`
	Sources  int       `json:"sources"`
	Built    int       `json:"builtEntries"`
	Findings []Finding `json:"findings"`
}

// Check compares the sources with lorebook.json and looks for keyword problems.
func Check(dir string, sources []Source) *CheckResult {
	res := &CheckResult{Sources: len(sources), Findings: []Finding{}}
	add := func(level, where, format string, args ...any) {
		res.Findings = append(res.Findings, Finding{Level: level, Where: where, Msg: fmt.Sprintf(format, args...)})
	}
	built := map[string]card.LorebookEntry{}
	if b, err := os.ReadFile(filepath.Join(dir, card.LorebookFile)); err == nil {
		var book card.Lorebook
		if err := json.Unmarshal(b, &book); err != nil {
			add("error", card.LorebookFile, "not valid JSON: %v", err)
		}
		res.Built = len(book.Entries)
		for _, e := range book.Entries {
			built[e.Name] = e
		}
	} else {
		add("warning", card.LorebookFile, "missing; run `hearthroom lorebook build`")
	}
	seen := map[string]bool{}
	constant := 0
	keywordOwners := map[string][]string{}
	for _, s := range sources {
		where := SourceDir + "/" + s.File
		e := s.Entry
		seen[e.Name] = true
		if e.Constant {
			constant++
		}
		if e.Content == "" {
			add("error", where, "no content")
		}
		if len(e.Keywords) == 0 && !e.Constant && !e.Disabled {
			add("warning", where, "no keywords and not constant: only semantic admission can bring it in, which is not a promise")
		}
		for _, k := range e.Keywords {
			kl := strings.ToLower(strings.TrimSpace(k))
			if kl == "" {
				continue
			}
			keywordOwners[kl] = append(keywordOwners[kl], e.Name)
			if isCommonWord(kl) {
				add("warning", where, "keyword %q is very short; a word the character says every turn makes the entry permanent (use matchWholeWords or a longer phrase)", k)
			}
		}
		if len(built) > 0 {
			old, ok := built[e.Name]
			switch {
			case !ok:
				add("warning", where, "not in %s (run build)", card.LorebookFile)
			case old.Content != e.Content || strings.Join(old.Keywords, "\x00") != strings.Join(e.Keywords, "\x00") || old.Constant != e.Constant || old.Disabled != e.Disabled:
				add("warning", where, "differs from %s (run build)", card.LorebookFile)
			}
		}
	}
	for name := range built {
		if !seen[name] {
			add("warning", card.LorebookFile, "entry %q has no source file under %s/ (run build to drop it, or add the file)", name, SourceDir)
		}
	}
	keys := make([]string, 0, len(keywordOwners))
	for k := range keywordOwners {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		owners := keywordOwners[k]
		if len(owners) > 1 {
			add("warning", SourceDir, "keyword %q fires %d entries together (%s); stagger them so one sentence does not admit several", k, len(owners), strings.Join(owners, ", "))
		}
	}
	if constant > 5 {
		add("warning", SourceDir, "%d constant entries; keep them few and short, most always-on rules belong in the definition", constant)
	}
	res.Status = "ok"
	for _, f := range res.Findings {
		if f.Level == "error" {
			res.Status = "error"
		}
	}
	return res
}

func isCommonWord(k string) bool {
	runes := []rune(k)
	if len(runes) == 0 {
		return false
	}
	cjk := 0
	for _, r := range runes {
		if unicode.Is(unicode.Han, r) {
			cjk++
		}
	}
	if cjk == len(runes) {
		return len(runes) <= 1
	}
	if strings.ContainsAny(k, " /") {
		return false
	}
	return len(runes) <= 3
}
