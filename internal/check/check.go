package check

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/hearthroom/cli/internal/card"
)

// Finding is one observation: an error blocks, a warning is read, an info is context.
type Finding struct {
	Level string `json:"level"` // error | warning | info
	Where string `json:"where"`
	Msg   string `json:"msg"`
}

// Result is the whole report.
type Result struct {
	Status   string      `json:"status"` // ok | error
	Findings []Finding   `json:"findings"`
	Replay   *Health     `json:"replay,omitempty"`
	Declared Declaration `json:"declared"`
}

// Declaration is what README.md (never sent) says about the card's UI role.
type Declaration struct {
	UIRole    string  `json:"uiRole,omitempty"`    // assist | core | ""
	Threshold float64 `json:"threshold,omitempty"` // status overhead ratio the card accepts; 0 = not declared
}

// Errors counts the blocking findings.
func (r *Result) Errors() int {
	n := 0
	for _, f := range r.Findings {
		if f.Level == "error" {
			n++
		}
	}
	return n
}

type report struct{ findings []Finding }

func (r *report) add(level, where, format string, args ...any) {
	r.findings = append(r.findings, Finding{Level: level, Where: where, Msg: fmt.Sprintf(format, args...)})
}
func (r *report) errorf(where, f string, a ...any) { r.add("error", where, f, a...) }
func (r *report) warnf(where, f string, a ...any)  { r.add("warning", where, f, a...) }
func (r *report) infof(where, f string, a ...any)  { r.add("info", where, f, a...) }

var (
	slashForm   = regexp.MustCompile(`(?s)^/(.+)/([a-zA-Z]*)$`)
	sdkCap      = regexp.MustCompile(`\bsdk\.([a-zA-Z]+)(?:\.([a-zA-Z]+))?`)
	sdkOn       = regexp.MustCompile("\\bsdk\\.on\\(\\s*['\"`]([^'\"`]+)['\"`]")
	sdkOffOnce  = regexp.MustCompile(`\bsdk\.(off|once)\b`)
	saveSet     = regexp.MustCompile("\\bsave\\.set\\(\\s*['\"`]([^'\"`]*)['\"`]")
	awaitSend   = regexp.MustCompile(`(?s)\bawait\b.{0,200}\bsdk\.message\.send\(`)
	moduleSyn   = regexp.MustCompile(`\bimport\s+[\w{*]|\bexport\s+(default|const|function)`)
	httpScript  = regexp.MustCompile(`<script[^>]+src=["']http://`)
	scriptTag   = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script\s*>`)
	styleTag    = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style\s*>`)
	authorAttr  = regexp.MustCompile(`<[a-zA-Z][^>]*\s(data-[\w-]+|aria-[\w-]+|role)=`)
	svgBlock    = regexp.MustCompile(`(?is)<svg\b.*?</svg>`)
	onHandler   = regexp.MustCompile(`(?i)\son[a-z]+=`)
	cjkTag      = regexp.MustCompile(`</?([\x{4e00}-\x{9fff}][^\s>/]*)\s*/?>`)
	hcComponent = regexp.MustCompile(`\bhc-[a-z][a-z0-9-]*`)
	randomPipe  = regexp.MustCompile(`\{\{random:[^}]*\|[^}]*\}\}`)
	assetConcat = regexp.MustCompile("[\"'`]assets/[^\"'`]*[\"'`]\\s*\\+|\\+\\s*[\"'`][^\"'`]*assets/")
	assetLit    = regexp.MustCompile(`(["'])(assets/[^"'\s]+\.[a-zA-Z0-9]+)["']`)
	fieldToken  = regexp.MustCompile(`\$[a-zA-Z_]`)
	markerRegex = regexp.MustCompile(`\\\[([a-zA-Z0-9_\x{4e00}-\x{9fff}-]+)\\\]`)
	markerAngle = regexp.MustCompile(`<([a-zA-Z][a-zA-Z0-9_-]*)>`)
	markerLit   = regexp.MustCompile(`^\[([^\]]+)\]$|^<([^>]+)>$`)
	toldMarker  = regexp.MustCompile(`\[([a-zA-Z][a-zA-Z0-9_-]{1,30})\]`)
	mountDyn    = regexp.MustCompile(`\[data-chat|<script`)
	uiRoleLine  = regexp.MustCompile(`(?mi)^\s*uiRole:\s*(assist|core)\b`)
	threshLine  = regexp.MustCompile(`(?mi)^\s*statusOverheadThreshold:\s*(\d+(?:\.\d+)?)\s*(%?)`)
)

// ReadDeclarations parses the two declarations the checker reads from README.md.
func ReadDeclarations(readme string) Declaration {
	var d Declaration
	if m := uiRoleLine.FindStringSubmatch(readme); m != nil {
		d.UIRole = strings.ToLower(m[1])
	}
	if m := threshLine.FindStringSubmatch(readme); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		if m[2] == "%" || v > 1 {
			v /= 100
		}
		d.Threshold = v
	}
	return d
}

type consumed struct {
	name  string
	angle bool
	where string
}

// Card runs every check on a folder. It never contacts the provider.
func Card(dir string) (*Result, error) {
	r := &report{}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return ""
		}
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	res := &Result{Declared: ReadDeclarations(read("README.md"))}
	if res.Declared.UIRole == "" {
		r.infof("README.md", "no `uiRole: assist | core` declared; the checker assumes assist (the replies must read well with display rules off)")
	}
	if res.Declared.UIRole == "core" && res.Declared.Threshold == 0 {
		r.warnf("README.md", "a core card declares its own `statusOverheadThreshold:` with a reason; none found")
	}

	var manifest card.Manifest
	if raw := read(card.ManifestFile); raw != "" {
		if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
			r.errorf(card.ManifestFile, "not valid JSON: %v", err)
		}
	}
	var lorebook card.Lorebook
	if raw := read(card.LorebookFile); raw != "" {
		if err := json.Unmarshal([]byte(raw), &lorebook); err != nil {
			r.errorf(card.LorebookFile, "not valid JSON: %v", err)
		}
	}
	definition := read(card.DefinitionFile)
	welcome := read(card.WelcomeFile)
	var openings []string
	if entries, err := os.ReadDir(filepath.Join(dir, card.OpeningsDir)); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".md") {
				openings = append(openings, read(filepath.Join(card.OpeningsDir, e.Name())))
			}
		}
	}
	modelFacing := []string{definition, manifest.OutputContract, manifest.CustomInstructions}
	for _, e := range lorebook.Entries {
		if e.Constant && !e.Disabled {
			modelFacing = append(modelFacing, e.Content)
		}
	}
	modelText := strings.Join(modelFacing, "\n")
	playerText := strings.Join(append([]string{welcome}, openings...), "\n")

	rulesRaw := read(card.RulesFile)
	if rulesRaw != "" {
		var rules card.Rules
		if err := json.Unmarshal([]byte(rulesRaw), &rules); err != nil {
			r.errorf(card.RulesFile, "not valid JSON: %v", err)
		} else {
			checkRules(dir, &rules, modelText, playerText, r)
		}
	}
	scanHTML(playerText, card.WelcomeFile, r, false)
	res.Findings = r.findings
	if res.Findings == nil {
		res.Findings = []Finding{}
	}
	res.Status = "ok"
	if res.Errors() > 0 {
		res.Status = "error"
	}
	return res, nil
}

func checkRules(dir string, rules *card.Rules, modelText, playerText string, r *report) {
	where := card.RulesFile
	if rules.PageMode != "" && !contains(contract.Provider.PageMode, rules.PageMode) {
		r.errorf(where, "pageMode %q is not one of %s", rules.PageMode, strings.Join(contract.Provider.PageMode, ", "))
	}
	if rules.MountLayer != "" && !contains(contract.Provider.MountLayer, rules.MountLayer) {
		r.errorf(where, "mountLayer %q is not one of %s", rules.MountLayer, strings.Join(contract.Provider.MountLayer, ", "))
	}
	if rules.CardFormat != "" && !contains(contract.Provider.CardFormat, rules.CardFormat) {
		r.errorf(where, "cardFormat %q is not one of mmd, tavern", rules.CardFormat)
	}
	sandbox := rules.PageMode == "sandbox"
	total, usesSDK := 0, false
	ids := map[string]bool{}
	var markers []consumed
	var assets []struct{ path, where string }
	for i, rule := range rules.Rules {
		id := rule.ID
		if id == "" {
			id = strconv.Itoa(i)
		}
		rw := fmt.Sprintf("%s#%s", card.RulesFile, id)
		if rule.ID != "" {
			if ids[rule.ID] {
				r.errorf(rw, "duplicate rule id")
			}
			ids[rule.ID] = true
		}
		if strings.TrimSpace(rule.Find) == "" {
			r.errorf(rw, "find is blank (the provider rejects the whole rule set)")
		}
		rb := len(rule.Replace)
		total += len(rule.Find) + rb + len(rule.Name)
		if rb > contract.Provider.ReplaceMaxBytes {
			r.errorf(rw, "replace is %d bytes, over %d (UTF-8 bytes; strip comments or split script and style into two rules)", rb, contract.Provider.ReplaceMaxBytes)
		} else if float64(rb) > float64(contract.Provider.ReplaceMaxBytes)*0.85 {
			r.warnf(rw, "replace is %d bytes, close to the %d limit", rb, contract.Provider.ReplaceMaxBytes)
		}
		if !rule.Enabled {
			continue
		}
		checkFind(rule, rw, r, &markers)
		if mountDyn.MatchString(rule.Replace) && (strings.Contains(rule.Replace, "sdk.") || strings.Contains(rule.Replace, "[data-chat") || strings.Contains(rule.Replace, "[data-slot") || strings.Contains(rule.Replace, "--chat-")) {
			usesSDK = true
		}
		if strings.Contains(rule.Replace, "sdk.") || strings.Contains(rule.Replace, "[data-chat") || strings.Contains(rule.Replace, "[data-slot") || strings.Contains(rule.Replace, "--chat-") {
			usesSDK = true
		}
		for _, m := range scriptTag.FindAllStringSubmatch(rule.Replace, -1) {
			if strings.Contains(strings.ToLower(m[1]), "src=") {
				continue
			}
			scanScript(m[2], rw, r)
		}
		scanHTML(rule.Replace, rw, r, true)
		for _, m := range assetLit.FindAllStringSubmatch(rule.Replace, -1) {
			assets = append(assets, struct{ path, where string }{m[2], rw})
		}
	}
	if total > contract.Provider.TotalMaxBytes {
		r.errorf(where, "the rule set is %d bytes, over %d", total, contract.Provider.TotalMaxBytes)
	}
	if usesSDK && !sandbox {
		r.errorf(where, "rules use the sandbox author API (sdk / [data-chat] / --chat-*) but pageMode is not \"sandbox\"")
	}
	if rules.MountTrigger != "" && mountDyn.MatchString(rules.MountTrigger) {
		r.warnf(where+"#mountTrigger", "the function bar is rendered once at load and only its text goes through rules; put scripts in a rule")
	}
	// Render rules are not generation rules: a marker a rule consumes must be something the
	// model is told to write (bracketed forms only; a bare word appears in almost any prose).
	for _, m := range markers {
		told := strings.Contains(modelText, "["+m.name+"]") || strings.Contains(modelText, "<"+m.name+">") || strings.Contains(modelText, "[/"+m.name+"]") || strings.Contains(modelText, "【"+m.name+"】")
		shown := strings.Contains(playerText, "["+m.name+"]") || strings.Contains(playerText, "<"+m.name+">")
		switch {
		case !told && !shown:
			r.warnf(m.where, "the rule consumes the marker %q but neither the definition, the output contract, a constant Lorebook entry nor an opening mentions it: the panel will never appear after the first message", m.name)
		case !told && shown:
			r.warnf(m.where, "%q appears in the opening but the model is never told to emit it: it shows once and never updates", m.name)
		}
		if m.angle && sandbox && hasCJK(m.name) {
			r.warnf(m.where, "<%s> works in a rule but is stripped before any script reads the bubble; prefer [%s]", m.name, m.name)
		}
	}
	if len(rules.Rules) > 0 {
		finds := make([]string, 0, len(rules.Rules))
		for _, rule := range rules.Rules {
			finds = append(finds, rule.Find)
		}
		joined := strings.Join(finds, "\n")
		seen := map[string]bool{}
		for _, m := range toldMarker.FindAllStringSubmatch(modelText, -1) {
			name := m[1]
			if seen[name] || name == "hr-pinned" {
				continue
			}
			seen[name] = true
			consumedByRule := false
			for _, c := range markers {
				if c.name == name {
					consumedByRule = true
				}
			}
			if !consumedByRule && !strings.Contains(joined, "["+name+"]") {
				r.infof(card.DefinitionFile, "the model is told to write [%s] but no rule consumes it; it will stay visible as text (fine if intended)", name)
			}
		}
	}
	for _, a := range assets {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(a.path))); err != nil {
			r.errorf(a.where, "%s is referenced but the file does not exist in the card folder", a.path)
		}
	}
}

func checkFind(rule card.DisplayRule, rw string, r *report, markers *[]consumed) {
	m := slashForm.FindStringSubmatch(strings.Trim(rule.Find, "`"))
	if m == nil {
		lit := strings.TrimSpace(rule.Find)
		if mm := markerLit.FindStringSubmatch(lit); mm != nil {
			name := mm[1]
			if name == "" {
				name = mm[2]
			}
			*markers = append(*markers, consumed{name: name, angle: strings.HasPrefix(lit, "<"), where: rw})
		}
		return
	}
	source, flags := m[1], m[2]
	for _, f := range flags {
		if !strings.ContainsRune(contract.Rules.Flags, f) {
			r.errorf(rw, "flag %q is not allowed (only %s)", string(f), contract.Rules.Flags)
			return
		}
	}
	re, err := compileJS(source, flags)
	if err != nil {
		if errors.Is(err, errUnsupportedSyntax) {
			r.infof(rw, "pattern uses lookaround or backreferences, which this local check cannot compile (the play page can); the provider's render verifies it")
		} else {
			r.errorf(rw, "invalid pattern: %v (rolled back as bad_regex)", err)
			return
		}
	} else if re.MatchString("") {
		r.errorf(rw, "the pattern can match the empty string (rolled back as empty_match)")
	}
	if fieldToken.MatchString(rule.Replace) && !strings.Contains(source, "(") {
		r.warnf(rw, "$name in the replacement needs a first capture group shaped key::value;;key::value")
	}
	if mm := markerRegex.FindStringSubmatch(source); mm != nil {
		*markers = append(*markers, consumed{name: mm[1], where: rw})
	} else if mm := markerAngle.FindStringSubmatch(source); mm != nil {
		*markers = append(*markers, consumed{name: mm[1], angle: true, where: rw})
	}
}

var errUnsupportedSyntax = errors.New("unsupported syntax")

// compileJS compiles a JavaScript pattern with Go's RE2 engine. The play page runs the real
// JavaScript engine, so lookaround and backreferences are reported, not rejected.
func compileJS(source, flags string) (*regexp.Regexp, error) {
	if strings.Contains(source, "(?=") || strings.Contains(source, "(?!") || strings.Contains(source, "(?<=") || strings.Contains(source, "(?<!") || regexp.MustCompile(`\\[1-9]`).MatchString(source) {
		return nil, errUnsupportedSyntax
	}
	prefix := "(?"
	if strings.ContainsRune(flags, 'i') {
		prefix += "i"
	}
	if strings.ContainsRune(flags, 'm') {
		prefix += "m"
	}
	if strings.ContainsRune(flags, 's') {
		prefix += "s"
	}
	if prefix == "(?" {
		prefix = ""
	} else {
		prefix += ")"
	}
	// JavaScript escapes Go's RE2 does not know: \/ and \- inside the pattern.
	src := strings.ReplaceAll(source, `\/`, `/`)
	return regexp.Compile(prefix + src)
}

func scanScript(code, where string, r *report) {
	caps := contract.SDK.Capabilities
	for _, m := range sdkCap.FindAllStringSubmatch(code, -1) {
		key, method := m[1], m[2]
		if key == "on" || key == "version" {
			continue
		}
		if !contains(contract.SDK.Keys, key) {
			r.errorf(where, "sdk.%s does not exist on the sandbox page (keys: %s)", key, strings.Join(contract.SDK.Keys, ", "))
			continue
		}
		if method != "" && !contains(caps, key+"."+method) {
			r.errorf(where, "sdk.%s.%s does not exist; a misspelled capability never runs and never errors", key, method)
		}
	}
	for _, m := range sdkOn.FindAllStringSubmatch(code, -1) {
		if !contains(contract.Events.Names, m[1]) {
			r.errorf(where, "sdk.on(%q) is not an event; it never fires (events: %s)", m[1], strings.Join(contract.Events.Names, ", "))
		}
	}
	if sdkOffOnce.MatchString(code) {
		r.errorf(where, "sdk.off / sdk.once do not exist; keep your own registry")
	}
	for _, api := range []string{"sdk.vars", "vars:change", "<abc_vars"} {
		if strings.Contains(code, api) {
			r.errorf(where, "%s is not provided by any Hearthroom chat page", api)
		}
	}
	if moduleSyn.MatchString(code) {
		r.errorf(where, "ES module syntax: rule scripts run as classic scripts")
	}
	if awaitSend.MatchString(code) {
		r.warnf(where, "an await before sdk.message.send leaves the click gesture: the player will be asked to confirm")
	}
	keyRe, err := regexp.Compile(contract.Save.KeyPattern)
	if err == nil {
		for _, m := range saveSet.FindAllStringSubmatch(code, -1) {
			if !keyRe.MatchString(m[1]) {
				r.errorf(where, "save key %q is not valid (%s)", m[1], contract.Save.KeyPattern)
			}
		}
	}
	if strings.Contains(code, "document.currentScript") {
		r.infof(where, "document.currentScript is the running element for inline rule scripts but null in the wrapped fallback; do not depend on it")
	}
	if httpScript.MatchString(code) {
		r.warnf(where, "http:// external scripts are skipped; use https://")
	}
}

func scanHTML(html, where string, r *report, isRule bool) {
	stripped := styleTag.ReplaceAllString(scriptTag.ReplaceAllString(html, ""), "")
	for _, m := range authorAttr.FindAllStringSubmatch(stripped, -1) {
		r.warnf(where, "attribute %s is removed by the sanitizer on the shell render path; use a class instead", m[1])
	}
	for _, m := range svgBlock.FindAllString(stripped, -1) {
		if onHandler.MatchString(m) {
			r.warnf(where, "on* handlers inside <svg> are removed")
		}
	}
	for _, m := range cjkTag.FindAllStringSubmatch(stripped, -1) {
		r.warnf(where, "<%s> is stripped by the sanitizer (text kept); use [%s] square brackets for model markers", m[1], m[1])
	}
	if m := hcComponent.FindString(stripped); m != "" {
		r.warnf(where, "%s is a classic-page component; the sandbox page does not register it", m)
	}
	if randomPipe.MatchString(html) {
		r.warnf(where, "{{random:a|b}} uses the wrong separator; the engine expects {{random:a::b}}")
	}
	if assetConcat.MatchString(html) {
		r.warnf(where, "an assets/ path built by concatenation is not uploaded on push; write every path as one literal")
	}
	_ = isRule
}

func hasCJK(s string) bool {
	for _, c := range s {
		if c >= 0x4e00 && c <= 0x9fff {
			return true
		}
	}
	return false
}
