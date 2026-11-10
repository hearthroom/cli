package lorebook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/card"
)

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "card.json"), []byte(`{"formatVersion":1,"name":"Harbor"}`), 0o644)
	wb := filepath.Join(dir, SourceDir)
	os.MkdirAll(wb, 0o755)
	files := map[string]string{
		"_manifest.md": "# notes\nskipped\n",
		"curfew.md":    "---\nname: Harbor curfew\nkeywords: curfew, harbour\norder: 10\nscanDepth: 2\nmatchWholeWords: true\n---\nThe harbour closes at dusk.\n",
		"keeper.md":    "---\nid: keeper-01\nname: The keeper\nkeywords: [keeper, lamp]\nconstant: true\norder: 5\n---\nShe keeps the light.\n",
		"old.md":       "# scene-老爺爺洞府 — 跌崖得殘魂金手指\n\n> category: scene · keywords: 洞府,老爺爺,跌崖 · isConstant 建議:false\n\n## 觸發\n跌下山崖。\n",
		"nokeys.md":    "---\nname: Orphan\n---\nNobody can reach this.\n",
		"short.md":     "---\nname: Short\nkeywords: a, harbour\n---\nShort key.\n",
	}
	for name, body := range files {
		os.WriteFile(filepath.Join(wb, name), []byte(body), 0o644)
	}
	return dir
}

func TestParseAndBuild(t *testing.T) {
	dir := fixture(t)
	sources, err := ReadSources(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 5 {
		t.Fatalf("sources: %d", len(sources))
	}
	if sources[0].ID != "keeper-01" || sources[0].Order != 5 || !sources[0].Entry.Constant {
		t.Fatalf("order/id: %+v", sources[0])
	}
	var old Source
	for _, s := range sources {
		if s.File == "old.md" {
			old = s
		}
	}
	if old.Entry.Name != "scene-老爺爺洞府" || strings.Join(old.Entry.Keywords, ",") != "洞府,老爺爺,跌崖" || old.Entry.Category != "scene" || !strings.HasPrefix(old.Entry.Content, "## 觸發") {
		t.Fatalf("heading convention: %+v", old.Entry)
	}
	book, err := Build(dir, sources)
	if err != nil {
		t.Fatal(err)
	}
	if book.Name != "Harbor" || len(book.Entries) != 5 || book.Entries[0].Name != "The keeper" {
		t.Fatalf("book: %+v", book)
	}
	var curfew card.LorebookEntry
	for _, e := range book.Entries {
		if e.Name == "Harbor curfew" {
			curfew = e
		}
	}
	var opts map[string]any
	json.Unmarshal(curfew.MatchOptions, &opts)
	if opts["scanDepth"] != float64(2) || opts["matchWholeWords"] != true {
		t.Fatalf("matchOptions: %s", curfew.MatchOptions)
	}
	b, _ := os.ReadFile(filepath.Join(dir, card.LorebookFile))
	if !strings.Contains(string(b), `"keywords": [`) {
		t.Fatalf("lorebook.json shape: %s", b)
	}
	// Remote ids survive a rebuild when the name matches.
	var withID card.Lorebook
	json.Unmarshal(b, &withID)
	withID.Entries[0].ID = "remote-1"
	raw, _ := json.Marshal(withID)
	os.WriteFile(filepath.Join(dir, card.LorebookFile), raw, 0o644)
	book, _ = Build(dir, sources)
	if book.Entries[0].ID != "remote-1" {
		t.Fatalf("remote id lost: %+v", book.Entries[0])
	}
}

func TestCheck(t *testing.T) {
	dir := fixture(t)
	sources, _ := ReadSources(dir)
	res := Check(dir, sources)
	joined := ""
	for _, f := range res.Findings {
		joined += f.Level + ":" + f.Msg + "\n"
	}
	if !strings.Contains(joined, "missing; run") {
		t.Fatalf("expected missing lorebook.json: %s", joined)
	}
	Build(dir, sources)
	os.WriteFile(filepath.Join(dir, SourceDir, "curfew.md"), []byte("---\nname: Harbor curfew\nkeywords: curfew, harbour\n---\nChanged.\n"), 0o644)
	sources, _ = ReadSources(dir)
	res = Check(dir, sources)
	joined = ""
	for _, f := range res.Findings {
		joined += f.Level + ":" + f.Msg + "\n"
	}
	for _, want := range []string{"differs from lorebook.json", "no keywords and not constant", `keyword "harbour" fires 2 entries together`, `keyword "a" is very short`} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
	if res.Status != "ok" {
		t.Fatalf("warnings only: %s", res.Status)
	}
}

func TestSecondFrontmatterIsAnError(t *testing.T) {
	text := "---\nname: A\nkeywords: a1\n---\nA body.\n---\nname: B\nkeywords: [\"b1\"]\n---\nB body.\n"
	_, err := ParseSource("a.md", text)
	if err == nil || !strings.Contains(err.Error(), "line 6") || !strings.Contains(err.Error(), "one file holds one entry") {
		t.Fatalf("want a line-6 error, got %v", err)
	}
	// A Markdown rule inside the content is not a frontmatter block.
	s, err := ParseSource("rule.md", "---\nname: R\n---\nAbove.\n\n---\n\nBelow: not a key.\n")
	if err != nil || !strings.Contains(s.Entry.Content, "Below") {
		t.Fatalf("rule: %v %+v", err, s.Entry)
	}
}

func TestScalarQuotesAreStripped(t *testing.T) {
	s, err := ParseSource("q.md", "---\nid: 'q-1'\nname: \"甲条\"\ncategory: \"scene\"\n---\nBody.\n")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "q-1" || s.Entry.Name != "甲条" || s.Entry.Category != "scene" {
		t.Fatalf("quotes kept: %q %q %q", s.ID, s.Entry.Name, s.Entry.Category)
	}
}

func TestSubfoldersAreRead(t *testing.T) {
	dir := t.TempDir()
	wb := filepath.Join(dir, SourceDir)
	for name, body := range map[string]string{
		"top.md":           "---\nname: Top\nkeywords: top\n---\nTop.\n",
		"门派/少林.md":         "---\nkeywords: 少林\n---\nShaolin.\n",
		"门派/_note.md":      "notes\n",
		"_drafts/draft.md": "---\nname: Draft\n---\nDraft.\n",
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(wb, name)), 0o755)
		os.WriteFile(filepath.Join(wb, name), []byte(body), 0o644)
	}
	sources, err := ReadSources(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("sources: %+v", sources)
	}
	var sub Source
	for _, s := range sources {
		if s.Entry.Name == "少林" {
			sub = s
		}
	}
	if sub.File != "门派/少林.md" || sub.ID != "门派/少林" {
		t.Fatalf("subfolder entry: %+v", sub)
	}
}

func TestQuotedNameKeepsRemoteID(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, SourceDir), 0o755)
	os.WriteFile(filepath.Join(dir, SourceDir, "q.md"), []byte("---\nname: \"甲条\"\nkeywords: 甲\n---\nBody.\n"), 0o644)
	// Built by an older CLI that kept the quotes in the name.
	os.WriteFile(filepath.Join(dir, card.LorebookFile), []byte(`{"name":"Q","entries":[{"id":"remote-q","name":"\"甲条\"","content":"Body.","keywords":["甲"]}]}`), 0o644)
	sources, err := ReadSources(dir)
	if err != nil {
		t.Fatal(err)
	}
	book, err := Build(dir, sources)
	if err != nil {
		t.Fatal(err)
	}
	if book.Entries[0].Name != "甲条" || book.Entries[0].ID != "remote-q" {
		t.Fatalf("entry: %+v", book.Entries[0])
	}
}
