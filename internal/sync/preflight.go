package sync

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/hearthroom/cli/internal/card"
)

// These v1 limits match provider role/author_asset.go and role/limits.go.
// TestProviderPreflightContract checks the receiving source when supplied.
const ruleReplaceMax = 256 << 10
const rulesTotalMax = 32 << 20
const trialBodyMax = 48 << 20

var fieldLimits = map[string][2]int{
	"roleDesc": {500, 2500}, "roleDetailDesc": {10000, 50000},
	"roleWelcome": {8000, 10000}, "customInstructions": {2000, 4000},
	"roleOutputContract": {4000, 8000}, "roleReadme": {20000, 40000},
}

// RuleSize measures the actual replace string built for the wire, not JSON escaping.
type RuleSize struct {
	Index          int    `json:"index"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	RewrittenBytes int    `json:"rewrittenBytes"`
	LimitBytes     int    `json:"limitBytes"`
	RemainingBytes int    `json:"remainingBytes"`
}

func preflight(f *card.Folder, p card.Payload, language string, checkSizes bool) ([]RuleSize, error) {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	checkText := func(field, value string) {
		limits, ok := fieldLimits[field]
		if !ok {
			return
		}
		max := limits[0]
		if language == "en" {
			max = limits[1]
		}
		if n := utf8.RuneCountInString(value); checkSizes && n > max {
			add("%s contains %d characters; the limit is %d characters (%d %s over). Shorten it before pushing.", field, n, max, n-max, unit(n-max, "character"))
		}
	}
	for _, field := range []string{"roleDesc", "roleDetailDesc", "customInstructions", "roleOutputContract", "roleReadme"} {
		if value, ok := p.Card[field].(string); ok {
			checkText(field, value)
		}
	}
	if p.Welcome != nil {
		value, _ := p.Welcome["roleWelcome"].(string)
		checkText("roleWelcome", value)
	}
	defaults := f.Manifest.ResponseDefaults
	// The receiving service still accepts these two agency aliases on existing cards.
	if alias := map[string]string{"assist": "protect", "lines": "coauthor"}[defaults["agency"]]; alias != "" {
		defaults = card.ResponseDefaults{}
		for k, v := range f.Manifest.ResponseDefaults {
			defaults[k] = v
		}
		defaults["agency"] = alias
	}
	problems = append(problems, card.CheckResponseDefaults(defaults)...)
	for _, slot := range []struct{ wire, local string }{{"roleAvatar", "portrait"}, {"roleBackground", "background"}, {"roleBackgroundLandscape", "backgroundLandscape"}, {"roleShareImage", "share"}} {
		raw, _ := p.Card[slot.wire].(string)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			add("Invalid media URL in card.json: media.%s. Correct the URL before pushing.", slot.local)
		}
	}
	var sizes []RuleSize
	if p.AuthorAsset != nil {
		allowed := func(key string, values ...string) {
			value, _ := p.AuthorAsset[key].(string)
			for _, v := range values {
				if value == v {
					return
				}
			}
			add("rules.json: %s has an unsupported value %q. Use %s.", key, value, strings.Join(values, ", "))
		}
		allowed("cardFormat", "", "mmd", "tavern")
		allowed("mountLayer", "", "under", "over", "cover")
		allowed("pageMode", "", "classic", "immersive", "sandbox")
		problems = append(problems, card.CheckSharedState(f.Rules.SharedState)...)
		total := 0
		rules, _ := p.AuthorAsset["rules"].([]map[string]any)
		for i, r := range rules {
			id, _ := r["id"].(string)
			name, _ := r["name"].(string)
			find, _ := r["find"].(string)
			replace, _ := r["replace"].(string)
			n := len(replace)
			sizes = append(sizes, RuleSize{Index: i + 1, ID: id, Name: name, RewrittenBytes: n, LimitBytes: ruleReplaceMax, RemainingBytes: ruleReplaceMax - n})
			if strings.TrimSpace(find) == "" {
				add("Rule %d (%q, %q) has an empty find pattern. Add a pattern or remove the rule before pushing.", i+1, id, name)
			}
			if checkSizes && n > ruleReplaceMax {
				add("Rule %d (%q, %q) replacement is %d bytes after URL rewriting; the limit is %d bytes (%d %s over). Reduce the replacement content before pushing.", i+1, id, name, n, ruleReplaceMax, n-ruleReplaceMax, unit(n-ruleReplaceMax, "byte"))
			}
			total += len(find) + n + len(name)
		}
		if checkSizes && total > rulesTotalMax {
			add("Display rules total %d bytes (find + replacement + name); the limit is %d bytes (%d %s over). Reduce the rules before pushing.", total, rulesTotalMax, total-rulesTotalMax, unit(total-rulesTotalMax, "byte"))
		}
	}
	if p.Worldbook != nil {
		name, _ := p.Worldbook["name"].(string)
		if name == "" {
			name = f.Manifest.Name
		}
		if n := utf8.RuneCountInString(name); n > 120 {
			add("lorebook.json: name contains %d characters; the limit is 120 characters (%d %s over). Shorten the name before pushing.", n, n-120, unit(n-120, "character"))
		}
		if format, _ := p.Worldbook["format"].(string); format != "" && format != "tavern" {
			add(`lorebook.json: format must be "tavern" or omitted.`)
		}
		entries, _ := p.Worldbook["entries"].([]map[string]any)
		for i, e := range entries {
			name, _ := e["name"].(string)
			if strings.TrimSpace(name) == "" {
				add("lorebook.json: entry %d has no name. Add a name before pushing.", i+1)
			}
			if n := utf8.RuneCountInString(name); n > 120 {
				add("lorebook.json: entry %d (%q) name contains %d characters; the limit is 120 characters (%d %s over). Shorten the name before pushing.", i+1, name, n, n-120, unit(n-120, "character"))
			}
			allowed := func(key string, values ...string) {
				value, _ := e[key].(string)
				for _, v := range values {
					if value == v {
						return
					}
				}
				add("lorebook.json: entry %d (%q): %s has an unsupported value %q. Use %s.", i+1, name, key, value, strings.Join(values, ", "))
			}
			allowed("category", "", "rule", "character", "location", "item", "event", "custom")
			allowed("triggerRegion", "", "both", "user_only", "ai_only")
			if mo, ok := e["matchOptions"]; ok {
				raw, err := json.Marshal(mo)
				var options struct {
					Selective       *bool                      `json:"selective"`
					ScanDepth       int                        `json:"scanDepth"`
					GroupID         string                     `json:"groupId"`
					GroupOrder      int                        `json:"groupOrder"`
					Order           *int                       `json:"order"`
					Extensions      map[string]json.RawMessage `json:"extensions"`
					CaseSensitive   bool                       `json:"caseSensitive"`
					MatchWholeWords bool                       `json:"matchWholeWords"`
					SelectiveLogic  int                        `json:"selectiveLogic"`
				}
				if err != nil || json.Unmarshal(raw, &options) != nil {
					add("lorebook.json: entry %d (%q): matchOptions must match the supported Lorebook options format.", i+1, name)
					continue
				}
				if options.ScanDepth < 0 || options.ScanDepth > 100 || len(options.GroupID) > 128 || options.GroupOrder < 0 || options.GroupOrder > 10000 || options.SelectiveLogic < 0 || options.SelectiveLogic > 3 {
					add("lorebook.json: entry %d (%q): matchOptions is out of range (scanDepth 0-100, groupId up to 128 bytes, groupOrder 0-10000, selectiveLogic 0-3). Correct these values before pushing.", i+1, name)
				}
				ext, _ := json.Marshal(options.Extensions)
				if len(ext) > 32768 {
					add("lorebook.json: entry %d (%q): matchOptions.extensions is %d bytes; the limit is 32768 bytes (%d %s over). Reduce the extensions before pushing.", i+1, name, len(ext), len(ext)-32768, unit(len(ext)-32768, "byte"))
				}
			}
		}
	}
	if !checkSizes {
		sizes = nil
	}
	if len(problems) > 0 {
		return sizes, &PreflightError{Problems: problems, Rules: sizes}
	}
	return sizes, nil
}

func textReferences(f *card.Folder, rel string) bool {
	clone := *f
	clone.Manifest.Media = card.Media{}
	before, err := clone.Build(nil)
	if err != nil {
		return true
	}
	urls := map[string]string{rel: "https://planned.invalid/asset"}
	for _, dir := range f.AssetDirs() {
		if strings.HasPrefix(rel, dir) {
			urls[dir] = "https://planned.invalid/dir/"
		}
	}
	after, err := clone.Build(urls)
	return err != nil || card.Digest(before) != card.Digest(after)
}

func preflightPrepared(p card.Payload, owned bool, entryMax, trialMax int, evict bool) error {
	if !owned && utf8.RuneCountInString(p.Name) > 100 {
		return preflightErrorf("Trial name contains %d characters; the limit is 100 characters (%d %s over). Shorten the name before pushing.", utf8.RuneCountInString(p.Name), utf8.RuneCountInString(p.Name)-100, unit(utf8.RuneCountInString(p.Name)-100, "character"))
	}
	if p.Worldbook != nil {
		entries, _ := p.Worldbook["entries"].([]map[string]any)
		if !owned && len(entries) > trialMax {
			return preflightErrorf("Trial cards allow %d Lorebook entries; this configuration needs %d after grouping. Use --create for a larger owned card (unlisted by default; anyone with the link can play its latest saved content).", trialMax, len(entries))
		}
		for i, e := range entries {
			v, _ := e["content"].(string)
			if n := utf8.RuneCountInString(v); entryMax > 0 && n > entryMax {
				return preflightErrorf("Lorebook entry %d contains %d characters; the limit is %d characters (%d %s over). Shorten the entry or remove explicit grouping to allow automatic splitting.", i+1, n, entryMax, n-entryMax, unit(n-entryMax, "character"))
			}
		}
	}
	if !owned {
		body := map[string]any{"name": p.Name}
		if evict {
			body["evict"] = true
		}
		for _, s := range card.Sections {
			if sec := p.Section(s); sec != nil {
				body[s] = sec
			}
		}
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		if len(raw) > trialBodyMax {
			return preflightErrorf("Trial request body is %d bytes; the limit is %d bytes (%d %s over). Reduce the content before pushing.", len(raw), trialBodyMax, len(raw)-trialBodyMax, unit(len(raw)-trialBodyMax, "byte"))
		}
	}
	return nil
}

func unit(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// PreflightError keeps the report machine-readable when a limit is exceeded.
type PreflightError struct {
	Problems []string   `json:"problems"`
	Rules    []RuleSize `json:"rules,omitempty"`
}

func (e *PreflightError) Error() string {
	return "Push preflight failed; no card content was written:\n  - " + strings.Join(e.Problems, "\n  - ")
}
func (e *PreflightError) ErrorDetail() any { return e }
func preflightErrorf(format string, args ...any) error {
	return &PreflightError{Problems: []string{fmt.Sprintf(format, args...)}}
}

type RuleTotalSize struct {
	Bytes          int `json:"bytes"`
	LimitBytes     int `json:"limitBytes"`
	RemainingBytes int `json:"remainingBytes"`
}

func ruleTotal(p card.Payload) *RuleTotalSize {
	if p.AuthorAsset == nil {
		return nil
	}
	total := 0
	rules, _ := p.AuthorAsset["rules"].([]map[string]any)
	for _, r := range rules {
		for _, key := range []string{"find", "replace", "name"} {
			v, _ := r[key].(string)
			total += len(v)
		}
	}
	return &RuleTotalSize{Bytes: total, LimitBytes: rulesTotalMax, RemainingBytes: rulesTotalMax - total}
}
