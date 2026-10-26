package check

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/importer"
)

func TestLorebookRegexWarnings(t *testing.T) {
	raw, err := os.ReadFile("testdata/lorebook-regex-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Key  string
		Warn bool
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Key, func(t *testing.T) {
			lb, _ := json.Marshal(map[string]any{"entries": []any{map[string]any{"name": "Harbor gate", "content": "An original setting.", "keywords": []string{tc.Key}, "secondaryKeywords": []string{tc.Key}}}})
			dir := write(t, t.TempDir(), map[string]string{"card.json": `{"language":"en"}`, "lorebook.json": string(lb)})
			before, _ := os.ReadFile(dir + "/lorebook.json")
			result, err := Card(dir)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, f := range result.Findings {
				if strings.HasPrefix(f.Where, "lorebook.json entries[") {
					count++
					if f.Level != "warning" {
						t.Fatalf("not advisory: %+v", f)
					}
				}
			}
			want := 0
			if tc.Warn {
				want = 2
			}
			if count != want {
				t.Fatalf("got %d Lorebook warnings, want %d: %+v", count, want, result.Findings)
			}
			if result.Status != "ok" {
				t.Fatalf("warning blocks check: %+v", result)
			}
			after, _ := os.ReadFile(dir + "/lorebook.json")
			if string(after) != string(before) {
				t.Fatal("check changed local data")
			}
		})
	}
}

func TestImportedRegexKeysDoNotWarn(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			source := fmt.Sprintf(`{"spec":"chara_card_v3","data":{"name":"Harbor gate","character_book":{"entries":[{"keys":["20[6-7][0-9]"],"secondary_keys":["^dock$"],"content":"An original setting.","use_regex":%t}]}}}`, enabled)
			parsed, err := importer.ParseCardFile([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if _, err := importer.WriteFolder(importer.FromTavern(parsed, "en"), dir, false); err != nil {
				t.Fatal(err)
			}
			result, err := Card(dir)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, f := range result.Findings {
				if strings.HasPrefix(f.Where, "lorebook.json entries[") {
					count++
				}
			}
			want := 2
			if enabled {
				want = 0
			}
			if count != want {
				t.Fatalf("import use_regex=%t: got %d, want %d: %+v", enabled, count, want, result.Findings)
			}
		})
	}
}
