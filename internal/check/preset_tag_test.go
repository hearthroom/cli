package check

import "testing"

func TestPresetTagInfoNoticeDoesNotFailCheck(t *testing.T) {
	dir := write(t, t.TempDir(), map[string]string{
		"card.json":     `{"formatVersion":1,"name":"Mira","language":"zh-Hant","tags":["自訂","roleplay"]}`,
		"README.md":     "uiRole: assist\n",
		"definition.md": "Persona",
		"welcome.md":    "Hello",
	})
	r, err := Card(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Errors() != 0 || r.Status != "ok" {
		t.Fatalf("status=%q errors=%d: %v", r.Status, r.Errors(), msgs(r))
	}
	if !has(msgs(r), "info:no preset category or play-style tag in tags: public publication requires at least one platform preset tag (custom tags do not count); run `hearthroom tags --preset --lang zh-Hant` and add a display name (not its key) to card.json") {
		t.Fatalf("missing or mismatched info finding: %v", msgs(r))
	}

	withPreset := write(t, t.TempDir(), map[string]string{
		"card.json":     `{"formatVersion":1,"name":"Mira","language":"zh-Hant","tags":["角色扮演"]}`,
		"README.md":     "uiRole: assist\n",
		"definition.md": "Persona",
		"welcome.md":    "Hello",
	})
	r2, err := Card(withPreset)
	if err != nil {
		t.Fatal(err)
	}
	if has(msgs(r2), "no preset category or play-style tag") {
		t.Fatalf("unexpected preset tag finding when preset tag is present: %v", msgs(r2))
	}
}
