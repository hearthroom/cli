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
