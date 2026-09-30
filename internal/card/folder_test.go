package card

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sample(t *testing.T) *Folder {
	t.Helper()
	dir := t.TempDir()
	f := &Folder{Dir: dir}
	f.Manifest = Manifest{
		FormatVersion: 1, Name: "Mira", Summary: "A lighthouse keeper", Tags: []string{"drama"}, Type: "story",
		TalkExample: []TalkExample{{RoleType: "user", Content: "Hi"}, {RoleType: "ai", Content: "Hello"}},
		Prologue:    []string{"You arrive at dusk."},
		Media:       Media{Portrait: "assets/mira.png"},
		Extra:       map[string]any{"x-custom": map[string]any{"k": "v"}},
	}
	f.Definition = "Mira keeps the light. See assets/map.png for the island."
	f.Welcome = "The lamp flickers. ![](assets/mira.png)"
	f.Alternates = []Opening{{Text: "Rain again."}}
	f.Lorebook = &Lorebook{Name: "Island", Entries: []LorebookEntry{{Name: "Lamp", Content: "Whale oil.", Keywords: []string{"lamp"}}}}
	f.Rules = &Rules{Rules: []DisplayRule{{Find: "a", Replace: "b", Enabled: true}}, MountLayer: "under"}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "mira.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestRoundTripKeepsFilesAndExtraKeys(t *testing.T) {
	f := sample(t)
	back, err := Load(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if back.Manifest.Name != "Mira" || back.Manifest.Extra["x-custom"] == nil {
		t.Fatalf("manifest lost data: %+v", back.Manifest)
	}
	if !strings.HasPrefix(back.Definition, "Mira keeps") || back.Alternates[0].File != "openings/alt-01.md" {
		t.Fatalf("text files: %q %+v", back.Definition, back.Alternates)
	}
	if back.Lorebook.Entries[0].Keywords[0] != "lamp" || !back.Rules.Rules[0].Enabled {
		t.Fatal("lorebook/rules lost")
	}
	raw, _ := os.ReadFile(filepath.Join(f.Dir, ManifestFile))
	if !strings.HasPrefix(string(raw), "{\n  \"formatVersion\": 1,\n  \"name\": \"Mira\"") {
		t.Fatalf("manifest ordering:\n%s", raw)
	}
	// Saving again is byte-identical.
	if err := back.Save(); err != nil {
		t.Fatal(err)
	}
	raw2, _ := os.ReadFile(filepath.Join(f.Dir, ManifestFile))
	if string(raw) != string(raw2) {
		t.Fatalf("manifest not stable:\n%s\n---\n%s", raw, raw2)
	}
}

func TestAssetRefsAndRewrite(t *testing.T) {
	f := sample(t)
	present, missing := f.AssetRefs()
	if len(present) != 1 || present[0] != "assets/mira.png" {
		t.Fatalf("present = %v", present)
	}
	if len(missing) != 1 || missing[0] != "assets/map.png" {
		t.Fatalf("missing = %v", missing)
	}
	out := Rewrite("see assets/mira.png. and assets/other.png", map[string]string{"assets/mira.png": "https://cdn/x.png"})
	if out != "see https://cdn/x.png. and assets/other.png" {
		t.Fatalf("rewrite = %q", out)
	}
}

func TestBuildProducesSectionsAndStableDigests(t *testing.T) {
	f := sample(t)
	urls := map[string]string{"assets/mira.png": "https://cdn/mira.png"}
	p, err := f.Build(urls)
	if err != nil {
		t.Fatal(err)
	}
	if p.Card["roleAvatar"] != "https://cdn/mira.png" || p.Card["roleName"] != "Mira" {
		t.Fatalf("card = %v", p.Card)
	}
	if !strings.Contains(p.Welcome["roleWelcome"].(string), "https://cdn/mira.png") {
		t.Fatalf("welcome not rewritten: %v", p.Welcome)
	}
	if p.Welcome["prologue"].([]string)[0] != "You arrive at dusk." || len(p.Welcome["alternates"].([]string)) != 1 {
		t.Fatalf("welcome = %v", p.Welcome)
	}
	if len(p.Worldbook["entries"].([]map[string]any)) != 1 || p.AuthorAsset["mountLayer"] != "under" {
		t.Fatalf("worldbook/asset = %v %v", p.Worldbook, p.AuthorAsset)
	}
	d1 := p.Digests()
	p2, _ := f.Build(urls)
	d2 := p2.Digests()
	for _, s := range Sections {
		if d1[s] == "" || d1[s] != d2[s] {
			t.Fatalf("digest %s unstable: %s vs %s", s, d1[s], d2[s])
		}
	}
	f.Welcome += "\nMore."
	d3, _ := f.Build(urls)
	if d3.Digests()[SectionWelcome] == d1[SectionWelcome] || d3.Digests()[SectionCard] != d1[SectionCard] {
		t.Fatal("editing welcome must change only the welcome digest")
	}
}

func TestBodyIgnoresPlaceholders(t *testing.T) {
	if Body("<!-- fill me -->\n") != "" || Body("  text ") != "text" {
		t.Fatal("Body")
	}
}

func TestFromRemoteRoundTrip(t *testing.T) {
	d := RemoteDetail{RoleID: "r1", RoleName: "Mira", RoleDesc: "s", RoleTag: []string{"a"}, RoleType: "story",
		RoleAvatar: "https://cdn/a.png", RoleBackground: "https://cdn/a.png", RoleBackgroundLandscape: "https://cdn/wide.png", RoleDetailDesc: "def", RoleWelcome: "hi",
		RoleWelcomeAlts: []string{"alt"}, RolePrologue: []string{"p"}, CreationMethod: "trial", CardMeta: json.RawMessage(`{"creator":"x"}`)}
	entries := []RemoteEntry{{EntryID: "e1", Name: "n", Content: "c", Keywords: []string{"k"}, IsEnabled: true, MatchOptions: json.RawMessage("null")}}
	asset := &RemoteAsset{Rules: []DisplayRule{{ID: "1", Find: "a", Replace: "b", Enabled: true}}, MountLayer: "under", PageMode: "classic", Version: 3}
	f := FromRemote(t.TempDir(), d, "wb1", "Book", entries, asset)
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	back, err := Load(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if back.State.RoleID != "r1" || back.State.Target != "trial" || back.State.LorebookID != "wb1" || back.State.AuthorAssetVersion != 3 {
		t.Fatalf("state = %+v", back.State)
	}
	if back.Manifest.Media.Background != "" || back.Manifest.Media.Portrait != "https://cdn/a.png" || back.Manifest.Media.BackgroundLandscape != "https://cdn/wide.png" {
		t.Fatalf("media = %+v", back.Manifest.Media)
	}
	if back.Lorebook.Entries[0].ID != "e1" || back.Lorebook.Entries[0].MatchOptions != nil {
		t.Fatalf("entry = %+v", back.Lorebook.Entries[0])
	}
	var meta map[string]any
	if err := json.Unmarshal(back.Manifest.CardMeta, &meta); err != nil || meta["creator"] != "x" {
		t.Fatalf("cardMeta = %s (%v)", back.Manifest.CardMeta, err)
	}
}

func TestLoadRejectsNewerFormat(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, ManifestFile), []byte(`{"formatVersion": 99, "name": "x"}`), 0o644)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("err = %v", err)
	}
	if _, err := Load(t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a card folder") {
		t.Fatalf("err = %v", err)
	}
}

// Agents read AGENTS.md when they start working in a folder, so a new card
// folder carries the pointer to the writing toolkit and the CLI loop.
func TestInitWritesTheAgentsGuide(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mira")
	if _, err := Init(dir, "Mira"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, AgentsFile))
	if err != nil {
		t.Fatalf("AGENTS.md missing: %v", err)
	}
	for _, want := range []string{"Mira", "hearthroom/skills", "using-hearthroom", "card push", "card render", "--allow-spend"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("AGENTS.md lacks %q", want)
		}
	}
}

// The stage prefers a landscape background on wide screens; the folder carries
// it as media.backgroundLandscape and push sends it as roleBackgroundLandscape.
func TestBuildSendsTheLandscapeBackground(t *testing.T) {
	f := sample(t)
	f.Manifest.Media.BackgroundLandscape = "assets/wide.png"
	if err := os.WriteFile(filepath.Join(f.Dir, "assets", "wide.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	urls := map[string]string{"assets/mira.png": "https://cdn/mira.png", "assets/wide.png": "https://cdn/wide.png"}
	p, err := f.Build(urls)
	if err != nil {
		t.Fatal(err)
	}
	if p.Card["roleBackgroundLandscape"] != "https://cdn/wide.png" {
		t.Fatalf("card = %v", p.Card)
	}
	if refs, _ := f.AssetRefs(); !contains(refs, "assets/wide.png") {
		t.Fatalf("landscape background not collected for upload: %v", refs)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
