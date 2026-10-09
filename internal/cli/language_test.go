package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The provider files an undeclared card as zh-Hant, so the commands that write
// card.json take the language explicitly instead of guessing one.
func TestInitAndImportRequireTheCardLanguage(t *testing.T) {
	src := filepath.Join(t.TempDir(), "card.json")
	if err := os.WriteFile(src, []byte(`{"spec":"chara_card_v2","data":{"name":"Mira","first_mes":"hi"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"card", "init", filepath.Join(t.TempDir(), "a")},
		{"card", "init", filepath.Join(t.TempDir(), "b"), "--language", "zh"},
		{"card", "import", src, "--out", filepath.Join(t.TempDir(), "c")},
		{"card", "import", src, "--out", filepath.Join(t.TempDir(), "d"), "--language", "zh-TW"},
	} {
		root := NewRoot(BuildInfo{})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "language") {
			t.Fatalf("%v: err = %v, want a language error", args, err)
		}
	}
	dir := filepath.Join(t.TempDir(), "ok")
	root := NewRoot(BuildInfo{})
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"card", "init", dir, "--language", "zh-Hans"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "card.json"))
	if !strings.Contains(string(b), `"language": "zh-Hans"`) {
		t.Fatalf("card.json = %s", b)
	}
}
