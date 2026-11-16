package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hearthroom/cli/internal/card"
)

// Rili AI (talkaicc.com) exports:
//   - whole card JSON (<title>.json)
//   - chat settings override JSON (rili-chat-settings-<cardId>.json)
//   - module JSON (rili-mod-<id>.json, format == "rili-mod")
//   - standalone lorebook JSON (lorebook.json, {prompt: {lorebook: [...]}})
//
// Test fixtures in testdata/rili-cases.json are shared byte-for-byte with
// hearthroom/web/test/fixtures/rili-cases.json.

type riliNote struct {
	code string
	text string
}

type riliEntryMeta struct {
	triggerRegion  string
	region         bool
	position       bool
	probability    bool
	weighted       bool
	blacklistOnAnd bool
}

var riliKnownTopKeys = map[string]bool{
	"id":                             true,
	"authorId":                       true,
	"title":                          true,
	"description":                    true,
	"background":                     true,
	"backgroundWidth":                true,
	"backgroundHeight":               true,
	"cover":                          true,
	"coverWidth":                     true,
	"coverHeight":                    true,
	"introPage":                      true,
	"changelog":                      true,
	"createdAt":                      true,
	"updatedAt":                      true,
	"initPublicAt":                   true,
	"prompt":                         true,
	"promptSizeBytes":                true,
	"renderTemplate":                 true,
	"assistantRegexReplace":          true,
	"assistantRegexReplaceSizeBytes": true,
	"enableMemorySchema":             true,
	"memorySchema":                   true,
	"injectStyles":                   true,
	"injectStylesSizeBytes":          true,
	"injectedStyles":                 true,
	"options":                        true,
	"language":                       true,
	"containsNSFW":                   true,
	"publicAccess":                   true,
	"deletedAt":                      true,
	"isAnonymous":                    true,
	"enableSafeguard":                true,
	"defaultModelSettings":           true,
	"model":                          true,
	"contextWindow":                  true,
	"modelSettings":                  true,
	"assistantPrefill":               true,
	"totalConversations":             true,
	"totalPlayedUsers":               true,
	"deeplyPlayed":                   true,
	"maxNonAuthorPlayCount":          true,
	"fixedPromptLength":              true,
	"lorebookLength":                 true,
	"cardsTags":                      true,
	"cardsVoices":                    true,
	"format":                         true,
	"formatVersion":                  true,
	"payload":                        true,
	"name":                           true,
}

var riliKnownPromptKeys = map[string]bool{
	"systemPrompt":      true,
	"headPrompt":        true,
	"rearPrompt":        true,
	"suggestionsPrompt": true,
	"lorebook":          true,
}

var riliCardSignalKeys = []string{
	"title",
	"introPage",
	"assistantRegexReplace",
	"injectStyles",
	"enableMemorySchema",
	"memorySchema",
	"renderTemplate",
	"defaultModelSettings",
	"modelSettings",
	"cardsTags",
	"options",
	"assistantPrefill",
}

func riliStr(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func riliRawStr(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func riliList(v any) []any {
	l, _ := v.([]any)
	return l
}

func riliStrList(v any) []string {
	var out []string
	for _, item := range riliList(v) {
		if s := riliStr(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func riliObjects(v any) []map[string]any {
	var out []map[string]any
	for _, item := range riliList(v) {
		if m, ok := item.(map[string]any); ok && m != nil {
			out = append(out, m)
		}
	}
	return out
}

func riliHasKey(m map[string]any, k string) bool {
	if m == nil {
		return false
	}
	_, ok := m[k]
	return ok
}

func isRiliLorebookEntry(v any) bool {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return false
	}
	if _, ok := m["triggerWords"].([]any); !ok {
		return false
	}
	return riliHasKey(m, "content") || riliHasKey(m, "contents")
}

func isValidRiliLorebookArray(v any) bool {
	arr, ok := v.([]any)
	if !ok {
		return false
	}
	for _, item := range arr {
		if !isRiliLorebookEntry(item) {
			return false
		}
	}
	return true
}

func decodeRiliJSONObject(b []byte) (map[string]any, bool) {
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
	return raw, raw != nil
}

// IsRiliLorebook reports whether raw is a standalone Rili lorebook.json ({prompt: {lorebook: [...]}}).
func IsRiliLorebook(raw map[string]any) bool {
	if len(raw) == 0 || riliStr(raw["format"]) == "rili-mod" {
		return false
	}
	for k := range raw {
		if k != "prompt" && k != "promptSizeBytes" && k != "lorebookLength" {
			return false
		}
	}
	prompt, ok := raw["prompt"].(map[string]any)
	if !ok || len(prompt) == 0 {
		return false
	}
	for k := range prompt {
		if k != "lorebook" {
			return false
		}
	}
	return isValidRiliLorebookArray(prompt["lorebook"])
}

// IsRiliLorebookBytes reports whether b is a standalone Rili lorebook.json.
func IsRiliLorebookBytes(b []byte) bool {
	raw, ok := decodeRiliJSONObject(b)
	return ok && IsRiliLorebook(raw)
}

// IsRiliExport reports whether raw is a Rili whole card, chat-settings override,
// or rili-mod module file (standalone lorebook.json returns false).
func IsRiliExport(raw map[string]any) bool {
	if len(raw) == 0 || IsRiliLorebook(raw) {
		return false
	}
	if riliStr(raw["format"]) == "rili-mod" {
		p, ok := raw["payload"].(map[string]any)
		return ok && p != nil
	}
	prompt, ok := raw["prompt"].(map[string]any)
	if !ok || prompt == nil {
		return false
	}
	if lb, exists := prompt["lorebook"]; exists && lb != nil && !isValidRiliLorebookArray(lb) {
		return false
	}
	hasPromptField := riliHasKey(prompt, "systemPrompt") ||
		riliHasKey(prompt, "headPrompt") ||
		riliHasKey(prompt, "rearPrompt") ||
		riliHasKey(prompt, "suggestionsPrompt") ||
		isValidRiliLorebookArray(prompt["lorebook"])
	if !hasPromptField {
		return false
	}
	for _, k := range riliCardSignalKeys {
		if riliHasKey(raw, k) {
			return true
		}
	}
	return riliHasKey(prompt, "systemPrompt") && (riliHasKey(prompt, "headPrompt") || riliHasKey(prompt, "rearPrompt"))
}

// DecodeRili parses b and returns the decoded map when it is a Rili card/settings/mod export.
func DecodeRili(b []byte) (map[string]any, bool) {
	raw, ok := decodeRiliJSONObject(b)
	if !ok || !IsRiliExport(raw) {
		return nil, false
	}
	return raw, true
}

func formatRiliEntryName(folder string, index int) string {
	suffix := fmt.Sprintf("#%d", index)
	if folder == "" {
		return suffix
	}
	maxFolder := 20 - 3 - utf8.RuneCountInString(suffix)
	if maxFolder < 1 {
		maxFolder = 1
	}
	shortFolder := truncateRunes(folder, maxFolder)
	return fmt.Sprintf("[%s] %s", shortFolder, suffix)
}

func convertRiliLorebookEntries(rows []map[string]any) []BookEntry {
	entries := make([]BookEntry, 0, len(rows))
	for idx, row := range rows {
		folder := riliStr(row["folder"])
		name := formatRiliEntryName(folder, idx+1)
		words := riliStrList(row["triggerWords"])
		blacks := riliStrList(row["blackListWords"])
		isAnd := strings.EqualFold(riliStr(row["triggerWordsType"]), "AND") && len(words) >= 2

		keys := words
		var secondaryKeys []string
		selective := false
		selectiveLogic := 0
		blacklistOnAnd := false

		if isAnd {
			keys = []string{words[0]}
			secondaryKeys = append([]string(nil), words[1:]...)
			selective = true
			selectiveLogic = 3
			if len(blacks) > 0 {
				blacklistOnAnd = true
			}
		} else if len(blacks) > 0 {
			keys = words
			secondaryKeys = blacks
			selective = true
			if strings.EqualFold(riliStr(row["blackListWordsType"]), "AND") {
				selectiveLogic = 1
			} else {
				selectiveLogic = 2
			}
		}

		weighted := false
		content := ""
		if s, ok := row["content"].(string); ok {
			content = strings.TrimSpace(s)
		}
		if riliStr(row["contentMethod"]) == "weighted" {
			if contentsArr, ok := row["contents"].([]any); ok {
				var parts []string
				for _, item := range contentsArr {
					if cm, ok := item.(map[string]any); ok {
						if cs, ok := cm["content"].(string); ok && strings.TrimSpace(cs) != "" {
							parts = append(parts, strings.TrimSpace(cs))
						}
					}
				}
				content = strings.Join(parts, "\n\n")
				weighted = true
			}
		}

		ranges := riliStrList(row["triggerRange"])
		hasUser := contains(ranges, "user")
		hasAssistant := contains(ranges, "assistant")
		hasOther := false
		for _, r := range ranges {
			if r != "user" && r != "assistant" {
				hasOther = true
				break
			}
		}
		triggerRegion := ""
		if hasUser && !hasAssistant {
			triggerRegion = "user_only"
		} else if hasAssistant && !hasUser {
			triggerRegion = "ai_only"
		}
		_, rangeIsArr := row["triggerRange"].([]any)
		region := hasOther || (rangeIsArr && len(ranges) > 0 && !hasUser && !hasAssistant)

		impacts := riliStrList(row["impactRange"])
		position := false
		for _, r := range impacts {
			if r != "system" {
				position = true
				break
			}
		}

		probability := false
		if probVal, exists := row["probability"]; exists && probVal != nil {
			if f, ok := fyNum(probVal); !ok || f != 100 {
				probability = true
			}
		}

		order := 0
		if f, ok := fyNum(row["priority"]); ok && f == math.Trunc(f) {
			order = int(f)
		}
		scanDepth := 0
		if f, ok := fyNum(row["scanDepth"]); ok && f == math.Trunc(f) && f > 0 && f <= 100 {
			scanDepth = int(f)
		}

		selCopy := selective
		meta := riliEntryMeta{
			triggerRegion:  triggerRegion,
			region:         region,
			position:       position,
			probability:    probability,
			weighted:       weighted,
			blacklistOnAnd: blacklistOnAnd,
		}
		entries = append(entries, BookEntry{
			Name:           name,
			Keys:           keys,
			SecondaryKeys:  secondaryKeys,
			Selective:      &selCopy,
			SelectiveLogic: selectiveLogic,
			Content:        content,
			Enabled:        row["enabled"] != false,
			InsertionOrder: order,
			ScanDepth:      scanDepth,
			Extensions:     map[string]any{"__rili": meta},
		})
	}
	return entries
}

func riliBookFromRaw(obj map[string]any) *Book {
	if IsRiliLorebook(obj) {
		prompt, _ := obj["prompt"].(map[string]any)
		rows := riliObjects(prompt["lorebook"])
		return &Book{
			Name:        "",
			Description: "",
			Entries:     convertRiliLorebookEntries(rows),
		}
	}
	if IsRiliExport(obj) {
		source := obj
		if riliStr(obj["format"]) == "rili-mod" {
			if p, ok := obj["payload"].(map[string]any); ok {
				source = p
			}
		}
		prompt, _ := source["prompt"].(map[string]any)
		if prompt == nil || !isValidRiliLorebookArray(prompt["lorebook"]) {
			return nil
		}
		rows := riliObjects(prompt["lorebook"])
		if len(rows) == 0 {
			return nil
		}
		return &Book{
			Name:        riliStr(obj["title"]),
			Description: "",
			Entries:     convertRiliLorebookEntries(rows),
		}
	}
	return nil
}

func riliBookNotes(entries []BookEntry) []riliNote {
	region, weighted, blacklistOnAnd, position, probability := 0, 0, 0, 0, 0
	for _, e := range entries {
		meta, ok := e.Extensions["__rili"].(riliEntryMeta)
		if !ok {
			continue
		}
		if meta.region {
			region++
		}
		if meta.weighted {
			weighted++
		}
		if meta.blacklistOnAnd {
			blacklistOnAnd++
		}
		if meta.position {
			position++
		}
		if meta.probability {
			probability++
		}
	}
	var notes []riliNote
	if region > 0 {
		notes = append(notes, riliNote{
			code: "entryRegion",
			text: fmt.Sprintf("%d Lorebook entries scanned system or memory fields on RiliAI; they now scan chat messages", region),
		})
	}
	if weighted > 0 {
		notes = append(notes, riliNote{
			code: "entryWeighted",
			text: fmt.Sprintf("%d weighted Lorebook entries were merged so all segments load together when matched", weighted),
		})
	}
	if blacklistOnAnd > 0 {
		notes = append(notes, riliNote{
			code: "entryBlacklistOnAnd",
			text: fmt.Sprintf("%d Lorebook entries had both AND trigger words and blocklist words; AND trigger words were kept and blocklist words were skipped", blacklistOnAnd),
		})
	}
	if position > 0 {
		notes = append(notes, riliNote{
			code: "entryPosition",
			text: fmt.Sprintf("%d Lorebook entries carry an insertion position; the provider places entries itself, so it was not imported", position),
		})
	}
	if probability > 0 {
		notes = append(notes, riliNote{
			code: "entryProbability",
			text: fmt.Sprintf("%d Lorebook entries carry a trigger chance; entries here are added whenever a keyword matches", probability),
		})
	}
	return notes
}

func riliBookNoteCodes(entries []BookEntry) []string {
	notes := riliBookNotes(entries)
	out := make([]string, 0, len(notes))
	for _, n := range notes {
		out = append(out, n.code)
	}
	return out
}

var (
	riliSlashRegex = regexp.MustCompile(`(?s)^/(.+)/([gimsuy]*)$`)
	riliHbsTagRe   = regexp.MustCompile(`(?s)\{\{\{.*?\}\}\}|\{\{.*?\}\}`)
	riliSuggestRe  = regexp.MustCompile(`\{\{[^{}]*\bsuggestions\b`)
)

func escapeRiliBareSlashes(src string) string {
	var b strings.Builder
	escaped := false
	for _, r := range src {
		switch {
		case r == '/' && !escaped:
			b.WriteString(`\/`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
		escaped = r == '\\' && !escaped
	}
	return b.String()
}

func escapeRiliLiteralForRegex(lit string) string {
	var b strings.Builder
	for _, r := range lit {
		switch r {
		case '.', '*', '+', '?', '^', '$', '{', '}', '(', ')', '|', '[', ']', '\\', '/':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\r':
			b.WriteString(`\r`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// RiliRegexFind converts a Rili regex field into a /pattern/flags literal.
// Rili only compiles /pattern/flags as a RegExp; bare strings are literal split/join.
func RiliRegexFind(pattern string) string {
	if m := riliSlashRegex.FindStringSubmatch(pattern); m != nil {
		return "/" + escapeRiliBareSlashes(m[1]) + "/" + m[2]
	}
	return "/" + escapeRiliLiteralForRegex(pattern) + "/g"
}

var riliRoleTargets = map[string][]string{
	"assistant": {"ai"},
	"user":      {"user"},
	"all":       {"ai", "user"},
}

func convertRiliRegex(rows []map[string]any, add func(code, format string, args ...any)) []RegexRule {
	var valid []map[string]any
	for _, r := range rows {
		if pat, ok := r["regex"].(string); ok && strings.TrimSpace(pat) != "" {
			valid = append(valid, r)
		}
	}
	backendCount, historyCount, unknownScope := 0, 0, 0
	var kept []map[string]any
	for _, r := range valid {
		mode := riliStr(r["applyMode"])
		if mode == "backend" {
			backendCount++
			continue
		}
		kept = append(kept, r)
		if mode == "all" {
			historyCount++
		}
		if _, hasRole := r["messageRole"]; hasRole && r["messageRole"] != nil {
			if _, known := riliRoleTargets[riliStr(r["messageRole"])]; !known {
				unknownScope++
			}
		}
	}
	if backendCount > 0 {
		add("regexBackend", "%d backend-only regex rules were skipped because display rules here change only what is shown", backendCount)
	}
	if historyCount > 0 {
		add("regexHistory", "%d regex rules rewrite chat history on RiliAI; display rules here change only what is shown", historyCount)
	}
	if unknownScope > 0 {
		add("regexScope", "%d regex rules had an unrecognised messageRole; they now apply to every message", unknownScope)
	}

	rules := make([]RegexRule, 0, len(kept))
	for i, r := range kept {
		role := riliStr(r["messageRole"])
		if role == "" {
			role = "assistant"
		}
		name := riliStr(r["name"])
		if name == "" {
			name = fmt.Sprintf("#%d", i+1)
		}
		pat, _ := r["regex"].(string)
		rep, _ := r["replaceString"].(string)
		rule := RegexRule{
			Name:    truncateRunes(name, 40),
			Find:    RiliRegexFind(pat),
			Replace: rep,
			Enabled: r["enabled"] != false,
		}
		if t, ok := riliRoleTargets[role]; ok {
			targets := append([]string(nil), t...)
			rule.Targets = &targets
		}
		rules = append(rules, rule)
	}
	return rules
}

func isRiliIdentByte(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '$'
}

func rewriteRiliHandlebarsTag(tag string) string {
	const needle = "workingMemory"
	var out strings.Builder
	i := 0
	for i < len(tag) {
		idx := strings.Index(tag[i:], needle)
		if idx < 0 {
			out.WriteString(tag[i:])
			break
		}
		idx += i
		out.WriteString(tag[i:idx])
		var prev byte
		hasPrev := idx > 0
		if hasPrev {
			prev = tag[idx-1]
		}
		afterIdx := idx + len(needle)
		var next byte
		hasNext := afterIdx < len(tag)
		if hasNext {
			next = tag[afterIdx]
		}
		if (!hasPrev || (!isRiliIdentByte(prev) && prev != '.')) && (!hasNext || !isRiliIdentByte(next)) {
			if hasNext && next == '.' && afterIdx+1 < len(tag) {
				c := tag[afterIdx+1]
				if c != ' ' && c != '\t' && c != '\r' && c != '\n' && c != '}' {
					i = afterIdx + 1
					continue
				}
			}
			out.WriteByte('.')
			i = afterIdx
			continue
		}
		out.WriteString(needle)
		i = afterIdx
	}
	return out.String()
}

// RewriteRiliRenderTemplate rewrites {{workingMemory.xxx}} to {{xxx}} and {{#with workingMemory}} to {{#with .}}.
func RewriteRiliRenderTemplate(content string) string {
	return riliHbsTagRe.ReplaceAllStringFunc(content, rewriteRiliHandlebarsTag)
}

func riliVarKeyPath(p, key string) string {
	if key == "" || strings.ContainsAny(key, `.[]"`) {
		return p + `["` + key + `"]`
	}
	if p == "" {
		return key
	}
	return p + "." + key
}

func riliNodeStr(n *card.JSONNode, key string) string {
	if v, ok := n.Get(key); ok && v.Kind == card.JSONString {
		return strings.TrimSpace(v.Str)
	}
	return ""
}

func riliNodeNum(n *card.JSONNode, key string) (float64, bool) {
	v, ok := n.Get(key)
	if !ok || v.Kind != card.JSONNumber {
		return 0, false
	}
	f, err := strconv.ParseFloat(v.Num, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func inferRiliSchemaType(node *card.JSONNode) string {
	if t := riliNodeStr(node, "type"); t != "" {
		return t
	}
	if props, ok := node.Get("properties"); ok && props.Kind == card.JSONObject {
		return "object"
	}
	if _, ok := node.Get("items"); ok {
		return "array"
	}
	if enumNode, ok := node.Get("enum"); ok && enumNode.Kind == card.JSONArray && len(enumNode.Arr) > 0 {
		first := enumNode.Arr[0]
		switch first.Kind {
		case card.JSONBool:
			return "boolean"
		case card.JSONNumber:
			if f, err := strconv.ParseFloat(first.Num, 64); err == nil && f == math.Trunc(f) {
				return "integer"
			}
			return "number"
		}
	}
	return "string"
}

func formatRiliSchemaNodeRule(node *card.JSONNode) string {
	t := inferRiliSchemaType(node)
	desc := riliNodeStr(node, "description")
	var constraints []string
	if t == "integer" {
		constraints = append(constraints, "integer")
	}
	if minV, ok := riliNodeNum(node, "minimum"); ok {
		constraints = append(constraints, "min: "+card.FormatJSONNumber(minV))
	}
	if maxV, ok := riliNodeNum(node, "maximum"); ok {
		constraints = append(constraints, "max: "+card.FormatJSONNumber(maxV))
	}
	if enumNode, ok := node.Get("enum"); ok && enumNode.Kind == card.JSONArray && len(enumNode.Arr) > 0 {
		parts := make([]string, 0, len(enumNode.Arr))
		for _, v := range enumNode.Arr {
			parts = append(parts, v.Compact())
		}
		constraints = append(constraints, "enum: "+strings.Join(parts, " | "))
	}
	base := desc
	if len(constraints) > 0 {
		joined := strings.Join(constraints, ", ")
		if desc != "" {
			base = desc + " (" + joined + ")"
		} else {
			base = joined
		}
	}
	if t == "array" {
		if items, ok := node.Get("items"); ok && items.Kind == card.JSONObject {
			itemRule := formatRiliSchemaNodeRule(items)
			if props, ok := items.Get("properties"); ok && props.Kind == card.JSONObject {
				var propRules []string
				for i, k := range props.Keys {
					child := props.Vals[i]
					if child.Kind != card.JSONObject {
						continue
					}
					if r := formatRiliSchemaNodeRule(child); r != "" {
						propRules = append(propRules, k+": "+r)
					} else {
						propRules = append(propRules, k)
					}
				}
				if len(propRules) > 0 {
					summary := "{" + strings.Join(propRules, "; ") + "}"
					if itemRule != "" {
						itemRule = itemRule + " " + summary
					} else {
						itemRule = summary
					}
				}
			}
			if itemRule != "" {
				if base != "" {
					base = base + "; items: " + itemRule
				} else {
					base = "items: " + itemRule
				}
			}
		}
	}
	return base
}

func buildRiliSchemaTree(node *card.JSONNode, path string, pathRules map[string]string, pathOrder *[]string) *card.JSONNode {
	t := inferRiliSchemaType(node)
	props, hasProps := node.Get("properties")
	hasProps = t == "object" && hasProps && props.Kind == card.JSONObject

	if path != "" {
		if rule := formatRiliSchemaNodeRule(node); rule != "" {
			if _, seen := pathRules[path]; !seen {
				*pathOrder = append(*pathOrder, path)
			}
			pathRules[path] = rule
		}
	}

	if hasProps {
		out := card.NewJSONObject()
		for i, k := range props.Keys {
			child := props.Vals[i]
			cp := riliVarKeyPath(path, k)
			if child.Kind == card.JSONObject {
				out.Set(k, buildRiliSchemaTree(child, cp, pathRules, pathOrder))
			} else {
				out.Set(k, card.NewJSONString(""))
			}
		}
		if defNode, ok := node.Get("default"); ok && defNode.Kind == card.JSONObject {
			for i, k := range defNode.Keys {
				if _, exists := out.Get(k); exists {
					out.Set(k, defNode.Vals[i].Clone())
				}
			}
		}
		return out
	}

	if defNode, ok := node.Get("default"); ok {
		return defNode.Clone()
	}
	if enumNode, ok := node.Get("enum"); ok && enumNode.Kind == card.JSONArray && len(enumNode.Arr) > 0 {
		return enumNode.Arr[0].Clone()
	}
	switch t {
	case "object":
		return card.NewJSONObject()
	case "array":
		return &card.JSONNode{Kind: card.JSONArray, Arr: []*card.JSONNode{}}
	case "integer", "number":
		if minV, ok := riliNodeNum(node, "minimum"); ok && minV > 0 {
			return &card.JSONNode{Kind: card.JSONNumber, Num: card.FormatJSONNumber(minV)}
		}
		if maxV, ok := riliNodeNum(node, "maximum"); ok && maxV < 0 {
			return &card.JSONNode{Kind: card.JSONNumber, Num: card.FormatJSONNumber(maxV)}
		}
		return &card.JSONNode{Kind: card.JSONNumber, Num: "0"}
	case "boolean":
		return &card.JSONNode{Kind: card.JSONBool, Bool: false}
	default:
		return card.NewJSONString("")
	}
}

func extractOrderedMemorySchemaNode(rawBytes []byte, isMod bool) *card.JSONNode {
	if len(rawBytes) == 0 {
		return nil
	}
	root, err := card.ParseJSON(bytes.TrimPrefix(rawBytes, []byte("\xef\xbb\xbf")))
	if err != nil || root.Kind != card.JSONObject {
		return nil
	}
	source := root
	if isMod {
		if p, ok := root.Get("payload"); ok && p.Kind == card.JSONObject {
			source = p
		}
	}
	ms, ok := source.Get("memorySchema")
	if !ok {
		return nil
	}
	if ms.Kind == card.JSONObject {
		return ms
	}
	return nil
}

func convertRiliVariables(source map[string]any, rawBytes []byte, isMod bool, add func(code, format string, args ...any)) *card.Variables {
	enabled := source["enableMemorySchema"] == true
	rawSchema := ""
	var schemaNode *card.JSONNode
	switch v := source["memorySchema"].(type) {
	case string:
		rawSchema = strings.TrimSpace(v)
	case map[string]any:
		schemaNode = extractOrderedMemorySchemaNode(rawBytes, isMod)
		if schemaNode == nil {
			schemaNode = card.JSONFromAny(v)
		}
		rawSchema = card.FormatJSON(schemaNode, "")
	}

	rt, _ := source["renderTemplate"].(map[string]any)
	rawTemplate := ""
	injectSuggestions := false
	if rt != nil {
		if s, ok := rt["content"].(string); ok {
			rawTemplate = strings.TrimSpace(s)
		}
		injectSuggestions = rt["injectSuggestions"] == true
	}
	template := ""
	if rawTemplate != "" {
		template = RewriteRiliRenderTemplate(rawTemplate)
	}
	usesSuggestions := injectSuggestions || riliSuggestRe.MatchString(rawTemplate)

	if !enabled {
		if rawSchema != "" {
			add("memoryDisabled", "memorySchema was present in the file, but enableMemorySchema is false, so card variables stayed off")
		}
		if rawTemplate != "" {
			add("variablesTemplateOnly", "renderTemplate was present without an enabled memorySchema, so card variables stayed off")
		}
		return nil
	}
	if rawSchema == "" {
		if rawTemplate != "" {
			add("variablesTemplateOnly", "renderTemplate was present without a memorySchema, so card variables stayed off")
		}
		return nil
	}

	if schemaNode == nil {
		n, err := card.ParseJSON([]byte(rawSchema))
		if err != nil {
			text, _ := json.Marshal(rawSchema)
			add("memoryInvalid", "memorySchema is not valid JSON; it was kept as text in %s and must be fixed before pushing", card.VariablesFile)
			if template != "" && usesSuggestions {
				add("templateSuggestions", "renderTemplate references suggestions, which are empty here; review %s before pushing", card.PanelFile)
			}
			return &card.Variables{Convention: "plain", Data: text, Panel: template}
		}
		schemaNode = n
	}

	invalidRoot := schemaNode.Kind != card.JSONObject
	if !invalidRoot {
		if tNode, hasType := schemaNode.Get("type"); hasType {
			invalidRoot = tNode.Kind != card.JSONString || strings.TrimSpace(tNode.Str) != "object"
		}
	}
	if invalidRoot {
		text, _ := json.Marshal(rawSchema)
		add("memoryInvalid", "memorySchema root must be a JSON object schema; it was kept as text in %s and must be fixed before pushing", card.VariablesFile)
		if template != "" && usesSuggestions {
			add("templateSuggestions", "renderTemplate references suggestions, which are empty here; review %s before pushing", card.PanelFile)
		}
		return &card.Variables{Convention: "plain", Data: text, Panel: template}
	}

	pathRules := map[string]string{}
	var pathOrder []string
	dataNode := buildRiliSchemaTree(schemaNode, "", pathRules, &pathOrder)
	if dataNode.Kind != card.JSONObject {
		text, _ := json.Marshal(rawSchema)
		add("memoryInvalid", "memorySchema root must be a JSON object schema; it was kept as text in %s and must be fixed before pushing", card.VariablesFile)
		if template != "" && usesSuggestions {
			add("templateSuggestions", "renderTemplate references suggestions, which are empty here; review %s before pushing", card.PanelFile)
		}
		return &card.Variables{Convention: "plain", Data: text, Panel: template}
	}

	rulesText := riliNodeStr(schemaNode, "description")
	pathBytes := 0
	for _, k := range pathOrder {
		if v := strings.TrimSpace(pathRules[k]); v != "" {
			pathBytes += len(k) + len(v)
		}
	}
	if pathBytes > card.VariablesPathRuleMax {
		var lines []string
		for _, k := range pathOrder {
			if v := strings.TrimSpace(pathRules[k]); v != "" {
				lines = append(lines, k+": "+v)
			}
		}
		if joined := strings.Join(lines, "\n"); joined != "" {
			if rulesText != "" {
				rulesText = rulesText + "\n\n" + joined
			} else {
				rulesText = joined
			}
		}
		pathRules = nil
	}

	v := &card.Variables{
		Convention: "plain",
		Data:       json.RawMessage(dataNode.Compact()),
		Rules:      rulesText,
		Panel:      template,
	}
	if len(pathRules) > 0 {
		v.PathRules = pathRules
	}
	if problems := card.CheckVariables(v, nil); len(problems) > 0 {
		add("memoryLimit", "working memory became card variables, but exceeds limits (%s); adjust before pushing", problems[0])
	} else {
		add("memoryConverted", "working memory schema became card variables (%s); review starting values before pushing", card.VariablesFile)
	}
	if template != "" && usesSuggestions {
		add("templateSuggestions", "renderTemplate references suggestions, which are empty here; review %s before pushing", card.PanelFile)
	}
	return v
}

func riliHasNonEmptyContent(v any) bool {
	switch t := v.(type) {
	case []any:
		return len(t) > 0
	case map[string]any:
		for _, val := range t {
			if val == nil || val == "" || val == false {
				continue
			}
			if arr, ok := val.([]any); ok && len(arr) == 0 {
				continue
			}
			return true
		}
		return false
	case string:
		s := strings.TrimSpace(t)
		return fengyueURL.MatchString(s) || utf8.RuneCountInString(s) > 30 || strings.Contains(t, "\n")
	}
	return false
}

func riliHasModelSettings(raw map[string]any) bool {
	if riliStr(raw["model"]) != "" {
		return true
	}
	if f, ok := fyNum(raw["contextWindow"]); ok && f > 0 {
		return true
	}
	if ms, ok := raw["modelSettings"].(map[string]any); ok && len(ms) > 0 {
		return true
	}
	if d, ok := raw["defaultModelSettings"].(map[string]any); ok && d != nil {
		if riliStr(d["model"]) != "" {
			return true
		}
		if f, ok := fyNum(d["contextWindow"]); ok && f > 0 {
			return true
		}
		if ms, ok := d["modelSettings"].(map[string]any); ok && len(ms) > 0 {
			return true
		}
	}
	return false
}

func riliReportUnmapped(raw, source, prompt map[string]any, isMod bool, add func(code, format string, args ...any)) {
	if isMod {
		add("modFile", "imported RiliAI module file (rili-mod); add a card name and welcome/opening page before pushing")
	}
	cover := riliStr(source["cover"])
	bg := riliStr(source["background"])
	if cover != "" || bg != "" {
		var urls []string
		if cover != "" {
			urls = append(urls, "https://public.talkaicc.com/cover/"+cover)
		}
		if bg != "" {
			urls = append(urls, "https://public.talkaicc.com/background/"+bg)
		}
		add("images", "cover/background carry RiliAI object keys only (%s); download and add them to assets/ before pushing", strings.Join(urls, ", "))
	}
	if opts, ok := source["options"].(map[string]any); ok {
		if n := len(riliObjects(opts["items"])); n > 0 {
			add("presetOptions", "%d quick-input buttons (options.items) have no destination", n)
		}
	}
	if riliStr(prompt["suggestionsPrompt"]) != "" {
		add("suggestionsPrompt", "suggestionsPrompt was not imported; per-turn reply suggestions are not generated here")
	}
	if n := len(riliObjects(source["cardsVoices"])); n > 0 {
		add("voices", "%d voice configurations (cardsVoices) were not imported", n)
	}
	if riliHasModelSettings(source) {
		add("modelSettings", "default model and sampling settings were not imported; players choose their model in chat")
	}
	if source["enableSafeguard"] == true {
		add("safeguard", "enableSafeguard was not imported; author prompts are protected automatically by the platform")
	}
	if riliStr(source["assistantPrefill"]) != "" {
		add("assistantPrefill", "assistantPrefill was not imported; put any required reply prefix in output_contract.md")
	}

	other := 0
	for k, v := range raw {
		if !riliKnownTopKeys[k] && riliHasNonEmptyContent(v) {
			other++
		}
	}
	if isMod {
		for k, v := range source {
			if !riliKnownTopKeys[k] && riliHasNonEmptyContent(v) {
				other++
			}
		}
	}
	for k, v := range prompt {
		if !riliKnownPromptKeys[k] && riliHasNonEmptyContent(v) {
			other++
		}
	}
	if other > 0 {
		add("otherFields", "%d unrecognised RiliAI fields have no destination and were not imported", other)
	}
}

// FromRili converts a decoded Rili card/settings/mod export into a Result.
func FromRili(raw map[string]any, rawBytes []byte, language string) *Result {
	r, notes := convertRili(raw, rawBytes, language)
	for _, n := range notes {
		r.Notes = append(r.Notes, n.text)
	}
	return r
}

func convertRili(raw map[string]any, rawBytes []byte, language string) (*Result, []riliNote) {
	var notes []riliNote
	add := func(code, format string, args ...any) {
		notes = append(notes, riliNote{code: code, text: fmt.Sprintf(format, args...)})
	}
	r := &Result{Spec: "rili", Language: language}

	isMod := riliStr(raw["format"]) == "rili-mod"
	source := raw
	if isMod {
		if p, ok := raw["payload"].(map[string]any); ok && p != nil {
			source = p
		}
	}
	prompt, _ := source["prompt"].(map[string]any)
	if prompt == nil {
		prompt = map[string]any{}
	}

	r.Name = riliStr(source["title"])
	if r.Name == "" {
		r.Name = riliStr(raw["title"])
	}
	r.Summary = riliStr(source["description"])
	if r.Summary == "" {
		r.Summary = riliStr(raw["description"])
	}
	r.Definition = riliStr(prompt["systemPrompt"])
	r.OutputContract = riliStr(prompt["headPrompt"])
	r.Custom = riliStr(prompt["rearPrompt"])

	var rawTags []string
	for _, item := range riliObjects(source["cardsTags"]) {
		name := ""
		if tagObj, ok := item["tag"].(map[string]any); ok {
			name = riliStr(tagObj["name"])
		} else {
			name = riliStr(item["name"])
		}
		if name != "" {
			rawTags = append(rawTags, name)
		}
	}
	if len(rawTags) > TagsMax {
		add("tags", "%d tags beyond the limit of %d were dropped", len(rawTags)-TagsMax, TagsMax)
		rawTags = rawTags[:TagsMax]
	}
	r.Tags = rawTags

	for _, f := range []struct{ label, value, limitKey string }{
		{"name", r.Name, "name"},
		{"summary", r.Summary, "summary"},
		{"definition", r.Definition, "definition"},
		{"outputContract", r.OutputContract, "outputContract"},
		{"custom", r.Custom, "customInstructions"},
	} {
		if n, cap := utf8.RuneCountInString(f.value), fengyueFieldLimit(language, f.limitKey); cap > 0 && n > cap {
			add("tooLong."+f.label, "%s is %d characters, over the limit of %d; shorten it before pushing", f.label, n, cap)
		}
	}

	if intro, ok := source["introPage"].(string); ok {
		r.OpeningPage = strings.TrimSpace(intro)
	}
	if r.OpeningPage == "" {
		add("noWelcome", "both introPage and welcome are empty; add welcome.md or opening.html before pushing")
	}

	if bookRows := riliObjects(prompt["lorebook"]); len(bookRows) > 0 {
		entries := convertRiliLorebookEntries(bookRows)
		drafts := BookEntriesToDrafts(entries, language)
		r.Lorebook = &LorebookDraft{Name: r.Name, Entries: drafts}
		notes = append(notes, riliBookNotes(entries)...)
		limit := EntryContentMaxFor(language)
		split := 0
		for _, e := range entries {
			dec := ParseDecorators(strings.TrimSpace(e.Content))
			if utf8.RuneCountInString(strings.TrimSpace(dec.Content)) > limit {
				split++
			}
		}
		if split > 0 {
			add("split", "%d Lorebook entries were longer than %d characters and were split into several entries with the same keywords", split, limit)
		}
	}

	rules := convertRiliRegex(riliObjects(source["assistantRegexReplace"]), add)
	css := ""
	if s, ok := riliRawStr(source["injectStyles"]); ok {
		css = s
	} else if s, ok := riliRawStr(raw["injectedStyles"]); ok {
		css = s
	} else if s, ok := riliRawStr(source["injectedStyles"]); ok {
		css = s
	}
	if css != "" {
		r.CardCSS, r.CardCSSDialect = css, "rili"
	}
	r.Rules = &RuleSet{Rules: rules, PageMode: "sandbox", Format: "fengyue"}

	r.Variables = convertRiliVariables(source, rawBytes, isMod, add)
	riliReportUnmapped(raw, source, prompt, isMod, add)
	return r, notes
}
