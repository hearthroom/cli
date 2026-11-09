package check

import (
	"strings"
	"testing"
)

// card check reports broken or oversized variables before a push, and the
// companion files that would be ignored without variables.json.
func TestCheckVariables(t *testing.T) {
	base := map[string]string{
		"card.json":     `{"formatVersion":1,"name":"Mira","language":"en"}`,
		"definition.md": "Mira.",
		"welcome.md":    "Hello.",
	}
	with := func(extra map[string]string) []string {
		files := map[string]string{}
		for k, v := range base {
			files[k] = v
		}
		for k, v := range extra {
			files[k] = v
		}
		r, err := Card(write(t, t.TempDir(), files))
		if err != nil {
			t.Fatal(err)
		}
		return msgs(r)
	}
	if got := with(map[string]string{"variables.json": "{\n\"convention\": \"mvu\",\n\"data\": {\"a\": [1, \"r\"],}\n}"}); !has(got, "error:", "near line 3") {
		t.Fatalf("invalid JSON: %v", got)
	}
	if got := with(map[string]string{"variables.json": `{"convention":"plain","data":{"a":1}}`, "panel.hbs": strings.Repeat("x", 32769)}); !has(got, "error:", "panel.hbs is 32769 bytes") {
		t.Fatalf("panel size: %v", got)
	}
	if got := with(map[string]string{"variables.md": "Rules."}); !has(got, "warning:", "variables.md is not sent without variables.json") {
		t.Fatalf("orphan rules: %v", got)
	}
	if got := with(map[string]string{"variables.json": `{"convention":"mvu","data":{"a":[1,"r"]}}`, "variables.md": "Rules."}); has(got, "variables") {
		t.Fatalf("valid variables reported: %v", got)
	}
}
