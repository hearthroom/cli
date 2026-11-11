package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A Fengyue export has no spec/data/first_mes, so it used to fall through to the MMD
// classifier and fail as "not recognised". Each export also renames its keys at random;
// the title key alone was seen as title, ttl, name, nm and app_name.
func TestImportFengyueExport(t *testing.T) {
	b, err := os.ReadFile("../importer/testdata/fengyue-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []struct {
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}
	}
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatal(err)
	}
	for _, c := range file.Cases[:7] {
		t.Run(c.Name, func(t *testing.T) {
			src := filepath.Join(t.TempDir(), "export.json")
			if err := os.WriteFile(src, c.Input, 0o644); err != nil {
				t.Fatal(err)
			}
			for _, extra := range [][]string{nil, {"--mmd"}} {
				dir := filepath.Join(t.TempDir(), "card")
				root := NewRoot(BuildInfo{})
				var out bytes.Buffer
				root.SetOut(&out)
				root.SetErr(&out)
				root.SetArgs(append([]string{"card", "import", src, "--out", dir, "--language", "zh-Hans"}, extra...))
				if err := root.Execute(); err != nil {
					t.Fatalf("%v: %v\n%s", extra, err, out.String())
				}
				manifest, _ := os.ReadFile(filepath.Join(dir, "card.json"))
				if !strings.Contains(string(manifest), `"name": "xB_au9VW"`) || !strings.Contains(string(manifest), "TEST-OPENING") {
					t.Fatalf("%v: card.json = %s", extra, manifest)
				}
				rules, _ := os.ReadFile(filepath.Join(dir, "rules.json"))
				if strings.Contains(string(rules), ".st-bar{color:#c33") || !strings.Contains(string(rules), `"cardCssDialect": "fengyue-v2"`) {
					t.Fatalf("%v: rules.json = %s", extra, rules)
				}
				css, err := os.ReadFile(filepath.Join(dir, "card.css"))
				if err != nil || !strings.Contains(string(css), ".st-bar{color:#c33") {
					t.Fatalf("%v: card.css = %q, %v", extra, css, err)
				}
			}
		})
	}
}
