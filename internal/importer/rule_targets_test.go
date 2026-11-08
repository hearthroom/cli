package importer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRuleImportPreservesMessageTargets(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      any
		count     int
	}{
		{"user", `[{"findRegex":"x","replaceString":"y","placement":[1]}]`, []any{"user"}, 1},
		{"ai", `[{"findRegex":"x","replaceString":"y","placement":[2]}]`, []any{"ai"}, 1},
		{"both", `[{"findRegex":"x","replaceString":"y","placement":[1,2,1]}]`, []any{"user", "ai"}, 1},
		{"non-message", `[{"findRegex":"x","placement":[3,4]}]`, nil, 0},
		{"missing", `[{"findRegex":"x"}]`, nil, 1},
		{"empty", `[{"findRegex":"x","placement":[]}]`, nil, 1},
		{"native-ai", `{"rules":[{"find":"x","replace":"y","targets":["ai"]}]}`, []any{"ai"}, 1},
		{"native-empty", `{"rules":[{"find":"x","replace":"y","targets":[]}]}`, []any{}, 1},
		{"native-legacy", `{"rules":[{"find":"x","replace":"y"}]}`, nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw any
			if err := json.Unmarshal([]byte(tc.raw), &raw); err != nil {
				t.Fatal(err)
			}
			set, _ := RuleSetFromImport(raw)
			if tc.count == 0 {
				if set != nil {
					t.Fatalf("non-message script retained: %+v", set)
				}
				return
			}
			if set == nil || len(set.Rules) != tc.count {
				t.Fatalf("rules=%+v, want count %d", set, tc.count)
			}
			report, err := WriteFolder(&Result{Name: "Fixture", Language: "en", Rules: set}, t.TempDir(), false)
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(filepath.Join(report.Dir, "rules.json"))
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Rules []map[string]any `json:"rules"`
			}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Rules[0]["targets"], tc.want) {
				t.Fatalf("targets=%#v want %#v", got.Rules[0]["targets"], tc.want)
			}
		})
	}
}
