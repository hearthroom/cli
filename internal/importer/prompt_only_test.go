package importer

import (
	"strings"
	"testing"
)

func TestTavernPromptOnlyDoesNotBecomeDisplayRule(t *testing.T) {
	raw := []byte(`{"spec":"chara_card_v3","spec_version":"3.0","data":{"name":"Synthetic","extensions":{"regex_scripts":[{"scriptName":"prompt","findRegex":"x","replaceString":"hidden","promptOnly":true},{"scriptName":"both","findRegex":"x","replaceString":"hidden","promptOnly":true,"markdownOnly":true},{"scriptName":"display","findRegex":"x","replaceString":"visible","markdownOnly":true},{"scriptName":"legacy","findRegex":"x","replaceString":"visible"}]}}}`)
	parsed, err := ParseCardFile(raw)
	if err != nil {
		t.Fatal(err)
	}
	result := FromTavern(parsed, "en")
	if result.Rules == nil || len(result.Rules.Rules) != 2 || result.Rules.Rules[0].Name != "display" || result.Rules.Rules[1].Name != "legacy" {
		t.Fatalf("visible rules: %+v", result.Rules)
	}
	if !strings.Contains(strings.Join(result.Notes, "\n"), "2 prompt-only") {
		t.Fatalf("skip report: %v", result.Notes)
	}
}

func TestRuleFilePromptOnlyReport(t *testing.T) {
	for _, raw := range []string{
		`[{"findRegex":"x","replaceString":"hidden","promptOnly":true},{"findRegex":"x","replaceString":"visible","markdownOnly":true}]`,
		`[{"findRegex":"x","replaceString":"hidden","promptOnly":true}]`,
	} {
		file, err := ClassifyMMDFile("rules.json", []byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		result := MergeMMDFiles([]*MMDFile{file}, "en")
		if !strings.Contains(strings.Join(result.Notes, "\n"), "1 prompt-only rule skipped") {
			t.Fatalf("skip report: %v", result.Notes)
		}
	}
}
