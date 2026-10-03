package importer

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEntryContentMaxFor(t *testing.T) {
	cases := map[string]int{
		"":        4000,
		"zh":      4000,
		"zh-TW":   4000,
		"zh_Hant": 4000,
		"zh-HK":   4000,
		"zh-CN":   4000,
		"zh-Hans": 4000,
		"zh_sg":   4000,
		"en":      12000,
		"en-US":   12000,
		"EN_gb":   12000,
		"ja":      6000,
		"JA":      6000,
		"ko":      6000,
		"fr":      4000,
		"ja-JP":   6000,
		"ko_KR":   6000,
		"de":      4000,
	}
	for lang, want := range cases {
		if got := EntryContentMaxFor(lang); got != want {
			t.Errorf("EntryContentMaxFor(%q) = %d, want %d", lang, got, want)
		}
	}
}

// importLorebook runs a JSON card with one Lorebook entry through FromTavern.
func importLorebook(t *testing.T, language, content string) *Result {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"spec": "chara_card_v2", "spec_version": "2.0",
		"data": map[string]any{
			"name": "Limit", "first_mes": "hi",
			"character_book": map[string]any{"entries": []any{
				map[string]any{"keys": []string{"k"}, "content": content, "enabled": true},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCardFile(b)
	if err != nil {
		t.Fatal(err)
	}
	r := FromTavern(parsed, language)
	if r.Lorebook == nil {
		t.Fatal("no Lorebook")
	}
	return r
}

func splitNote(r *Result) string {
	for _, n := range r.Notes {
		if strings.Contains(n, "split") {
			return n
		}
	}
	return ""
}

func TestLorebookSplitFollowsCardLanguage(t *testing.T) {
	long := func(n int) string {
		var b strings.Builder
		for b.Len() < n {
			b.WriteString(strings.Repeat("a", 99) + "\n")
		}
		return b.String()[:n]
	}
	han := func(n int) string { return strings.Repeat("字", n) }

	if r := importLorebook(t, "en", long(10000)); len(r.Lorebook.Entries) != 1 || splitNote(r) != "" {
		t.Fatalf("en 10000: %d entries, note %q", len(r.Lorebook.Entries), splitNote(r))
	}
	if r := importLorebook(t, "zh-Hant", han(3500)); len(r.Lorebook.Entries) != 1 || splitNote(r) != "" {
		t.Fatalf("zh-Hant 3500: %d entries", len(r.Lorebook.Entries))
	}
	r := importLorebook(t, "zh-Hans", han(4500))
	if len(r.Lorebook.Entries) < 2 {
		t.Fatalf("zh-Hans 4500 should split: %d entries", len(r.Lorebook.Entries))
	}
	for _, e := range r.Lorebook.Entries {
		if n := len([]rune(e.Content)); n > 4000 {
			t.Fatalf("part of %d characters exceeds 4000", n)
		}
	}
	if !strings.Contains(splitNote(r), "4000") {
		t.Fatalf("note should name the 4000 limit: %q", splitNote(r))
	}
	if r := importLorebook(t, "ja", han(6500)); len(r.Lorebook.Entries) < 2 {
		t.Fatalf("ja 6500 should split: %d entries", len(r.Lorebook.Entries))
	}
}
