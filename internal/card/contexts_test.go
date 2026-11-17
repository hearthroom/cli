package card

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestContextFolderBuildAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{"card.json": `{"formatVersion":1,"name":"Synthetic","main":{"assembly":"client"}}`, "definition.md": "Synthetic instructions.", "contexts/settle.json": `{"id":"settle","name":"回合結算","preset":"contexts/settle.preset.json","worldbooks":["rules"],"history":"none"}`, "contexts/settle.preset.json": `{"schemaVersion":"hearthroom.st-preset.v1","st":{"prompts":[],"prompt_order":[]},"selection":{"profileId":0}}`, "lorebooks/rules.json": `{"id":"rules","name":"Rules","mainRecall":false,"entries":[{"name":"Rule","content":"Synthetic rule.","keywords":[]}]}`}
	for name, text := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	f, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := f.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(payload.Card)
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	contexts, ok := doc["contexts"].([]any)
	if !ok || len(contexts) != 1 {
		t.Fatalf("contexts not sent: %s", raw)
	}
	c := contexts[0].(map[string]any)
	if _, ok := c["preset"].(map[string]any); !ok {
		t.Fatalf("local preset path leaked: %v", c)
	}
	if doc["main"].(map[string]any)["assembly"] != "client" {
		t.Fatalf("main lost: %v", doc)
	}
	if len(f.Lorebooks) != 1 || f.Lorebooks["rules"].MainRecall == nil || *f.Lorebooks["rules"].MainRecall {
		t.Fatal("separate book binding lost")
	}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := again.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Digests()[SectionWorldbooks] == "" || payload.Digests()[SectionWorldbooks] != p2.Digests()[SectionWorldbooks] {
		t.Fatal("named book section digest lost or unstable")
	}
	if Digest(payload.Card) != Digest(p2.Card) {
		t.Fatal("private declaration round-trip changed payload")
	}
}
func TestContextPresetCannotEscapeFolder(t *testing.T) {
	for _, ref := range []string{"../outside.json", "/tmp/outside.json", "contexts/link.json"} {
		t.Run(ref, func(t *testing.T) {
			dir := t.TempDir()
			_ = os.WriteFile(filepath.Join(dir, "card.json"), []byte(`{"formatVersion":1,"name":"Synthetic"}`), 0600)
			_ = os.Mkdir(filepath.Join(dir, "contexts"), 0700)
			raw, _ := json.Marshal(map[string]any{"id": "test", "name": "Synthetic", "preset": ref})
			_ = os.WriteFile(filepath.Join(dir, "contexts", "test.json"), raw, 0600)
			outside := filepath.Join(t.TempDir(), "private.json")
			_ = os.WriteFile(outside, []byte(`{}`), 0600)
			_ = os.Symlink(outside, filepath.Join(dir, "contexts", "link.json"))
			if _, err := Load(dir); err == nil {
				t.Fatal("escaped preset accepted")
			}
		})
	}
}
