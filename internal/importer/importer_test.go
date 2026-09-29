package importer

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/card"
)

// buildPNG makes a minimal PNG with the given tEXt chunks.
func buildPNG(texts map[string]string) []byte {
	var out bytes.Buffer
	out.Write(pngSignature)
	chunk := func(typ string, data []byte) {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(data)))
		out.Write(l[:])
		body := append([]byte(typ), data...)
		out.Write(body)
		var c [4]byte
		binary.BigEndian.PutUint32(c[:], crc32.ChecksumIEEE(body))
		out.Write(c[:])
	}
	chunk("IHDR", []byte{0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0})
	for k, v := range texts {
		chunk("tEXt", append(append([]byte(k), 0), []byte(v)...))
	}
	chunk("IDAT", []byte{0x78, 0x9c, 0x63, 0, 1, 0, 0, 5, 0, 1})
	chunk("IEND", nil)
	return out.Bytes()
}

func v3Card() map[string]any {
	return map[string]any{
		"spec": "chara_card_v3", "spec_version": "3.0",
		"data": map[string]any{
			"name": "Mira 灯守", "description": "Mira keeps the light.", "personality": "Quiet.", "scenario": "A storm.",
			"first_mes": "The lamp flickers.", "alternate_greetings": []string{"Rain again."}, "group_only_greetings": []string{"Group hello"},
			"mes_example":   "<START>\n{{user}}: Hi\n{{char}}: Hello there.\nStill here.",
			"creator_notes": "Be gentle.", "creator_notes_multilingual": map[string]string{"zh-TW": "溫柔一點", "en": "Be gentle (en)"},
			"system_prompt": "Write in present tense.", "post_history_instructions": "Never break character.",
			"tags":    []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"},
			"creator": "someone", "character_version": "1.2", "nickname": "Mi", "creation_date": 1700000000,
			"character_book": map[string]any{
				"name": "Island", "scan_depth": 4,
				"entries": []any{
					map[string]any{"keys": []string{"lamp"}, "secondary_keys": []string{"oil"}, "content": "@@activate\n@@depth 3\nWhale oil.", "enabled": true, "comment": "Lamp", "position": "before_char", "use_regex": true},
					map[string]any{"keys": `["harbor","港"]`, "content": strings.Repeat("Long line of text.\n", 400), "constant": false},
					map[string]any{"keys": []string{"empty"}, "content": "   "},
				},
			},
			"extensions": map[string]any{
				"regex_scripts": []any{
					map[string]any{"scriptName": "bold", "findRegex": "/\\*\\*(.+?)\\*\\*/g", "replaceString": "<b>$1</b>", "placement": []any{2}},
					map[string]any{"scriptName": "input-only", "findRegex": "x", "replaceString": "y", "placement": []any{1}},
				},
				"other": true,
			},
			"assets": []any{
				map[string]any{"type": "icon", "uri": "ccdefault:", "name": "main", "ext": "png"},
				map[string]any{"type": "emotion", "uri": "embeded://assets/happy.png", "name": "happy", "ext": "png"},
			},
		},
	}
}

func TestPNGV3Import(t *testing.T) {
	raw, _ := json.Marshal(v3Card())
	png := buildPNG(map[string]string{"chara": base64.StdEncoding.EncodeToString([]byte(`{"name":"old v2","first_mes":"x"}`)), "ccv3": base64.StdEncoding.EncodeToString(raw)})
	p, err := ParseCardFile(png)
	if err != nil {
		t.Fatal(err)
	}
	if p.Card.Spec != "chara_card_v3" || p.Card.Data.Name != "Mira 灯守" || len(p.Image) != len(png) {
		t.Fatalf("parsed: spec=%s name=%q image=%d", p.Card.Spec, p.Card.Data.Name, len(p.Image))
	}
	r := FromTavern(p, "zh-Hant")
	if r.Summary != "溫柔一點" {
		t.Fatalf("multilingual note: %q", r.Summary)
	}
	if !strings.Contains(r.Definition, "## Personality\nQuiet.") || !strings.Contains(r.Definition, "## Scenario\nA storm.") {
		t.Fatalf("definition: %q", r.Definition)
	}
	if len(r.Alternates) != 2 || len(r.Tags) != TagsMax || r.OutputContract != "Write in present tense." || r.Custom != "Never break character." {
		t.Fatalf("alts=%v tags=%d contract=%q custom=%q", r.Alternates, len(r.Tags), r.OutputContract, r.Custom)
	}
	if len(r.TalkExample) != 2 || r.TalkExample[1].Content != "Hello there.\nStill here." {
		t.Fatalf("talk example: %+v", r.TalkExample)
	}
	if r.CardMeta["creator"] != "someone" || r.CardMeta["creationDate"] != int64(1700000000) {
		t.Fatalf("cardMeta: %v", r.CardMeta)
	}
	if r.Rules == nil || len(r.Rules.Rules) != 1 || r.Rules.Rules[0].Name != "bold" || r.Rules.Format != "tavern" {
		t.Fatalf("rules: %+v", r.Rules)
	}
	if r.Lorebook == nil {
		t.Fatal("no lorebook")
	}
	e := r.Lorebook.Entries
	// Entry 1: decorators handled, regex keys wrapped; entry 2 split into
	// several with a shared group; entry 3 skipped (blank).
	if e[0].Name != "Lamp" || !e[0].Constant || e[0].Keywords[0] != "/lamp/i" || e[0].SecondaryKeywords[0] != "/oil/i" || e[0].Content != "Whale oil." {
		t.Fatalf("entry 1: %+v", e[0])
	}
	if e[0].MatchOptions["scanDepth"] != 4 {
		t.Fatalf("scanDepth inherited: %v", e[0].MatchOptions["scanDepth"])
	}
	split := 0
	for _, x := range e[1:] {
		if strings.HasPrefix(x.Name, "harbor (") {
			split++
			if x.Keywords[1] != "港" || x.MatchOptions["groupId"] == "" {
				t.Fatalf("split entry: %+v", x)
			}
		}
	}
	if split < 2 {
		t.Fatalf("expected split entries, got %d (total %d)", split, len(e))
	}
	joined := strings.Join(r.Notes, "\n")
	for _, want := range []string{"group-only", "tags beyond", "extension keys", "decorators", "@@depth", "insertion position", "split", "1 additional assets"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("notes missing %q:\n%s", want, joined)
		}
	}
	rep, err := WriteFolder(r, filepath.Join(t.TempDir(), "mira"), false)
	if err != nil {
		t.Fatal(err)
	}
	f, err := card.Load(rep.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.Manifest.Media.Portrait != "assets/portrait.png" || f.Lorebook == nil || f.Rules == nil || f.Rules.CardFormat != "tavern" {
		t.Fatalf("folder: %+v rules=%+v", f.Manifest, f.Rules)
	}
	if _, err := os.Stat(filepath.Join(rep.Dir, card.AgentsFile)); err != nil {
		t.Fatal("AGENTS.md missing after import")
	}
	if !slices.Contains(rep.Files, card.AgentsFile) {
		t.Fatalf("report does not list %s: %v", card.AgentsFile, rep.Files)
	}
	if _, err := os.Stat(filepath.Join(rep.Dir, "README.md")); err != nil {
		t.Fatal("creator notes README missing")
	}
	if f.Lorebook.Entries[2].Disabled {
		t.Fatal("enabled entry marked disabled")
	}
}

func TestJSONV2AndV1(t *testing.T) {
	v2 := []byte(`{"spec":"chara_card_v2","spec_version":"2.0","data":{"name":"Bo","first_mes":"Hi","mes_example":"free form text here"}}`)
	p, err := ParseCardFile(v2)
	if err != nil || p.Card.Spec != "chara_card_v2" {
		t.Fatalf("v2: %v %+v", err, p)
	}
	r := FromTavern(p, "en")
	if r.OutputContract != "free form text here" {
		t.Fatalf("mes_example fallback: %q", r.OutputContract)
	}
	v1 := []byte(`{"name":"Old","first_mes":"Hello","description":"desc"}`)
	p, err = ParseCardFile(v1)
	if err != nil || p.Card.Spec != "chara_card_v1" || p.Card.Data.Description != "desc" {
		t.Fatalf("v1: %v %+v", err, p)
	}
	if _, err := ParseCardFile([]byte(`{"foo":1}`)); err != ErrInvalid {
		t.Fatalf("non-card: %v", err)
	}
	if _, err := ParseCardFile([]byte(`not json`)); err != ErrInvalid {
		t.Fatalf("garbage: %v", err)
	}
}

func TestCHARXImport(t *testing.T) {
	c := v3Card()
	data := c["data"].(map[string]any)
	data["assets"] = []any{
		map[string]any{"type": "icon", "uri": "embeded://assets/icon.png", "name": "main", "ext": "png"},
		map[string]any{"type": "background", "uri": "embeded://assets/bg.jpg", "name": "main", "ext": "jpg"},
	}
	raw, _ := json.Marshal(c)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string][]byte{"card.json": raw, "assets/icon.png": []byte("ICON"), "assets/bg.jpg": []byte("BG"), "assets/other/sound.mp3": []byte("MP3")} {
		w, _ := zw.Create(name)
		w.Write(body)
	}
	zw.Close()
	p, err := ParseCardFile(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if string(p.Image) != "ICON" || p.ImageExt != "png" || string(p.Background) != "BG" || p.BackgroundExt != "jpg" {
		t.Fatalf("assets: %q %s %q %s", p.Image, p.ImageExt, p.Background, p.BackgroundExt)
	}
	rep, err := WriteFolder(FromTavern(p, "en"), filepath.Join(t.TempDir(), "x"), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(rep.Dir, "assets", "other", "sound.mp3")); err != nil {
		t.Fatal("bundled asset not copied")
	}
	f, _ := card.Load(rep.Dir)
	if f.Manifest.Media.Portrait != "assets/portrait.png" || f.Manifest.Media.Background != "assets/background.jpg" {
		t.Fatalf("media: %+v", f.Manifest.Media)
	}
}

func TestTruncatedPNGAndMissingChunk(t *testing.T) {
	png := buildPNG(map[string]string{"ccv3": "eyJuYW1lIjoieCJ9"})
	// Cut inside the ccv3 chunk (IDAT and IEND together are 34 bytes).
	if _, err := ParseCardFile(png[:len(png)-40]); err != ErrPNGTruncated {
		t.Fatalf("truncated: %v", err)
	}
	// A hostile length field.
	bad := append([]byte{}, pngSignature...)
	bad = append(bad, 0xff, 0xff, 0xff, 0xff, 'I', 'H', 'D', 'R')
	if _, err := ParseCardFile(bad); err != ErrPNGTruncated {
		t.Fatalf("hostile length: %v", err)
	}
	if _, err := ParseCardFile(buildPNG(nil)); err != ErrNoMetadata {
		t.Fatalf("no chunk: %v", err)
	}
}

func TestMMDSet(t *testing.T) {
	rulesFile := []byte(`{"chatVersion":"1","pageDepth":1,"statusbar":"《狀1》","beginning":"Welcome, traveler.","regex_scripts":[{"id":1,"scriptName":"status","findRegex":"《狀1》","replaceString":"<div>hp</div>"}]}`)
	bookFile := []byte(`{"entries":{"0":{"uid":0,"key":"[\"town\",\"鎮\"]","keysecondary":"","comment":"Town","content":"A small town.","disable":false,"order":5},"1":{"uid":1,"key":"x","content":"","disable":true}}}`)
	defFile := []byte("She is the keeper of the lighthouse.\nShe speaks softly.")
	a, err := ClassifyMMDFile("rules.json", rulesFile)
	if err != nil || a.Part != PartRules || a.Welcome != "Welcome, traveler." || !a.Set.Lowered || a.Set.PageMode != "sandbox" {
		t.Fatalf("rules: %v %+v", err, a)
	}
	b, err := ClassifyMMDFile("book.json", bookFile)
	if err != nil || b.Part != PartBook || len(b.Book.Entries) != 2 || b.Book.Entries[0].Keys[1] != "鎮" {
		t.Fatalf("book: %v %+v", err, b)
	}
	c, err := ClassifyMMDFile("Mira.txt", defFile)
	if err != nil || c.Part != PartDefinition {
		t.Fatalf("def: %v %+v", err, c)
	}
	r := MergeMMDFiles([]*MMDFile{a, b, c}, "zh-Hant")
	if r.Name != "Mira" || r.Welcome != "Welcome, traveler." || !strings.HasPrefix(r.Definition, "She is") {
		t.Fatalf("merged: %+v", r)
	}
	if r.Lorebook == nil || len(r.Lorebook.Entries) != 1 || r.Rules == nil || r.Rules.Statusbar != "《狀1》" {
		t.Fatalf("merged parts: %+v %+v", r.Lorebook, r.Rules)
	}
	rep, err := WriteFolder(r, filepath.Join(t.TempDir(), "mira"), false)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := card.Load(rep.Dir)
	if f.Rules.MountLayer != "under" || f.Rules.PageMode != "sandbox" || f.Rules.MountTrigger != "《狀1》" || f.Rules.CardFormat != "" {
		t.Fatalf("rules.json: %+v", f.Rules)
	}
	// Partial set reports the missing part.
	partial := MergeMMDFiles([]*MMDFile{c}, "en")
	if len(partial.Notes) != 2 {
		t.Fatalf("partial notes: %v", partial.Notes)
	}
	// Meimo "export regex" list and API envelope.
	l, err := ClassifyMMDFile("export.json", []byte(`{"code":0,"data":[{"regex":"《美1》","content":"<style>a{}</style>","name":"style"}]}`))
	if err != nil || l.Part != PartRules || l.Set.Rules[0].Find != "《美1》" {
		t.Fatalf("meimo list: %v %+v", err, l)
	}
	// Errors.
	if _, err := ClassifyMMDFile("e.json", []byte("  ")); err != ErrMMDEmpty {
		t.Fatalf("empty: %v", err)
	}
	if _, err := ClassifyMMDFile("e.json", []byte("{broken")); err != ErrMMDInvalidJSON {
		t.Fatalf("broken: %v", err)
	}
	if _, err := ClassifyMMDFile("e.json", []byte(`{"unrelated":1}`)); err != ErrMMDUnknown {
		t.Fatalf("unknown: %v", err)
	}
}

func TestDecoratorsAndKeys(t *testing.T) {
	d := ParseDecorators("@@activate\n@@@dont_activate\n@@position after\n@@@additional_keys a, b\nBody")
	if !d.Activate || d.DontActivate || len(d.AdditionalKeys) != 2 || d.Content != "Body" || len(d.Unsupported) != 1 || d.Unsupported[0] != "position" {
		t.Fatalf("%+v", d)
	}
	if RegexKey("/x/", true) != "/x/" || RegexKey("x", false) != "/x/i" || RegexKey("x", true) != "/x/" {
		t.Fatal("RegexKey")
	}
	if got := keyList("a, b，c"); len(got) != 3 {
		t.Fatalf("keyList csv: %v", got)
	}
	parts := SplitEntryContent(strings.Repeat("x", 100), 50)
	if len(parts) != 10 {
		t.Fatalf("split: %d", len(parts))
	}
}
