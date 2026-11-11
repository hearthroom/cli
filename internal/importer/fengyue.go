package importer

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hearthroom/cli/internal/card"
)

// AI Fengyue (fyue.app) "export config" files.
//
// The export is one flat JSON object whose top-level keys are drawn at random
// from an alias pool on every export: eleven exports of the same test card
// used title, ttl, name, nm and app_name for the title alone. Keys inside the
// array items (Lorebook entries, regex rules, quick commands) are stable.
//
// This portable contract is mirrored at hearthroom/web/src/lib/fengyue-format.json,
// and testdata/fengyue-cases.json at hearthroom/web/test/fixtures/fengyue-cases.json.
// Update both copies together; both importers are checked against the same cases.
//
//go:embed fengyue-format.json
var fengyueContractJSON []byte

type fengyueContract struct {
	Fields map[string][]string `json:"fields"`
	Detect struct {
		MinFields          int      `json:"minFields"`
		SignalKeyPrefixes  []string `json:"signalKeyPrefixes"`
		WorldbookEntryKeys []string `json:"worldbookEntryKeys"`
	} `json:"detect"`
	Worldbook struct {
		KeyPrefix          string            `json:"keyPrefix"`
		KeySeparator       string            `json:"keySeparator"`
		TriggerRegion      map[string]string `json:"triggerRegion"`
		DefaultValueRegion int               `json:"defaultValueRegion"`
		DefaultProbability int               `json:"defaultProbability"`
	} `json:"worldbook"`
	OpeningMode struct {
		Openings    int `json:"openings"`
		PresetChats int `json:"presetChats"`
	} `json:"openingMode"`
	Limits struct {
		Openings int `json:"openings"`
	} `json:"limits"`
	Regex struct {
		Flags   string              `json:"flags"`
		Targets map[string][]string `json:"targets"`
	} `json:"regex"`
}

var fengyueFormat = func() fengyueContract {
	var c fengyueContract
	if err := json.Unmarshal(fengyueContractJSON, &c); err != nil {
		panic(err)
	}
	return c
}()

var fengyueAliasOf = func() map[string]string {
	m := map[string]string{}
	for field, aliases := range fengyueFormat.Fields {
		for _, a := range aliases {
			m[a] = field
		}
	}
	return m
}()

func fengyueSignalKey(k string) bool {
	for _, p := range fengyueFormat.Detect.SignalKeyPrefixes {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

func fengyuePick(raw map[string]any, field string) any {
	var fallback any
	seen := false
	for _, alias := range fengyueFormat.Fields[field] {
		v, ok := raw[alias]
		if !ok {
			continue
		}
		if !fengyueEmpty(v) {
			return v
		}
		if !seen {
			fallback, seen = v, true
		}
	}
	return fallback
}

func fengyueEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case []any:
		return len(t) == 0
	}
	return false
}

func fyStr(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func fyList(v any) []any {
	l, _ := v.([]any)
	return l
}

func fyStrings(items []any) []string {
	var out []string
	for _, o := range items {
		if s := fyStr(o); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func fyObjects(v any) []map[string]any {
	var out []map[string]any
	for _, item := range fyList(v) {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// fyNum reads a JSON number (decoded with UseNumber) or a numeric string; ok is false when absent.
func fyNum(v any) (float64, bool) {
	switch t := v.(type) {
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case float64:
		return t, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	}
	return 0, false
}

// fyKey renders a value the way JavaScript's String() does for map lookups ("2", "10").
func fyKey(v any) string {
	if f, ok := fyNum(v); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	if v == nil {
		return "undefined"
	}
	return fmt.Sprint(v)
}

// DecodeFengyue parses b and returns the object when it is a Fengyue export.
func DecodeFengyue(b []byte) (map[string]any, bool) {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if t := bytes.TrimLeft(b, " \t\r\n"); len(t) == 0 || t[0] != '{' {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, false
	}
	return raw, IsFengyueExport(raw)
}

// IsFengyueExport looks at which keys are present, not their values: the
// export writes empty fields too. It requires several field aliases at once
// plus a variable-panel key, never a single key.
func IsFengyueExport(raw map[string]any) bool {
	if raw == nil {
		return false
	}
	signal := false
	fields := map[string]bool{}
	for k := range raw {
		if fengyueSignalKey(k) {
			signal = true
		}
		if f, ok := fengyueAliasOf[k]; ok {
			fields[f] = true
		}
	}
	if !signal || len(fields) < fengyueFormat.Detect.MinFields {
		return false
	}
	if book := fyList(fengyuePick(raw, "worldbook")); len(book) > 0 {
		first, ok := book[0].(map[string]any)
		if !ok {
			return false
		}
		for _, k := range fengyueFormat.Detect.WorldbookEntryKeys {
			if _, ok := first[k]; !ok {
				return false
			}
		}
	}
	return true
}

// FengyueRegexFind wraps a delimiter-less pattern as /pattern/flags, escaping bare slashes.
func FengyueRegexFind(pattern string) string {
	var b strings.Builder
	escaped := false
	for _, r := range pattern {
		if r == '/' && !escaped {
			b.WriteString(`\/`)
		} else {
			b.WriteRune(r)
		}
		escaped = r == '\\' && !escaped
	}
	return "/" + b.String() + "/" + fengyueFormat.Regex.Flags
}

type fengyueNote struct{ code, text string }

var (
	fengyueURL      = regexp.MustCompile(`(?i)^https?://`)
	fengyueKeySplit = regexp.MustCompile(`[,，]`)
)

// FromFengyue maps a Fengyue export to a Result.
func FromFengyue(raw map[string]any, language string) *Result {
	r, notes := convertFengyue(raw, language)
	for _, n := range notes {
		r.Notes = append(r.Notes, n.text)
	}
	return r
}

func convertFengyue(raw map[string]any, language string) (*Result, []fengyueNote) {
	var notes []fengyueNote
	add := func(code, format string, args ...any) {
		notes = append(notes, fengyueNote{code, fmt.Sprintf(format, args...)})
	}
	r := &Result{Spec: "fengyue", Language: language}
	r.Name = fyStr(fengyuePick(raw, "name"))
	r.Summary = fyStr(fengyuePick(raw, "summary"))
	r.Definition = fyStr(fengyuePick(raw, "prompt"))
	r.OutputContract = fyStr(fengyuePick(raw, "prefix"))
	r.Custom = fyStr(fengyuePick(raw, "suffix"))

	// Both Fengyue opening modes let the player pick their own first line; the AI never speaks
	// first. Opening-choice mode picks from the openings list; preset-chat mode picks the line
	// inside each preset, which also plays a scripted question and answer first.
	presets := fyObjects(fengyuePick(raw, "presetChats"))
	choices := fyStrings(fyList(fengyuePick(raw, "openings")))
	// When the mode key uses an alias we have not seen, go by which list has content; with
	// both filled, do not guess which one players see.
	modeValue, _ := fyNum(fengyuePick(raw, "openingMode"))
	mode := "openings"
	switch {
	case int(modeValue) == fengyueFormat.OpeningMode.PresetChats:
		mode = "presets"
	case int(modeValue) == fengyueFormat.OpeningMode.Openings:
		mode = "openings"
	case len(presets) > 0 && len(choices) == 0:
		mode = "presets"
	case len(choices) > 0 && len(presets) == 0:
		mode = "openings"
	case len(presets) > 0:
		mode = "unknown"
	}
	var openings []string
	if mode == "presets" {
		for _, p := range presets {
			openings = append(openings, fyStrings(fyList(p["suggested_questions"]))...)
		}
		if len(presets) > 0 {
			add("presetChats", "the questions and answers of %d preset chats have no destination yet; the line players pick in each became an opening choice", len(presets))
		}
		if len(choices) > 0 {
			add("unusedOpenings", "%d opening lines are hidden on Fengyue (the card uses preset chats) and were not imported", len(choices))
		}
	} else if mode == "unknown" {
		openings = choices
		add("openingModeUnknown", "could not tell whether the card uses opening lines or preset chats on Fengyue; the opening lines were imported and %d preset chats were not", len(presets))
	} else {
		openings = choices
		if len(presets) > 0 {
			add("unusedPresets", "%d preset chats are hidden on Fengyue (the card uses opening lines) and were not imported", len(presets))
		}
	}
	max := fengyueFormat.Limits.Openings
	if len(openings) > max {
		add("prologueLimit", "%d opening choices beyond the limit of %d were dropped", len(openings)-max, max)
		openings = openings[:max]
	}
	r.Prologue = openings

	// Over-long text stays whole: the provider rejects it on push, and the author decides what to cut.
	for _, f := range []struct{ label, value, limitKey string }{
		{"name", r.Name, "name"}, {"summary", r.Summary, "summary"}, {"definition", r.Definition, "definition"},
		{"outputContract", r.OutputContract, "outputContract"}, {"custom", r.Custom, "customInstructions"},
	} {
		if n, cap := utf8.RuneCountInString(f.value), fengyueFieldLimit(language, f.limitKey); cap > 0 && n > cap {
			add("tooLong."+f.label, "%s is %d characters, over the limit of %d; shorten it before pushing", f.label, n, cap)
		}
	}

	// The detailed introduction becomes the opening page (opening.html): shown at the top of every
	// new chat, never sent to the model.
	intro, _ := fengyuePick(raw, "intro").(string)
	r.OpeningPage = strings.TrimSpace(intro)
	// No AI opening on Fengyue: with an opening page or opening choices the player speaks first here too.
	if r.OpeningPage == "" && len(r.Prologue) == 0 {
		add("noWelcome", "Fengyue cards have no opening message (the player speaks first) and this one has no opening choices or detailed introduction; write welcome.md before pushing")
	}

	if rows := fyObjects(fengyuePick(raw, "worldbook")); len(rows) > 0 {
		entries := fengyueBook(rows, add)
		drafts := BookEntriesToDrafts(entries, language)
		for i := range drafts {
			mo := map[string]any{}
			for k, v := range drafts[i].MatchOptions {
				mo[k] = v
			}
			if ext, ok := mo["extensions"].(map[string]any); ok {
				rest := map[string]any{}
				for k, v := range ext {
					if k == "fengyue" {
						if fy, ok := v.(map[string]any); ok {
							drafts[i].TriggerRegion, _ = fy["triggerRegion"].(string)
						}
						continue
					}
					rest[k] = v
				}
				if len(rest) > 0 {
					mo["extensions"] = rest
				} else {
					delete(mo, "extensions")
				}
			}
			drafts[i].MatchOptions = mo
		}
		r.Lorebook = &LorebookDraft{Name: r.Name, Entries: drafts}
		for _, n := range BookEntryNotes(entries, language) {
			add("split", "%s", n)
		}
	}

	rules := fengyueRegex(fyObjects(fengyuePick(raw, "regex")), add)
	if css, _ := fengyuePick(raw, "css").(string); strings.TrimSpace(css) != "" {
		// Page-wide CSS is emitted once, independently of message replacement rules.
		r.CardCSS, r.CardCSSDialect = strings.TrimSpace(css), "fengyue"
	}
	if len(rules) > 0 || r.OpeningPage != "" || r.CardCSS != "" {
		r.Rules = &RuleSet{Rules: rules, PageMode: "sandbox", Format: "fengyue"}
	}

	r.Variables = fengyueVariables(raw, add)
	fengyueUnmapped(raw, add)
	return r, notes
}

// fengyueVariables maps the variable panel to card variables. Fengyue's data
// skeleton is a JSON string of bare values (no per-value rules), so it becomes
// convention plain; the variable prompt tells the model how values change and
// becomes variables.md; the panel skin (Handlebars) becomes panel.hbs. An empty
// skeleton means the panel is not used: a prompt alone does not turn variables
// on and is kept in the report instead. A skeleton that is not valid JSON is
// kept as text in data, so card check points at it and push waits for a fix.
func fengyueVariables(raw map[string]any, add func(code, format string, args ...any)) *card.Variables {
	skeleton := ""
	var parsed *card.JSONNode
	switch v := fengyuePick(raw, "variableSkeleton").(type) {
	case map[string]any:
		parsed = card.JSONFromAny(v)
		skeleton = parsed.Compact()
	case string:
		skeleton = strings.TrimSpace(v)
	}
	prompt := fyStr(fengyuePick(raw, "variablePrompt"))
	template, _ := fengyuePick(raw, "variableTemplate").(string)
	template = strings.TrimSpace(template)
	if skeleton == "" {
		if prompt != "" {
			add("variablesPromptOnly", "the variable panel had a variable prompt but no data skeleton, so variables were not turned on; prompt text: %s", prompt)
		}
		if template != "" {
			add("variablesTemplateOnly", "the variable panel had a panel skin but no data skeleton, so variables were not turned on")
		}
		return nil
	}
	v := &card.Variables{Convention: "plain", Rules: prompt, Panel: template}
	if parsed == nil {
		if n, err := card.ParseJSON([]byte(skeleton)); err == nil && n.Kind == card.JSONObject {
			parsed = n
		}
	}
	if parsed != nil {
		v.Data = json.RawMessage(parsed.Compact())
		add("variablesConverted", "the variable panel became card variables (%s); review them before pushing", card.VariablesFile)
	} else {
		text, _ := json.Marshal(skeleton)
		v.Data = text
		add("variablesPartial", "the variable panel's data skeleton is not valid JSON; it was kept as text in %s and must be fixed before pushing", card.VariablesFile)
	}
	return v
}

// fengyueFieldLimit mirrors the site's fallback limits for a new card (web role-draft fallbackLimits).
func fengyueFieldLimit(language, key string) int {
	l := strings.ToLower(language)
	en := strings.HasPrefix(l, "en")
	switch key {
	case "name":
		return 60
	case "outputContract":
		return 2000
	case "summary":
		if en {
			return 2500
		}
		return 500
	case "definition":
		if en {
			return 50000
		}
		return 10000
	case "customInstructions":
		if strings.HasPrefix(l, "zh") {
			return 500
		}
		return 1500
	}
	return 0
}

func fengyueBook(rows []map[string]any, add func(code, format string, args ...any)) []BookEntry {
	w := fengyueFormat.Worldbook
	region, position, probability, keyFormat, group := 0, 0, 0, 0, 0
	entries := make([]BookEntry, 0, len(rows))
	for _, row := range rows {
		key := fyStr(row["key"])
		if _, ok := w.TriggerRegion[fyKey(row["key_region"])]; !ok {
			region++
		}
		if v, ok := row["value_region"]; ok {
			if f, _ := fyNum(v); int(f) != w.DefaultValueRegion {
				position++
			}
		}
		if v, ok := row["probability"]; ok {
			if f, _ := fyNum(v); int(f) != w.DefaultProbability {
				probability++
			}
		}
		matchType, hasType := row["match_type"]
		mt, _ := fyNum(matchType)
		isOr := strings.HasPrefix(key, w.KeyPrefix) && (!hasType || mt == 2)
		if !isOr {
			keyFormat++
		}
		if fyStr(row["group"]) != "" {
			group++
		}
		// Only the _or_ prefix (any keyword, separated by @wb@) can be split into OR keywords; other prefixes stay as written for the author to fix.
		var keys []string
		if isOr {
			for _, part := range strings.Split(key[len(w.KeyPrefix):], w.KeySeparator) {
				for _, k := range fengyueKeySplit.Split(part, -1) {
					if k = strings.TrimSpace(k); k != "" {
						keys = append(keys, k)
					}
				}
			}
		} else if key != "" {
			keys = []string{key}
		}
		e := BookEntry{
			Keys:       keys,
			Content:    fyStr(row["value"]),
			Enabled:    row["enable"] != false,
			Extensions: map[string]any{"fengyue": map[string]any{"triggerRegion": w.TriggerRegion[fyKey(row["key_region"])]}},
		}
		if f, ok := fyNum(row["sort"]); ok && f == float64(int(f)) {
			e.InsertionOrder = int(f)
		}
		if f, ok := fyNum(row["depth"]); ok && f == float64(int(f)) && f > 0 && f <= 100 {
			e.ScanDepth = int(f)
		}
		entries = append(entries, e)
	}
	if region > 0 {
		add("entryRegion", "%d Lorebook entries had an unverified trigger source; they now trigger on both sides", region)
	}
	if position > 0 {
		add("entryPosition", "%d Lorebook entries carry an insertion position; the provider places entries itself, so it was not imported", position)
	}
	if probability > 0 {
		add("entryProbability", "%d Lorebook entries carry a trigger chance; entries here are added whenever a keyword matches", probability)
	}
	if keyFormat > 0 {
		add("entryKeyFormat", "%d Lorebook entries use AND or regex keywords; the keywords were kept as written and need rewriting", keyFormat)
	}
	if group > 0 {
		add("entryGroup", "%d Lorebook entries carry a group name, which has no destination", group)
	}
	return entries
}

func fengyueRegex(rows []map[string]any, add func(code, format string, args ...any)) []RegexRule {
	scopes := fengyueFormat.Regex.Targets
	unknown, history := 0, 0
	var rules []RegexRule
	for _, row := range rows {
		if _, ok := scopes[fyKey(row["scope"])]; !ok {
			unknown++
		}
		if row["is_only_render"] == false {
			history++
		}
	}
	if unknown > 0 {
		add("regexScope", "%d regex rules had an unverified scope; they now apply to every message", unknown)
	}
	if history > 0 {
		add("regexHistory", "%d regex rules rewrite the chat history on Fengyue; display rules here change only what is shown", history)
	}
	i := 0
	for _, row := range rows {
		pattern, _ := row["match_regex"].(string)
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		i++
		name := fyStr(row["title"])
		if name == "" {
			name = fmt.Sprintf("#%d", i)
		}
		replace, _ := row["replace_text"].(string)
		rule := RegexRule{Name: truncateRunes(name, 40), Find: FengyueRegexFind(pattern), Replace: replace, Enabled: row["is_enable"] != false}
		if t, ok := scopes[fyKey(row["scope"])]; ok {
			targets := append([]string{}, t...)
			rule.Targets = &targets
		}
		rules = append(rules, rule)
	}
	return rules
}

// fengyueUnmapped reports every feature that has no destination.
func fengyueUnmapped(raw map[string]any, add func(code, format string, args ...any)) {
	if n := len(fyList(fengyuePick(raw, "shortcuts"))); n > 0 {
		add("shortcuts", "%d quick commands have no destination", n)
	}
	banned := 0
	for _, w := range fyList(fengyuePick(raw, "bannedWords")) {
		if fyStr(w) != "" {
			banned++
		}
	}
	if banned > 0 {
		add("bannedWords", "%d blocked words have no destination", banned)
	}
	// The skeleton, prompt and skin are mapped by fengyueVariables; this is only
	// for variable-panel keys the alias table does not know.
	for k, v := range raw {
		if _, alias := fengyueAliasOf[k]; fengyueSignalKey(k) && !alias && fyStr(v) != "" {
			add("variablesUnknown", "the variable panel has a field this importer does not recognise; it was not imported")
			break
		}
	}
	// The export writes every field's key, empty ones too; a field with none of its aliases
	// present means Fengyue used a name we have not collected.
	missing := 0
	for _, aliases := range fengyueFormat.Fields {
		found := false
		for _, a := range aliases {
			if _, ok := raw[a]; ok {
				found = true
				break
			}
		}
		if !found {
			missing++
		}
	}
	if missing > 0 {
		add("missingFields", "%d fields were not found in the file (Fengyue may have renamed them); check the folder against the card on Fengyue", missing)
	}
	// Remaining keys have unmeasured aliases; recognise them by shape.
	other := 0
	for k, v := range raw {
		if _, alias := fengyueAliasOf[k]; alias || fengyueSignalKey(k) {
			continue
		}
		obj, isObj := v.(map[string]any)
		switch {
		case isObj && (fyStr(obj["big_prompt"]) != "" || fyStr(obj["small_prompt"]) != ""):
			add("summaryConfig", "custom summary prompts were not imported; memory is organised by the provider")
		case isObj && obj["tracks"] != nil:
			if tracks, ok := obj["tracks"].([]any); ok && len(tracks) > 0 {
				add("music", "%d background music tracks have no destination", len(tracks))
			}
		case fengyueHasContent(v):
			other++
		}
	}
	if other > 0 {
		add("otherFields", "%d other items have no destination (such as background images, covers, CG swaps or unrecognised fields); upload images again", other)
	}
}

func fengyueHasContent(v any) bool {
	switch t := v.(type) {
	case []any:
		return len(t) > 0
	case string:
		s := strings.TrimSpace(t)
		return fengyueURL.MatchString(s) || utf8.RuneCountInString(s) > 30 || strings.Contains(t, "\n")
	}
	return false
}
