package card

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Card variables (the provider's "state" section): values the story keeps exact
// from turn to turn, such as affection, money or an inventory.
//
// A folder keeps them in three files, the same split as the rest of the card:
// structured data in JSON, prose for the model in Markdown, HTML in its own file.
//
//	variables.json  {"convention": "mvu"|"plain", "data": {...}, "pathRules"?: {...}}
//	variables.md    optional rules for the model (rules.text)
//	panel.hbs       optional panel skin, Handlebars HTML (panel.template)
//
// The provider normalizes once on write; this file mirrors that normalization
// (provider internal/service/cardstate/definition.go, and the web editor's
// card-state.ts) for local checks and for rebuilding the author's form on pull.
// When they disagree the provider is right; change this copy to match.
const (
	VariablesFile     = "variables.json"
	VariableRulesFile = "variables.md"
	PanelFile         = "panel.hbs"
)

// Variable limits, in UTF-8 bytes (provider cardstate.Max*).
const (
	VariablesDataMax     = 16 << 10
	VariablesDepthMax    = 8
	VariablesKeysMax     = 400
	VariablesStringMax   = 2 << 10
	VariablesRulesMax    = 8 << 10
	VariablesPathRuleMax = 8 << 10
	VariablesPanelMax    = 32 << 10
)

// Variables is variables.json with its two companion files.
type Variables struct {
	// Convention is "mvu" (leaves written as [value, "rule"], $meta opens objects)
	// or "plain" (bare values; an empty {} may gain keys). Empty means plain.
	Convention string `json:"convention"`
	// Data is the skeleton as written, key order intact.
	Data json.RawMessage `json:"data"`
	// PathRules attaches rules to values by path where the skeleton cannot carry
	// them (a value inside a list, or any value on a plain card).
	PathRules map[string]string `json:"pathRules,omitempty"`
	// Rules is variables.md; Panel is panel.hbs. Empty when the file is absent.
	Rules string `json:"-"`
	Panel string `json:"-"`
	// Empty marks a variables.json with no content: push removes the card's variables.
	Empty bool `json:"-"`
}

// LoadVariables reads variables.json and its companion files; nil when absent.
func LoadVariables(dir string) (*Variables, error) {
	b, err := os.ReadFile(filepath.Join(dir, VariablesFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return &Variables{Empty: true}, nil
	}
	var v Variables
	if err := json.Unmarshal(b, &v); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			return nil, fmt.Errorf("%s: must be an object with convention and data (%v)", VariablesFile, err)
		}
		return nil, fmt.Errorf("%s: %s", VariablesFile, jsonErrorAt(b, err))
	}
	if v.Rules, err = readText(filepath.Join(dir, VariableRulesFile)); err != nil {
		return nil, err
	}
	if v.Panel, err = readText(filepath.Join(dir, PanelFile)); err != nil {
		return nil, err
	}
	return &v, nil
}

// jsonErrorAt names the line of a syntax error, so the author can find it.
func jsonErrorAt(b []byte, err error) string {
	var se *json.SyntaxError
	if errors.As(err, &se) {
		line := 1 + bytes.Count(b[:min(int(se.Offset), len(b))], []byte("\n"))
		return fmt.Sprintf("not valid JSON near line %d: %v", line, err)
	}
	return "not valid JSON: " + err.Error()
}

func (v *Variables) save(dir string) error {
	if v.Empty {
		return os.WriteFile(filepath.Join(dir, VariablesFile), nil, 0o644)
	}
	conv := v.Convention
	if conv == "" {
		conv = "plain"
	}
	var b strings.Builder
	b.WriteString("{\n  \"convention\": " + jsonString(conv) + ",\n  \"data\": ")
	if n, err := ParseJSON(v.Data); err == nil {
		b.WriteString(FormatJSON(n, "  "))
	} else if len(bytes.TrimSpace(v.Data)) > 0 {
		b.Write(bytes.TrimSpace(v.Data))
	} else {
		b.WriteString("{}")
	}
	if len(v.PathRules) > 0 {
		keys := make([]string, 0, len(v.PathRules))
		for k := range v.PathRules {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString(",\n  \"pathRules\": {")
		for i, k := range keys {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("\n    " + jsonString(k) + ": " + jsonString(v.PathRules[k]))
		}
		b.WriteString("\n  }")
	}
	b.WriteString("\n}\n")
	if err := os.WriteFile(filepath.Join(dir, VariablesFile), []byte(b.String()), 0o644); err != nil {
		return err
	}
	if v.Rules != "" {
		if err := writeText(filepath.Join(dir, VariableRulesFile), v.Rules); err != nil {
			return err
		}
	}
	if v.Panel != "" {
		if err := writeText(filepath.Join(dir, PanelFile), v.Panel); err != nil {
			return err
		}
	}
	return nil
}

// Body is the wire section {convention, data, rules?, panel?}; urls rewrites
// asset references in the rules and the panel. ok is false when data is not
// JSON at all (Load already rejects that, so only hand-built values reach it).
func (v *Variables) Body(urls map[string]string) (map[string]any, bool) {
	conv := v.Convention
	if conv == "" {
		conv = "plain"
	}
	data := json.RawMessage(bytes.TrimSpace(v.Data))
	if n, err := ParseJSON(v.Data); err == nil {
		data = json.RawMessage(n.Compact())
	} else if len(data) == 0 || !json.Valid(data) {
		return nil, false
	}
	out := map[string]any{"convention": conv, "data": data}
	rules := map[string]any{}
	if text := Body(v.Rules); text != "" {
		rules["text"] = Rewrite(text, urls)
	}
	if len(v.PathRules) > 0 {
		rules["paths"] = v.PathRules
	}
	if len(rules) > 0 {
		out["rules"] = rules
	}
	if t := strings.TrimSpace(Rewrite(v.Panel, urls)); t != "" {
		out["panel"] = map[string]any{"template": t}
	}
	return out, true
}

// NormalizedVariables is what the provider stores: plain values, the rule of
// each value by path, the objects that may gain keys and the keys that may not go.
type NormalizedVariables struct {
	Data     *JSONNode
	Paths    map[string]string
	Open     []string
	Required []string
}

const mvuExtensibleMarker = "$__META_EXTENSIBLE__$"

// variableKeyPath matches the provider's Path.String: keys joined with ".", and a
// key that would read as path syntax (empty, or holding . [ ] ") written as
// ["key"] with no dot before it, e.g. a["x.y"], ["HP.max"]. Open, required and
// rule paths come back from the provider in that form, so lookups must build it
// exactly the same way.
func variableKeyPath(p, key string) string {
	if key == "" || strings.ContainsAny(key, `.[]"`) {
		return p + `["` + key + `"]`
	}
	if p == "" {
		return key
	}
	return p + "." + key
}

func variableIndexPath(p string, i int) string { return p + "[" + strconv.Itoa(i) + "]" }

func isMarker(n *JSONNode) bool { return n.Kind == JSONString && n.Str == mvuExtensibleMarker }

// isPair: a two-item list whose second item is text is an MVU [value, "rule"] pair.
func isPair(n *JSONNode) bool {
	return n.Kind == JSONArray && len(n.Arr) == 2 && n.Arr[1].Kind == JSONString && !isMarker(n.Arr[0]) && !isMarker(n.Arr[1])
}

// NormalizeVariables applies the provider's write-time normalization. It fails
// with "convention" or "data" (data not an object).
func NormalizeVariables(convention string, data *JSONNode, pathRules map[string]string) (*NormalizedVariables, string) {
	if data == nil || data.Kind != JSONObject {
		return nil, "data"
	}
	out := &NormalizedVariables{Paths: map[string]string{}}
	for k, v := range pathRules {
		if v = strings.TrimSpace(v); v != "" {
			out.Paths[k] = v
		}
	}
	switch convention {
	case "plain", "":
		out.Data = data.Clone()
		var walk func(n *JSONNode, p string)
		walk = func(n *JSONNode, p string) {
			switch n.Kind {
			case JSONObject:
				if p != "" && len(n.Keys) == 0 {
					out.Open = append(out.Open, p)
				}
				for i, k := range n.Keys {
					walk(n.Vals[i], variableKeyPath(p, k))
				}
			case JSONArray:
				for i, v := range n.Arr {
					walk(v, variableIndexPath(p, i))
				}
			}
		}
		walk(data, "")
	case "mvu":
		var node, value func(n *JSONNode, p string, inherited bool) *JSONNode
		var list func(n *JSONNode, p string, inherited bool) *JSONNode
		node = func(n *JSONNode, p string, inherited bool) *JSONNode {
			switch n.Kind {
			case JSONArray:
				if isPair(n) {
					if rule := strings.TrimSpace(n.Arr[1].Str); p != "" && rule != "" {
						out.Paths[p] = rule
					}
					return value(n.Arr[0], p, inherited)
				}
				return list(n, p, inherited)
			case JSONObject:
				obj := NewJSONObject()
				open, recursive := inherited, inherited
				if meta, ok := n.Get("$meta"); ok && meta.Kind == JSONObject {
					if v, ok := meta.Get("extensible"); ok && v.Kind == JSONBool {
						open = v.Bool
					}
					if v, ok := meta.Get("recursiveExtensible"); ok && v.Kind == JSONBool && v.Bool {
						open, recursive = true, true
					}
					if req, ok := meta.Get("required"); ok && req.Kind == JSONArray {
						for _, r := range req.Arr {
							if r.Kind == JSONString {
								out.Required = append(out.Required, variableKeyPath(p, r.Str))
							}
						}
					}
				}
				for i, k := range n.Keys {
					if k == "$meta" {
						continue
					}
					obj.Set(k, node(n.Vals[i], variableKeyPath(p, k), recursive))
				}
				if open && p != "" {
					out.Open = append(out.Open, p)
				}
				return obj
			}
			return n.Clone()
		}
		value = func(n *JSONNode, p string, inherited bool) *JSONNode {
			if n.Kind == JSONArray {
				return list(n, p, inherited)
			}
			return node(n, p, inherited)
		}
		list = func(n *JSONNode, p string, inherited bool) *JSONNode {
			arr := &JSONNode{Kind: JSONArray, Arr: []*JSONNode{}}
			for _, v := range n.Arr {
				// The index is the item's place after markers are dropped, as the provider counts it.
				if !isMarker(v) {
					arr.Arr = append(arr.Arr, value(v, variableIndexPath(p, len(arr.Arr)), inherited))
				}
			}
			return arr
		}
		out.Data = node(data, "", false)
	default:
		return nil, "convention"
	}
	out.Open = sortDedupe(out.Open)
	out.Required = sortDedupe(out.Required)
	return out, ""
}

func sortDedupe(s []string) []string {
	sort.Strings(s)
	var out []string
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

// RemoteVariables is the author view of a card's variables in the role detail.
type RemoteVariables struct {
	Data  json.RawMessage `json:"data"`
	Panel *struct {
		Template string `json:"template"`
	} `json:"panel"`
	Rules struct {
		Text  string            `json:"text"`
		Paths map[string]string `json:"paths"`
	} `json:"rules"`
	Open     []string `json:"open"`
	Required []string `json:"required"`
}

// RebuildVariables turns the provider's normalized form back into what the
// author writes, so pushing the pulled folder stores the same thing again.
// Without value rules or required keys, and with only empty objects open, it is
// plain. Otherwise it is MVU: a value with a rule becomes [value, "rule"], an
// open object gets {"$meta": {"extensible": true}}, required keys go in the
// parent's $meta.required, and a two-item list ending in text is wrapped as
// [list, ""] so it stays a list. Rules MVU cannot carry (on a list item) are
// kept in pathRules.
func RebuildVariables(r RemoteVariables) (*Variables, error) {
	data, err := ParseJSON(r.Data)
	if err != nil {
		// The provider may hand the stored text back as a JSON string.
		var text string
		if json.Unmarshal(r.Data, &text) != nil {
			return nil, err
		}
		if data, err = ParseJSON([]byte(text)); err != nil {
			return nil, err
		}
	}
	if data.Kind != JSONObject {
		return nil, errors.New("variables are not an object")
	}
	v := &Variables{Rules: r.Rules.Text}
	if r.Panel != nil {
		v.Panel = r.Panel.Template
	}
	paths := map[string]string{}
	for k, rule := range r.Rules.Paths {
		paths[k] = rule
	}
	open := map[string]bool{}
	for _, p := range r.Open {
		open[p] = true
	}
	plain, _ := NormalizeVariables("plain", data, nil)
	if len(paths) == 0 && len(r.Required) == 0 && sameStrings(plain.Open, sortDedupe(append([]string(nil), r.Open...))) {
		v.Convention = "plain"
		v.Data = json.RawMessage(data.Compact())
		return v, nil
	}
	// Each required path names a key under its parent object: group the keys by
	// the parent's path so the parent's $meta.required can list them.
	requiredByParent := map[string][]string{}
	for _, req := range r.Required {
		parts, ok := parseVariablePath(req)
		if !ok || parts[len(parts)-1].isIdx {
			continue
		}
		parent := formatVariablePath(parts[:len(parts)-1])
		requiredByParent[parent] = append(requiredByParent[parent], parts[len(parts)-1].key)
	}
	requiredUnder := func(p string) []string { return requiredByParent[p] }
	var build func(n *JSONNode, p string) *JSONNode
	build = func(n *JSONNode, p string) *JSONNode {
		switch n.Kind {
		case JSONArray:
			arr := &JSONNode{Kind: JSONArray, Arr: []*JSONNode{}}
			for i, item := range n.Arr {
				arr.Arr = append(arr.Arr, build(item, variableIndexPath(p, i)))
			}
			return arr
		case JSONObject:
			obj := NewJSONObject()
			meta := NewJSONObject()
			if p != "" && open[p] {
				meta.Set("extensible", &JSONNode{Kind: JSONBool, Bool: true})
			}
			if req := requiredUnder(p); len(req) > 0 {
				list := &JSONNode{Kind: JSONArray}
				for _, k := range req {
					list.Arr = append(list.Arr, NewJSONString(k))
				}
				meta.Set("required", list)
			}
			if len(meta.Keys) > 0 {
				obj.Set("$meta", meta)
			}
			for i, k := range n.Keys {
				cp := variableKeyPath(p, k)
				child := build(n.Vals[i], cp)
				if rule, ok := paths[cp]; ok {
					delete(paths, cp)
					child = &JSONNode{Kind: JSONArray, Arr: []*JSONNode{child, NewJSONString(rule)}}
				} else if isPair(child) {
					child = &JSONNode{Kind: JSONArray, Arr: []*JSONNode{child, NewJSONString("")}}
				}
				obj.Set(k, child)
			}
			return obj
		}
		return n.Clone()
	}
	v.Convention = "mvu"
	v.Data = json.RawMessage(build(data, "").Compact())
	if len(paths) > 0 {
		v.PathRules = paths
	}
	return v, nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// CheckVariables runs the provider's limits locally. The section body is the
// wire body (after asset rewriting), so the sizes are what will be sent.
func CheckVariables(v *Variables, body map[string]any) []string {
	if v == nil || v.Empty {
		return nil
	}
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	switch v.Convention {
	case "", "mvu", "plain":
	default:
		add("%s: convention %q is not supported. Use \"mvu\" (values written as [value, \"rule\"]) or \"plain\" (values only).", VariablesFile, v.Convention)
		return problems
	}
	data, err := ParseJSON(v.Data)
	if err != nil || data.Kind != JSONObject {
		hint := ""
		if err == nil && data.Kind == JSONString {
			hint = " It holds text instead; fix the JSON inside it and write it as an object."
		}
		add("%s: data must be a JSON object holding the starting values.%s", VariablesFile, hint)
		return problems
	}
	norm, reason := NormalizeVariables(v.Convention, data, v.PathRules)
	if reason != "" {
		add("%s: data must be a JSON object holding the starting values.", VariablesFile)
		return problems
	}
	var size bytes.Buffer
	norm.Data.write(&size, true)
	if n := size.Len(); n > VariablesDataMax {
		add("%s: the values take %d bytes; the limit is %d bytes (%d over). Remove values you do not need.", VariablesFile, n, VariablesDataMax, n-VariablesDataMax)
	}
	keys := 0
	reported := map[string]bool{}
	var walk func(n *JSONNode, p string, depth int)
	walk = func(n *JSONNode, p string, depth int) {
		if depth > VariablesDepthMax {
			if !reported["depth"] {
				reported["depth"] = true
				add("%s: %s is nested %d levels deep; the limit is %d. Flatten the deepest parts.", VariablesFile, quotePath(p), depth, VariablesDepthMax)
			}
			return
		}
		switch n.Kind {
		case JSONString:
			if len(n.Str) > VariablesStringMax || !utf8.ValidString(n.Str) {
				add("%s: the text at %s is %d bytes; the limit is %d bytes. Shorten it.", VariablesFile, quotePath(p), len(n.Str), VariablesStringMax)
			}
		case JSONArray:
			for i, item := range n.Arr {
				walk(item, variableIndexPath(p, i), depth+1)
			}
		case JSONObject:
			keys += len(n.Keys)
			for i, k := range n.Keys {
				walk(n.Vals[i], variableKeyPath(p, k), depth+1)
			}
		}
	}
	walk(norm.Data, "", 0)
	if keys > VariablesKeysMax {
		add("%s: the values hold %d keys; the limit is %d. Remove the ones you do not need.", VariablesFile, keys, VariablesKeysMax)
	}
	rulesText, panel := "", ""
	if body != nil {
		if r, ok := body["rules"].(map[string]any); ok {
			rulesText, _ = r["text"].(string)
		}
		if p, ok := body["panel"].(map[string]any); ok {
			panel, _ = p["template"].(string)
		}
	} else {
		rulesText, panel = Body(v.Rules), strings.TrimSpace(v.Panel)
	}
	if n := len(strings.TrimSpace(rulesText)); n > VariablesRulesMax {
		add("%s is %d bytes; the limit is %d bytes (%d over). Shorten the rules.", VariableRulesFile, n, VariablesRulesMax, n-VariablesRulesMax)
	}
	total := 0
	var bad []string
	for k, rule := range norm.Paths {
		total += len(k) + len(rule)
		if !variablePathExists(norm.Data, k) {
			bad = append(bad, k)
		}
	}
	sort.Strings(bad)
	for _, k := range bad {
		add("%s: the rule for %q does not match any value in data. Check which value it belongs to.", VariablesFile, k)
	}
	if total > VariablesPathRuleMax {
		add("%s: the rules attached to values take %d bytes; the limit is %d bytes (%d over). Shorten them or move them to %s.", VariablesFile, total, VariablesPathRuleMax, total-VariablesPathRuleMax, VariableRulesFile)
	}
	if n := len(panel); n > VariablesPanelMax {
		add("%s is %d bytes; the limit is %d bytes (%d over). Shorten the panel.", PanelFile, n, VariablesPanelMax, n-VariablesPanelMax)
	}
	return problems
}

func quotePath(p string) string {
	if p == "" {
		return "the top level"
	}
	return strconv.Quote(p)
}

// variablePathPart is one step of a variable path: an object key or a list index.
type variablePathPart struct {
	key   string
	index int
	isIdx bool
}

// parseVariablePath reads a.b, a[0].c, a["key.with.dots"] and /a/0/c the way the
// provider's ParsePath does.
func parseVariablePath(s string) ([]variablePathPart, bool) {
	type part = variablePathPart
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	var parts []part
	if strings.HasPrefix(s, "/") {
		for _, seg := range strings.Split(s[1:], "/") {
			seg = strings.ReplaceAll(strings.ReplaceAll(seg, "~1", "/"), "~0", "~")
			if i, err := strconv.Atoi(seg); err == nil && i >= 0 {
				parts = append(parts, part{key: seg, index: i, isIdx: true})
			} else {
				parts = append(parts, part{key: seg})
			}
		}
	} else {
		for i := 0; i < len(s); {
			switch s[i] {
			case '.':
				i++
			case '[':
				end := strings.IndexByte(s[i:], ']')
				if end < 0 {
					return nil, false
				}
				inner := strings.TrimSpace(s[i+1 : i+end])
				i += end + 1
				if len(inner) >= 2 && (inner[0] == '"' || inner[0] == '\'') && inner[len(inner)-1] == inner[0] {
					parts = append(parts, part{key: inner[1 : len(inner)-1]})
					continue
				}
				n, err := strconv.Atoi(inner)
				if err != nil || n < 0 {
					return nil, false
				}
				parts = append(parts, part{key: inner, index: n, isIdx: true})
			default:
				j := i
				for j < len(s) && s[j] != '.' && s[j] != '[' {
					j++
				}
				parts = append(parts, part{key: strings.TrimSpace(s[i:j])})
				i = j
			}
		}
	}
	return parts, len(parts) > 0
}

// formatVariablePath writes parts in the provider's canonical form.
func formatVariablePath(parts []variablePathPart) string {
	p := ""
	for _, pt := range parts {
		if pt.isIdx {
			p = variableIndexPath(p, pt.index)
		} else {
			p = variableKeyPath(p, pt.key)
		}
	}
	return p
}

// variablePathExists resolves a variable path against the values the way the
// provider does.
func variablePathExists(root *JSONNode, s string) bool {
	parts, ok := parseVariablePath(s)
	if !ok {
		return false
	}
	cur := root
	for _, pt := range parts {
		switch {
		case cur.Kind == JSONObject:
			v, ok := cur.Get(pt.key)
			if !ok {
				return false
			}
			cur = v
		case cur.Kind == JSONArray && pt.isIdx:
			if pt.index >= len(cur.Arr) {
				return false
			}
			cur = cur.Arr[pt.index]
		default:
			return false
		}
	}
	return len(parts) > 0
}

// OrphanVariableFiles reports variables.md or panel.hbs without variables.json:
// they are sent only together with it.
func OrphanVariableFiles(dir string, v *Variables) []string {
	if v != nil && !v.Empty {
		return nil
	}
	var out []string
	for _, name := range []string{VariableRulesFile, PanelFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			out = append(out, fmt.Sprintf("%s is not sent without %s; add the variables or remove the file", name, VariablesFile))
		}
	}
	return out
}
