// Package card defines the on-disk card folder (formatVersion 1) and converts
// it to and from the provider's document shapes. See docs/card-folder.md.
package card

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// FormatVersion is the folder format this build reads and writes.
const FormatVersion = 1

// File names inside a card folder.
const (
	ManifestFile   = "card.json"
	DefinitionFile = "definition.md"
	WelcomeFile    = "welcome.md"
	OpeningsDir    = "openings"
	LorebookFile   = "lorebook.json"
	RulesFile      = "rules.json"
	AssetsDir      = "assets"
	StateDir       = ".hearthroom"
	StateFile      = "state.json"
)

// Manifest is card.json. Short structured fields live here; long text lives
// in Markdown files. Unknown keys are kept in Extra and written back.
type Manifest struct {
	FormatVersion      int             `json:"formatVersion"`
	Name               string          `json:"name"`
	Summary            string          `json:"summary,omitempty"`
	Tags               []string        `json:"tags,omitempty"`
	Type               string          `json:"type,omitempty"`
	Sex                string          `json:"sex,omitempty"`
	PlayerName         string          `json:"playerName,omitempty"`
	Nickname           string          `json:"nickname,omitempty"`
	OutputContract     string          `json:"outputContract,omitempty"`
	CustomInstructions string          `json:"customInstructions,omitempty"`
	TalkExample        []TalkExample   `json:"talkExample,omitempty"`
	Prologue           []string        `json:"prologue,omitempty"`
	CardMeta           json.RawMessage `json:"cardMeta,omitempty"`
	Media              Media           `json:"media"`
	Language           string          `json:"language,omitempty"`
	Extra              map[string]any  `json:"-"`
}

// TalkExample is one example line; roleType is `user` or `ai`.
type TalkExample struct {
	RoleType string `json:"roleType"`
	Content  string `json:"content"`
}

// Media references either a relative path under assets/ or an absolute URL.
type Media struct {
	Portrait   string `json:"portrait,omitempty"`
	Background string `json:"background,omitempty"` // portrait (9:16) background, the baseline
	// BackgroundLandscape is the optional landscape (16:9) background the chat
	// page prefers on wide screens; it falls back to Background when empty.
	BackgroundLandscape string `json:"backgroundLandscape,omitempty"`
	// Folder is the media-library folder the card's assets/ tree is uploaded
	// into: assets/<path> is served at <libraryPrefix>/<Folder>/<path>. Empty
	// means the CLI picks one from the card name on the first push and keeps
	// it in state. Setting it moves the card's files there on the next push,
	// and is how two cards share one folder on purpose.
	Folder string `json:"folder,omitempty"`
}

// Lorebook is lorebook.json.
type Lorebook struct {
	ID          string          `json:"id,omitempty"` // remote id after pull/push to an owned card
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Format      string          `json:"format,omitempty"`
	Entries     []LorebookEntry `json:"entries"`
}

// LorebookEntry mirrors the provider's entry fields.
type LorebookEntry struct {
	ID                string          `json:"id,omitempty"` // remote entryId when known
	Name              string          `json:"name"`
	Content           string          `json:"content"`
	Category          string          `json:"category,omitempty"`
	Keywords          []string        `json:"keywords"`
	SecondaryKeywords []string        `json:"secondaryKeywords,omitempty"`
	Constant          bool            `json:"constant,omitempty"`
	Disabled          bool            `json:"disabled,omitempty"`
	TriggerRegion     string          `json:"triggerRegion,omitempty"`
	MatchOptions      json.RawMessage `json:"matchOptions,omitempty"`
}

// Rules is rules.json (the provider's "author asset": display rules).
type Rules struct {
	Rules        []DisplayRule `json:"rules"`
	MountTrigger string        `json:"mountTrigger,omitempty"`
	MountLayer   string        `json:"mountLayer,omitempty"` // under | over | cover (default over)
	PageMode     string        `json:"pageMode,omitempty"`   // classic | immersive | sandbox
	CardFormat   string        `json:"cardFormat,omitempty"` // "" (MMD-style) | tavern: how <style> in rules is scoped
}

// DisplayRule is one find/replace rule.
type DisplayRule struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Find    string `json:"find"`
	Replace string `json:"replace"`
	Enabled bool   `json:"enabled"`
}

// Folder is a loaded card folder.
type Folder struct {
	Dir        string
	Manifest   Manifest
	Definition string
	Welcome    string
	Alternates []Opening
	Lorebook   *Lorebook
	Rules      *Rules
	State      State
}

// Opening is one alternate opening file.
type Opening struct {
	File string // e.g. openings/alt-01.md
	Text string
}

// State is .hearthroom/state.json: what the folder is synced to.
type State struct {
	Target             string            `json:"target,omitempty"` // "trial" or "owned"
	RoleID             string            `json:"roleId,omitempty"`
	TrialKey           string            `json:"trialKey,omitempty"`
	API                string            `json:"api,omitempty"`
	Sections           map[string]string `json:"sections,omitempty"` // server hashes by section
	LocalHashes        map[string]string `json:"localHashes,omitempty"`
	Assets             map[string]Asset  `json:"assets,omitempty"` // by relative path
	AuthorAssetVersion int64             `json:"authorAssetVersion,omitempty"`
	LorebookID         string            `json:"lorebookId,omitempty"`
	LorebookEntryIDs   []string          `json:"lorebookEntryIds,omitempty"` // entries the CLI created or pulled; only these may be deleted remotely
	ConversationID     string            `json:"conversationId,omitempty"`
	AssetFolder        string            `json:"assetFolder,omitempty"` // media-library folder the assets were last uploaded into
}

// Asset records an uploaded file.
type Asset struct {
	SHA256   string `json:"sha256"`
	URL      string `json:"url"`
	FileName string `json:"fileName,omitempty"`
}

// ErrNotCardFolder is returned when card.json is missing.
var ErrNotCardFolder = errors.New("not a card folder (no card.json)")

// Init creates a minimal folder. It refuses to overwrite an existing card.json.
func Init(dir, name string) (*Folder, error) {
	if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err == nil {
		return nil, fmt.Errorf("%s already contains %s", dir, ManifestFile)
	}
	if err := os.MkdirAll(filepath.Join(dir, AssetsDir), 0o755); err != nil {
		return nil, err
	}
	f := &Folder{Dir: dir, Manifest: Manifest{FormatVersion: FormatVersion, Name: name, Type: "story"}}
	f.Welcome = ""
	f.Definition = ""
	if err := f.Save(); err != nil {
		return nil, err
	}
	// Placeholder files so the author sees where text goes.
	_ = os.WriteFile(filepath.Join(dir, DefinitionFile), []byte("<!-- Private definition of the character: who they are, how they speak, what the story is about. -->\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, WelcomeFile), []byte("<!-- Opening message the character sends first. -->\n"), 0o644)
	if _, err := WriteAgentsGuide(dir, name); err != nil {
		return nil, err
	}
	return Load(dir)
}

// Load reads a card folder.
func Load(dir string) (*Folder, error) {
	raw, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s: %w", dir, ErrNotCardFolder)
		}
		return nil, err
	}
	f := &Folder{Dir: dir}
	if err := f.Manifest.UnmarshalJSON(raw); err != nil {
		return nil, fmt.Errorf("%s: %w", ManifestFile, err)
	}
	if f.Manifest.FormatVersion == 0 {
		return nil, fmt.Errorf("%s: missing formatVersion", ManifestFile)
	}
	if f.Manifest.FormatVersion > FormatVersion {
		return nil, fmt.Errorf("%s: formatVersion %d is newer than this build supports (%d); upgrade hearthroom", ManifestFile, f.Manifest.FormatVersion, FormatVersion)
	}
	f.Definition, err = readText(filepath.Join(dir, DefinitionFile))
	if err != nil {
		return nil, err
	}
	f.Welcome, err = readText(filepath.Join(dir, WelcomeFile))
	if err != nil {
		return nil, err
	}
	f.Alternates, err = readOpenings(dir)
	if err != nil {
		return nil, err
	}
	if b, err := os.ReadFile(filepath.Join(dir, LorebookFile)); err == nil {
		var lb Lorebook
		if err := json.Unmarshal(b, &lb); err != nil {
			return nil, fmt.Errorf("%s: %w", LorebookFile, err)
		}
		f.Lorebook = &lb
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if b, err := os.ReadFile(filepath.Join(dir, RulesFile)); err == nil {
		var r Rules
		if err := json.Unmarshal(b, &r); err != nil {
			return nil, fmt.Errorf("%s: %w", RulesFile, err)
		}
		f.Rules = &r
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if b, err := os.ReadFile(filepath.Join(dir, StateDir, StateFile)); err == nil {
		if err := json.Unmarshal(b, &f.State); err != nil {
			return nil, fmt.Errorf("%s: %w", StateFile, err)
		}
	}
	return f, nil
}

// Save writes every file of the folder (state included).
func (f *Folder) Save() error {
	if err := os.MkdirAll(f.Dir, 0o755); err != nil {
		return err
	}
	if f.Manifest.FormatVersion == 0 {
		f.Manifest.FormatVersion = FormatVersion
	}
	raw, err := f.Manifest.MarshalJSON()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(f.Dir, ManifestFile), raw, 0o644); err != nil {
		return err
	}
	if err := writeText(filepath.Join(f.Dir, DefinitionFile), f.Definition); err != nil {
		return err
	}
	if err := writeText(filepath.Join(f.Dir, WelcomeFile), f.Welcome); err != nil {
		return err
	}
	if len(f.Alternates) > 0 {
		if err := os.MkdirAll(filepath.Join(f.Dir, OpeningsDir), 0o755); err != nil {
			return err
		}
		for i := range f.Alternates {
			if f.Alternates[i].File == "" {
				f.Alternates[i].File = filepath.ToSlash(filepath.Join(OpeningsDir, fmt.Sprintf("alt-%02d.md", i+1)))
			}
			if err := writeText(filepath.Join(f.Dir, filepath.FromSlash(f.Alternates[i].File)), f.Alternates[i].Text); err != nil {
				return err
			}
		}
	}
	if f.Lorebook != nil {
		if err := writeJSONFile(filepath.Join(f.Dir, LorebookFile), f.Lorebook); err != nil {
			return err
		}
	}
	if f.Rules != nil {
		if err := writeJSONFile(filepath.Join(f.Dir, RulesFile), f.Rules); err != nil {
			return err
		}
	}
	return f.SaveState()
}

// SaveState writes only .hearthroom/state.json.
func (f *Folder) SaveState() error {
	if err := os.MkdirAll(filepath.Join(f.Dir, StateDir), 0o755); err != nil {
		return err
	}
	return writeJSONFile(filepath.Join(f.Dir, StateDir, StateFile), f.State)
}

// MarshalJSON writes known fields plus Extra, keeping formatVersion first.
func (m Manifest) MarshalJSON() ([]byte, error) {
	type plain Manifest
	raw, err := json.Marshal(plain(m))
	if err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	for k, v := range m.Extra {
		if _, taken := obj[k]; taken {
			continue
		}
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		obj[k] = b
	}
	order := []string{"formatVersion", "name", "summary", "tags", "type", "sex", "playerName", "nickname", "language",
		"outputContract", "customInstructions", "talkExample", "prologue", "cardMeta", "media"}
	seen := map[string]bool{}
	var b strings.Builder
	b.WriteString("{\n")
	first := true
	write := func(k string) {
		v, ok := obj[k]
		if !ok || seen[k] {
			return
		}
		seen[k] = true
		if !first {
			b.WriteString(",\n")
		}
		first = false
		kb, _ := json.Marshal(k)
		b.WriteString("  ")
		b.Write(kb)
		b.WriteString(": ")
		b.Write(indent(v))
	}
	for _, k := range order {
		write(k)
	}
	rest := make([]string, 0, len(obj))
	for k := range obj {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		write(k)
	}
	b.WriteString("\n}\n")
	return []byte(b.String()), nil
}

// UnmarshalJSON reads known fields and stashes the rest in Extra.
func (m *Manifest) UnmarshalJSON(raw []byte) error {
	type plain Manifest
	var p plain
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	var all map[string]any
	if err := json.Unmarshal(raw, &all); err != nil {
		return err
	}
	known := map[string]bool{}
	for _, k := range []string{"formatVersion", "name", "summary", "tags", "type", "sex", "playerName", "nickname", "language",
		"outputContract", "customInstructions", "talkExample", "prologue", "cardMeta", "media"} {
		known[k] = true
	}
	for k, v := range all {
		if !known[k] {
			if p.Extra == nil {
				p.Extra = map[string]any{}
			}
			p.Extra[k] = v
		}
	}
	*m = Manifest(p)
	return nil
}

func indent(v json.RawMessage) []byte {
	var buf strings.Builder
	dec := json.NewDecoder(strings.NewReader(string(v)))
	dec.UseNumber()
	var any1 any
	if err := dec.Decode(&any1); err != nil {
		return v
	}
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("  ", "  ")
	if err := enc.Encode(any1); err != nil {
		return v
	}
	return []byte(strings.TrimRight(buf.String(), "\n"))
}

func readText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	if !utf8.Valid(b) {
		return "", fmt.Errorf("%s: not valid UTF-8", filepath.Base(path))
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n"), nil
}

func writeText(path, text string) error {
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

func writeJSONFile(path string, v any) error {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(buf.String()), 0o644)
}

func readOpenings(dir string) ([]Opening, error) {
	entries, err := os.ReadDir(filepath.Join(dir, OpeningsDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Opening
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		text, err := readText(filepath.Join(dir, OpeningsDir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, Opening{File: OpeningsDir + "/" + e.Name(), Text: text})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out, nil
}

// Text helpers: the API wants trimmed bodies; a Markdown comment placeholder
// counts as empty.
var placeholder = regexp.MustCompile(`(?s)^\s*<!--.*?-->\s*$`)

// Body returns text ready for the API: trimmed, placeholders removed.
func Body(text string) string {
	t := strings.TrimSpace(text)
	if placeholder.MatchString(t) {
		return ""
	}
	return t
}

// Digest returns "sha256:<hex>" of the canonical JSON of v.
func Digest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// FileDigest hashes a file's bytes.
func FileDigest(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// assetRef matches relative asset references in text and JSON.
var assetRef = regexp.MustCompile("assets/[^\\s\"'`<>()\\\\]+")

// refTarget classifies one matched reference: a file, or a directory (a
// trailing "/", or a path holding a $1 / ${x} / {{…}} placeholder, which
// stands for whatever files that directory holds).
func refTarget(ref string) (p string, dir bool) {
	if i := strings.IndexAny(ref, "${"); i >= 0 {
		j := strings.LastIndex(ref[:i], "/")
		return ref[:j+1], true
	}
	return ref, strings.HasSuffix(ref, "/")
}

func (f *Folder) scanRefs(add func(string)) {
	scan := func(text string) {
		for _, m := range assetRef.FindAllString(text, -1) {
			add(strings.TrimRight(m, ".,;:!?"))
		}
	}
	scan(f.Definition)
	scan(f.Welcome)
	for _, o := range f.Alternates {
		scan(o.Text)
	}
	if f.Lorebook != nil {
		for _, e := range f.Lorebook.Entries {
			scan(e.Content)
		}
	}
	if f.Rules != nil {
		for _, r := range f.Rules.Rules {
			scan(r.Replace)
		}
	}
}

// AssetDirs lists the referenced directories that exist on disk, e.g.
// "assets/art/expr/". Every file under one is uploaded, and the directory is
// rewritten to its served URL so card code can append file names at runtime.
func (f *Folder) AssetDirs() []string {
	seen := map[string]bool{}
	var out []string
	f.scanRefs(func(ref string) {
		p, dir := refTarget(ref)
		if !dir || seen[p] {
			return
		}
		seen[p] = true
		if st, err := os.Stat(filepath.Join(f.Dir, filepath.FromSlash(p))); err == nil && st.IsDir() {
			out = append(out, p)
		}
	})
	sort.Strings(out)
	return out
}

// AssetRefs lists every distinct relative asset path referenced by the folder,
// in stable order, including every file under a referenced directory. Only
// paths that exist on disk are returned; missing ones are reported separately.
func (f *Folder) AssetRefs() (present, missing []string) {
	seen := map[string]bool{}
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] || !strings.HasPrefix(p, AssetsDir+"/") {
			return
		}
		seen[p] = true
	}
	add(f.Manifest.Media.Portrait)
	add(f.Manifest.Media.Background)
	add(f.Manifest.Media.BackgroundLandscape)
	f.scanRefs(func(ref string) {
		p, _ := refTarget(ref)
		add(p)
	})
	refs := make([]string, 0, len(seen))
	for p := range seen {
		refs = append(refs, p)
	}
	for _, p := range refs {
		full := filepath.Join(f.Dir, filepath.FromSlash(p))
		st, err := os.Stat(full)
		switch {
		case err != nil:
			missing = append(missing, p)
		case st.IsDir():
			_ = filepath.WalkDir(full, func(fp string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if strings.HasPrefix(d.Name(), ".") {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if !d.IsDir() {
					rel, _ := filepath.Rel(f.Dir, fp)
					if r := filepath.ToSlash(rel); !seen[r] {
						seen[r] = true
						present = append(present, r)
					}
				}
				return nil
			})
		case strings.HasSuffix(p, "/"):
			missing = append(missing, p)
		default:
			present = append(present, p)
		}
	}
	sort.Strings(present)
	sort.Strings(missing)
	return present, missing
}

var unsafeFolderRune = regexp.MustCompile(`[\\/\p{Cc}\p{Cf}]+`)

func cleanFolderSegment(s string) string {
	s = unsafeFolderRune.ReplaceAllString(strings.TrimSpace(s), "-")
	s = strings.Trim(s, "- ")
	if s == "." || s == ".." {
		return ""
	}
	if r := []rune(s); len(r) > 80 {
		s = strings.TrimRight(string(r[:80]), "- ")
	}
	return s
}

// LibraryFolder returns the media-library folder for this card: media.folder
// when set (explicit=true), else one derived from the card name, else from the
// local folder name. It does not consult state; callers keep a chosen folder.
func (f *Folder) LibraryFolder() (folder string, explicit bool) {
	if raw := strings.TrimSpace(f.Manifest.Media.Folder); raw != "" {
		var parts []string
		for _, seg := range strings.Split(strings.ReplaceAll(raw, "\\", "/"), "/") {
			if c := cleanFolderSegment(seg); c != "" {
				parts = append(parts, c)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "/"), true
		}
	}
	for _, s := range []string{f.Manifest.Name, filepath.Base(f.Dir)} {
		if c := cleanFolderSegment(s); c != "" {
			return c, false
		}
	}
	return "card", false
}

// Rewrite replaces asset references in text using the map (relative path → URL).
func Rewrite(text string, urls map[string]string) string {
	if len(urls) == 0 {
		return text
	}
	return assetRef.ReplaceAllStringFunc(text, func(m string) string {
		trail := ""
		core := m
		for len(core) > 0 && strings.ContainsRune(".,;:!?", rune(core[len(core)-1])) {
			trail = string(core[len(core)-1]) + trail
			core = core[:len(core)-1]
		}
		if u, ok := urls[core]; ok {
			return u + trail
		}
		// A directory reference followed by a runtime part ($1, ${id}, …).
		if dir, ok := refTarget(core); ok {
			if u, ok := urls[dir]; ok {
				return u + core[len(dir):] + trail
			}
		}
		return m
	})
}
