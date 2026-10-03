package importer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Limits that mirror the community site's importer.
const (
	// TagsMax is the maximum number of tags a card keeps.
	TagsMax = 10
)

// EntryContentMaxFor is the per-entry Lorebook content limit for a card
// language, matching the provider: 4000 characters for Chinese (and for an
// empty or unknown language), 6000 for Japanese and Korean, 12000 for
// English. Longer entries are split into several with the same keywords.
func EntryContentMaxFor(language string) int {
	l := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(language)), "_", "-")
	switch {
	case strings.HasPrefix(l, "en"):
		return 12000
	case strings.HasPrefix(l, "ja"), strings.HasPrefix(l, "ko"):
		return 6000
	}
	return 4000
}

// Card is a SillyTavern character card (V1 flattened, V2 or V3).
type Card struct {
	Spec        string
	SpecVersion string
	Data        CardData
}

// CardData holds the card fields the importer understands. Unknown keys are
// kept in Raw for reporting.
type CardData struct {
	Name                     string            `json:"name"`
	Description              string            `json:"description"`
	Personality              string            `json:"personality"`
	Scenario                 string            `json:"scenario"`
	FirstMes                 string            `json:"first_mes"`
	MesExample               string            `json:"mes_example"`
	CreatorNotes             string            `json:"creator_notes"`
	SystemPrompt             string            `json:"system_prompt"`
	PostHistoryInstructions  string            `json:"post_history_instructions"`
	AlternateGreetings       []string          `json:"alternate_greetings"`
	GroupOnlyGreetings       []string          `json:"group_only_greetings"`
	CharacterBook            *Book             `json:"character_book"`
	Tags                     []string          `json:"tags"`
	Creator                  string            `json:"creator"`
	CharacterVersion         string            `json:"character_version"`
	Nickname                 string            `json:"nickname"`
	Source                   []string          `json:"source"`
	CreatorNotesMultilingual map[string]string `json:"creator_notes_multilingual"`
	CreationDate             json.Number       `json:"creation_date"`
	ModificationDate         json.Number       `json:"modification_date"`
	Assets                   []Asset           `json:"assets"`
	Extensions               map[string]any    `json:"extensions"`
}

// Asset is a V3 asset reference.
type Asset struct {
	Type string `json:"type"`
	URI  string `json:"uri"`
	Name string `json:"name"`
	Ext  string `json:"ext"`
}

// Book is a character_book or a World Info file normalised to one shape.
type Book struct {
	Name              string
	Description       string
	ScanDepth         int
	TokenBudget       int
	RecursiveScanning bool
	Entries           []BookEntry
}

// BookEntry is one Lorebook entry in SillyTavern terms.
type BookEntry struct {
	Keys            []string
	SecondaryKeys   []string
	Content         string
	Name            string
	Comment         string
	Enabled         bool
	Constant        bool
	Selective       *bool
	ScanDepth       int
	InsertionOrder  int
	Position        string
	CaseSensitive   bool
	MatchWholeWords bool
	SelectiveLogic  int
	UseRegex        bool
	Extensions      map[string]any
}

// Parsed is a card read from a file, with any bundled images.
type Parsed struct {
	Card          Card
	Image         []byte // portrait bytes (PNG cards: the file itself)
	ImageExt      string
	Background    []byte
	BackgroundExt string
	// Files holds other CHARX-bundled assets by archive path.
	Files map[string][]byte
}

var zipMagic = []byte{0x50, 0x4b, 0x03, 0x04}

// ParseCardFile detects PNG, CHARX (zip) or JSON by content.
func ParseCardFile(b []byte) (*Parsed, error) {
	switch {
	case IsPNG(b):
		raw, err := pngCardJSON(b)
		if err != nil {
			return nil, err
		}
		card, err := unwrap(raw)
		if err != nil {
			return nil, err
		}
		p := &Parsed{Card: card, Image: b, ImageExt: "png"}
		if bg := mainAsset(card.Data.Assets, "background"); bg != nil {
			p.Background, p.BackgroundExt = dataURI(bg.URI)
		}
		return p, nil
	case len(b) >= 4 && bytes.Equal(b[:4], zipMagic):
		return parseCharx(b)
	default:
		card, err := unwrap(b)
		if err != nil {
			return nil, err
		}
		p := &Parsed{Card: card}
		if icon := mainAsset(card.Data.Assets, "icon"); icon != nil {
			p.Image, p.ImageExt = dataURI(icon.URI)
		}
		if bg := mainAsset(card.Data.Assets, "background"); bg != nil {
			p.Background, p.BackgroundExt = dataURI(bg.URI)
		}
		return p, nil
	}
}

func parseCharx(b []byte) (*Parsed, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, ErrInvalid
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || f.UncompressedSize64 > 200<<20 {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, ErrInvalid
		}
		data, err := io.ReadAll(io.LimitReader(rc, 200<<20))
		rc.Close()
		if err != nil {
			return nil, ErrInvalid
		}
		files[f.Name] = data
	}
	cardRaw, ok := files["card.json"]
	if !ok {
		return nil, ErrNoMetadata
	}
	card, err := unwrap(cardRaw)
	if err != nil {
		return nil, err
	}
	p := &Parsed{Card: card, Files: files}
	embedded := func(a *Asset) ([]byte, string) {
		if a == nil {
			return nil, ""
		}
		uri := strings.TrimSpace(a.URI)
		if strings.HasPrefix(uri, "data:") {
			return dataURI(uri)
		}
		if rest, ok := strings.CutPrefix(uri, "embeded://"); ok {
			if data, ok := files[rest]; ok {
				ext := strings.ToLower(strings.TrimPrefix(a.Ext, "."))
				if ext == "" {
					ext = strings.TrimPrefix(path.Ext(rest), ".")
				}
				return data, ext
			}
		}
		return nil, ""
	}
	p.Image, p.ImageExt = embedded(mainAsset(card.Data.Assets, "icon"))
	p.Background, p.BackgroundExt = embedded(mainAsset(card.Data.Assets, "background"))
	return p, nil
}

// unwrap accepts {spec, data}, a flattened V1/V3 object, and rejects the rest.
func unwrap(raw []byte) (Card, error) {
	var obj map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&obj); err != nil || obj == nil {
		return Card{}, ErrInvalid
	}
	spec := jsonString(obj["spec"])
	specVersion := jsonString(obj["spec_version"])
	if data, ok := obj["data"]; ok && bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		var cd CardData
		if err := decodeData(data, &cd); err != nil {
			return Card{}, ErrInvalid
		}
		if spec == "" {
			spec = "chara_card_v2"
		}
		if specVersion == "" {
			specVersion = "2.0"
		}
		return Card{Spec: spec, SpecVersion: specVersion, Data: cd}, nil
	}
	if _, hasName := obj["name"]; hasName || obj["first_mes"] != nil {
		var cd CardData
		if err := decodeData(raw, &cd); err != nil {
			return Card{}, ErrInvalid
		}
		if spec == "" {
			spec = "chara_card_v1"
		}
		if specVersion == "" {
			specVersion = "1.0"
		}
		return Card{Spec: spec, SpecVersion: specVersion, Data: cd}, nil
	}
	return Card{}, ErrInvalid
}

func decodeData(raw []byte, cd *CardData) error {
	// Decode into a loose map first so a character_book in either shape and
	// odd field types do not abort the whole import.
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		return err
	}
	cd.Name = text(m["name"])
	cd.Description = text(m["description"])
	cd.Personality = text(m["personality"])
	cd.Scenario = text(m["scenario"])
	cd.FirstMes = text(m["first_mes"])
	cd.MesExample = text(m["mes_example"])
	cd.CreatorNotes = text(m["creator_notes"])
	cd.SystemPrompt = text(m["system_prompt"])
	cd.PostHistoryInstructions = text(m["post_history_instructions"])
	cd.AlternateGreetings = list(m["alternate_greetings"])
	cd.GroupOnlyGreetings = list(m["group_only_greetings"])
	cd.Tags = list(m["tags"])
	cd.Creator = text(m["creator"])
	cd.CharacterVersion = text(m["character_version"])
	cd.Nickname = text(m["nickname"])
	cd.Source = list(m["source"])
	if notes, ok := m["creator_notes_multilingual"].(map[string]any); ok {
		cd.CreatorNotesMultilingual = map[string]string{}
		for k, v := range notes {
			if strings.TrimSpace(k) != "" && text(v) != "" {
				cd.CreatorNotesMultilingual[strings.TrimSpace(k)] = text(v)
			}
		}
	}
	cd.CreationDate = number(m["creation_date"])
	cd.ModificationDate = number(m["modification_date"])
	if assets, ok := m["assets"].([]any); ok {
		for _, a := range assets {
			if am, ok := a.(map[string]any); ok {
				cd.Assets = append(cd.Assets, Asset{Type: text(am["type"]), URI: text(am["uri"]), Name: text(am["name"]), Ext: text(am["ext"])})
			}
		}
	}
	if ext, ok := m["extensions"].(map[string]any); ok {
		cd.Extensions = ext
	}
	if book := WorldInfoToBook(m["character_book"]); book != nil {
		cd.CharacterBook = book
	}
	return nil
}

func jsonString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

func text(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func list(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, x := range arr {
		if s := text(x); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func number(v any) json.Number {
	switch n := v.(type) {
	case json.Number:
		return n
	case float64:
		return json.Number(strconv.FormatFloat(n, 'f', -1, 64))
	case string:
		if _, err := strconv.ParseFloat(n, 64); err == nil {
			return json.Number(n)
		}
	}
	return ""
}

func numberOr(v any, fallback int) int {
	switch n := v.(type) {
	case json.Number:
		if f, err := n.Float64(); err == nil && !math.IsNaN(f) {
			return int(f)
		}
	case float64:
		return int(n)
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return int(f)
		}
	}
	return fallback
}

func boolOf(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

// keyList accepts an array, a JSON-encoded array string (MMD World Info
// exports) or a comma-separated string.
func keyList(v any) []string {
	if arr, ok := v.([]any); ok {
		return list(arr)
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return nil
	}
	var parsed []any
	if err := json.Unmarshal([]byte(s), &parsed); err == nil {
		return list(parsed)
	}
	var out []string
	for _, k := range regexp.MustCompile(`[,，]`).Split(s, -1) {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}

func ext(row map[string]any) map[string]any {
	if e, ok := row["extensions"].(map[string]any); ok {
		return e
	}
	return map[string]any{}
}

func firstNonNil(vals ...any) any {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

// WorldInfoToBook normalises a character_book object, a World Info file
// (entries keyed by uid; key/keysecondary/disable field names) or a whole
// card (its book) into a Book. Returns nil when the value is not a book.
func WorldInfoToBook(raw any) *Book {
	obj, ok := raw.(map[string]any)
	if !ok || obj == nil {
		return nil
	}
	if data, ok := obj["data"].(map[string]any); ok {
		return WorldInfoToBook(data["character_book"])
	}
	entriesAny, ok := obj["entries"]
	if !ok || entriesAny == nil {
		return nil
	}
	var rows []map[string]any
	switch e := entriesAny.(type) {
	case []any:
		for _, r := range e {
			if rm, ok := r.(map[string]any); ok {
				rows = append(rows, rm)
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(e))
		for k := range e {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			a, ea := strconv.Atoi(keys[i])
			b, eb := strconv.Atoi(keys[j])
			if ea == nil && eb == nil {
				return a < b
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys {
			if rm, ok := e[k].(map[string]any); ok {
				rows = append(rows, rm)
			}
		}
	default:
		return nil
	}
	bookDepth := numberOr(firstNonNil(obj["scan_depth"], obj["scanDepth"]), 2)
	book := &Book{
		Name:              text(obj["name"]),
		Description:       text(obj["description"]),
		ScanDepth:         bookDepth,
		TokenBudget:       numberOr(obj["token_budget"], 0),
		RecursiveScanning: boolOf(obj["recursive_scanning"]),
	}
	for _, row := range rows {
		enabled := true
		if v, ok := row["enabled"]; ok {
			enabled = !(v == false)
		} else if boolOf(row["disable"]) {
			enabled = false
		}
		var selective *bool
		if v, ok := row["selective"].(bool); ok {
			selective = &v
		}
		e := BookEntry{
			Keys:            keyList(firstNonNil(row["keys"], row["key"])),
			SecondaryKeys:   keyList(firstNonNil(row["secondary_keys"], row["keysecondary"])),
			Content:         text(row["content"]),
			Name:            text(row["name"]),
			Comment:         text(row["comment"]),
			Enabled:         enabled,
			Constant:        boolOf(row["constant"]),
			Selective:       selective,
			ScanDepth:       numberOr(firstNonNil(row["scan_depth"], row["scanDepth"], ext(row)["scan_depth"], ext(row)["scanDepth"]), bookDepth),
			InsertionOrder:  numberOr(firstNonNil(row["insertion_order"], row["order"]), 0),
			Position:        text(row["position"]),
			CaseSensitive:   boolOf(row["case_sensitive"]) || boolOf(row["caseSensitive"]) || boolOf(ext(row)["case_sensitive"]),
			MatchWholeWords: boolOf(row["matchWholeWords"]) || boolOf(row["match_whole_words"]) || boolOf(ext(row)["match_whole_words"]),
			SelectiveLogic:  numberOr(firstNonNil(row["selectiveLogic"], row["selective_logic"], ext(row)["selectiveLogic"], ext(row)["selective_logic"]), 0),
			UseRegex:        boolOf(row["use_regex"]) || boolOf(ext(row)["use_regex"]),
			Extensions:      ext(row),
		}
		book.Entries = append(book.Entries, e)
	}
	return book
}

func mainAsset(assets []Asset, typ string) *Asset {
	var first *Asset
	for i := range assets {
		a := &assets[i]
		if a.Type != typ || strings.TrimSpace(a.URI) == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(a.Name), "main") {
			return a
		}
		if first == nil {
			first = a
		}
	}
	return first
}

var dataURIRe = regexp.MustCompile(`(?i)^data:(image/[a-z0-9.+-]+);base64,([A-Za-z0-9+/=\s]+)$`)

// dataURI decodes an image data: URI; returns nil for anything else.
func dataURI(uri string) ([]byte, string) {
	m := dataURIRe.FindStringSubmatch(strings.TrimSpace(uri))
	if m == nil {
		return nil, ""
	}
	raw, err := decodeBase64UTF8(m[2])
	if err != nil {
		return nil, ""
	}
	ext := strings.ToLower(strings.TrimPrefix(m[1], "image/"))
	if ext == "jpeg" {
		ext = "jpg"
	}
	return raw, ext
}

// TalkLine is one example-dialogue line.
type TalkLine struct {
	RoleType string
	Content  string
}

var startRe = regexp.MustCompile(`(?i)^<\s*START\s*>$`)
var speakerRe = regexp.MustCompile(`(?i)^\{\{(user|char)\}\}\s*:\s*(.*)$`)

// ParseMesExample splits SillyTavern example dialogue into lines. An empty
// result means the text was free-form and should be reported instead.
func ParseMesExample(raw string) []TalkLine {
	var out []TalkLine
	var cur *TalkLine
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || startRe.MatchString(t) {
			cur = nil
			continue
		}
		if m := speakerRe.FindStringSubmatch(t); m != nil {
			role := "ai"
			if strings.EqualFold(m[1], "user") {
				role = "user"
			}
			out = append(out, TalkLine{RoleType: role, Content: strings.TrimSpace(m[2])})
			cur = &out[len(out)-1]
			continue
		}
		if cur != nil {
			cur.Content = strings.TrimSpace(cur.Content + "\n" + t)
		}
	}
	var kept []TalkLine
	for _, l := range out {
		if l.Content != "" {
			kept = append(kept, l)
		}
	}
	return kept
}

// JoinPersona combines description, personality and scenario with headings,
// skipping duplicates (MMD exports repeat the persona verbatim).
func JoinPersona(d CardData) string {
	var parts []string
	desc, pers, scen := strings.TrimSpace(d.Description), strings.TrimSpace(d.Personality), strings.TrimSpace(d.Scenario)
	if desc != "" {
		parts = append(parts, desc)
	}
	if pers != "" && pers != desc {
		parts = append(parts, "## Personality\n"+pers)
	}
	if scen != "" && scen != desc && scen != pers {
		parts = append(parts, "## Scenario\n"+scen)
	}
	return strings.Join(parts, "\n\n")
}

// Decorators parsed from the top of a V3 entry.
type Decorators struct {
	Content        string
	Activate       bool
	DontActivate   bool
	AdditionalKeys []string
	Unsupported    []string
}

var decoratorRe = regexp.MustCompile(`(?i)^([a-z_]+)\s*(.*)$`)

// ParseDecorators strips leading `@@name value` lines the way SillyTavern
// reads them; `@@@` lines are fallbacks used only when the previous
// decorator was unknown.
func ParseDecorators(raw string) Decorators {
	out := Decorators{Content: raw}
	if !strings.HasPrefix(raw, "@@") {
		return out
	}
	lines := strings.Split(raw, "\n")
	fallbacked := false
	i := 0
	for ; i < len(lines); i++ {
		line := lines[i]
		if !strings.HasPrefix(line, "@@") {
			break
		}
		isFallback := strings.HasPrefix(line, "@@@")
		if isFallback && !fallbacked {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "@@@"), "@@"))
		if !isFallback {
			body = strings.TrimSpace(line[2:])
		}
		m := decoratorRe.FindStringSubmatch(body)
		name, value := "", ""
		if m != nil {
			name, value = strings.ToLower(m[1]), strings.TrimSpace(m[2])
		}
		switch name {
		case "activate":
			out.Activate = true
		case "dont_activate":
			out.DontActivate = true
		case "additional_keys":
			for _, k := range strings.Split(value, ",") {
				if k = strings.TrimSpace(k); k != "" {
					out.AdditionalKeys = append(out.AdditionalKeys, k)
				}
			}
		default:
			if name != "" && !contains(out.Unsupported, name) {
				out.Unsupported = append(out.Unsupported, name)
			}
			fallbacked = true
			continue
		}
		fallbacked = false
	}
	out.Content = strings.Join(lines[i:], "\n")
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

var regexLiteral = regexp.MustCompile(`^/.+/[a-z]*$`)

// RegexKey wraps a keyword as /key/i unless it already is a regex literal.
func RegexKey(key string, caseSensitive bool) string {
	if regexLiteral.MatchString(key) {
		return key
	}
	if caseSensitive {
		return "/" + key + "/"
	}
	return "/" + key + "/i"
}

// SplitEntryContent splits content longer than max into pieces at newlines.
func SplitEntryContent(content string, max int) []string {
	if utf8.RuneCountInString(content) <= max {
		return []string{content}
	}
	budget := max - 40
	var out []string
	current := ""
	for _, para := range strings.Split(content, "\n") {
		piece := para
		candidate := piece
		if current != "" {
			candidate = current + "\n" + piece
		}
		if utf8.RuneCountInString(candidate) <= budget {
			current = candidate
			continue
		}
		if current != "" {
			out = append(out, current)
		}
		current = ""
		for utf8.RuneCountInString(piece) > budget {
			r := []rune(piece)
			out = append(out, string(r[:budget]))
			piece = string(r[budget:])
		}
		current = piece
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}

// EntryDraft is a Lorebook entry in the folder's shape plus match options.
type EntryDraft struct {
	Name              string
	Content           string
	Keywords          []string
	SecondaryKeywords []string
	Enabled           bool
	Constant          bool
	MatchOptions      map[string]any
}

func entryKeywords(e BookEntry, raw []string) []string {
	if !e.UseRegex {
		return raw
	}
	out := make([]string, 0, len(raw))
	for _, k := range raw {
		out = append(out, RegexKey(k, e.CaseSensitive))
	}
	return out
}

// EntryMatchOptions builds the provider's Tavern-format match options.
func EntryMatchOptions(e BookEntry) map[string]any {
	opts := map[string]any{
		"caseSensitive":   e.CaseSensitive,
		"matchWholeWords": e.MatchWholeWords,
		"selectiveLogic":  0,
	}
	if e.SelectiveLogic >= 0 && e.SelectiveLogic <= 3 {
		opts["selectiveLogic"] = e.SelectiveLogic
	}
	if e.Selective != nil {
		opts["selective"] = *e.Selective
	}
	if e.ScanDepth > 0 && e.ScanDepth <= 100 {
		opts["scanDepth"] = e.ScanDepth
	}
	opts["order"] = e.InsertionOrder
	if g, ok := e.Extensions["harperharbor"].(map[string]any); ok {
		if id, ok := g["groupId"].(string); ok && id != "" {
			opts["groupId"] = id
			opts["groupOrder"] = numberOr(g["groupOrder"], 0)
		}
	}
	if len(e.Extensions) > 0 {
		opts["extensions"] = e.Extensions
	}
	return opts
}

// BookEntriesToDrafts converts entries, splitting content longer than the
// card language's limit and keeping the site's naming rules.
func BookEntriesToDrafts(entries []BookEntry, language string) []EntryDraft {
	limit := EntryContentMaxFor(language)
	var drafts []EntryDraft
	for i, e := range entries {
		dec := ParseDecorators(strings.TrimSpace(e.Content))
		content := strings.TrimSpace(dec.Content)
		if content == "" {
			continue
		}
		base := e.Name
		if base == "" {
			base = e.Comment
		}
		if base == "" && len(e.Keys) > 0 {
			base = e.Keys[0]
		}
		if base == "" {
			base = fmt.Sprintf("#%d", i+1)
		}
		keywords := entryKeywords(e, append(append([]string{}, e.Keys...), dec.AdditionalKeys...))
		parts := SplitEntryContent(content, limit)
		opts := EntryMatchOptions(e)
		groupID, _ := opts["groupId"].(string)
		if groupID == "" && len(parts) > 1 {
			groupID = fmt.Sprintf("split-%d", i+1)
		}
		for pi, part := range parts {
			name := truncateRunes(base, 20)
			if len(parts) > 1 {
				name = fmt.Sprintf("%s (%d/%d)", truncateRunes(base, 14), pi+1, len(parts))
			}
			mo := map[string]any{}
			for k, v := range opts {
				mo[k] = v
			}
			if groupID != "" {
				mo["groupId"] = groupID
				if len(parts) > 1 {
					mo["groupOrder"] = pi
				} else if _, ok := mo["groupOrder"]; !ok {
					mo["groupOrder"] = 0
				}
			}
			drafts = append(drafts, EntryDraft{
				Name:              name,
				Content:           part,
				Keywords:          keywords,
				SecondaryKeywords: entryKeywords(e, e.SecondaryKeys),
				Enabled:           e.Enabled && !dec.DontActivate,
				Constant:          e.Constant || dec.Activate,
				MatchOptions:      mo,
			})
		}
	}
	return drafts
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// BookEntryNotes reports entry-level fields that have no destination and
// entries split for the card language's limit.
func BookEntryNotes(entries []BookEntry, language string) []string {
	limit := EntryContentMaxFor(language)
	positions, decorators := 0, 0
	names := map[string]bool{}
	split := 0
	for _, e := range entries {
		if e.Position != "" {
			positions++
		}
		dec := ParseDecorators(strings.TrimSpace(e.Content))
		if len(dec.Unsupported) > 0 {
			decorators++
			for _, n := range dec.Unsupported {
				names["@@"+n] = true
			}
		}
		if utf8.RuneCountInString(strings.TrimSpace(dec.Content)) > limit {
			split++
		}
	}
	var notes []string
	if positions > 0 {
		notes = append(notes, fmt.Sprintf("%d Lorebook entries carry an insertion position; the provider places entries itself, so it was not imported", positions))
	}
	if decorators > 0 {
		keys := make([]string, 0, len(names))
		for k := range names {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		notes = append(notes, fmt.Sprintf("%d Lorebook entries used decorators without a destination (%s); they were stripped", decorators, strings.Join(keys, ", ")))
	}
	if split > 0 {
		notes = append(notes, fmt.Sprintf("%d Lorebook entries were longer than %d characters and were split into several entries with the same keywords", split, limit))
	}
	return notes
}

// MultilingualNote picks the creator note for a language (exact, then same
// base language).
func MultilingualNote(notes map[string]string, language string) string {
	if len(notes) == 0 {
		return ""
	}
	want := strings.ToLower(language)
	base := strings.SplitN(want, "-", 2)[0]
	fallback := ""
	keys := make([]string, 0, len(notes))
	for k := range notes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		key := strings.ToLower(k)
		if key == want {
			return strings.TrimSpace(notes[k])
		}
		if fallback == "" && strings.SplitN(key, "-", 2)[0] == base {
			fallback = strings.TrimSpace(notes[k])
		}
	}
	return fallback
}

// CardMetaFromTavern keeps V3 provenance in the provider's cardMeta shape.
func CardMetaFromTavern(d CardData) map[string]any {
	meta := map[string]any{}
	if d.Creator != "" {
		meta["creator"] = d.Creator
	}
	if d.CharacterVersion != "" {
		meta["characterVersion"] = d.CharacterVersion
	}
	if len(d.Source) > 0 {
		meta["source"] = d.Source
	}
	if len(d.CreatorNotesMultilingual) > 0 {
		meta["creatorNotesMultilingual"] = d.CreatorNotesMultilingual
	}
	if n := positiveInt(d.CreationDate); n > 0 {
		meta["creationDate"] = n
	}
	if n := positiveInt(d.ModificationDate); n > 0 {
		meta["modificationDate"] = n
	}
	return meta
}

func positiveInt(n json.Number) int64 {
	if n == "" {
		return 0
	}
	f, err := n.Float64()
	if err != nil || f <= 0 {
		return 0
	}
	return int64(math.Floor(f))
}

// RegexRule is one display rule in the folder's shape.
type RegexRule struct {
	Name    string
	Find    string
	Replace string
	Enabled bool
}

// RulesFromTavern reads extensions.regex_scripts, keeping only scripts that
// act on AI output (placement 2, or no placement).
func RulesFromTavern(scripts any) []RegexRule {
	arr, ok := scripts.([]any)
	if !ok {
		return nil
	}
	var out []RegexRule
	for i, s := range arr {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		find := strText(m["findRegex"])
		if find == "" {
			continue
		}
		if pl, ok := m["placement"].([]any); ok && len(pl) > 0 {
			acts := false
			for _, p := range pl {
				if numberOr(p, -1) == 2 {
					acts = true
				}
			}
			if !acts {
				continue
			}
		}
		name := strText(m["scriptName"])
		if name == "" {
			name = fmt.Sprintf("#%d", i+1)
		}
		out = append(out, RegexRule{Name: name, Find: find, Replace: strText(m["replaceString"]), Enabled: !boolOf(m["disabled"])})
	}
	return out
}

// strText returns a string value untrimmed (rule bodies keep whitespace).
func strText(v any) string {
	s, _ := v.(string)
	return s
}
