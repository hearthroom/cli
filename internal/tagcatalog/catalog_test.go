package tagcatalog

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestCatalogCompletenessAndPredicates(t *testing.T) {
	cat := Catalog("")
	if len(cat.Groups) != 4 {
		t.Fatalf("groups = %v, want 4 groups", cat.Groups)
	}
	if len(cat.Items) != 52 {
		t.Fatalf("items = %d, want 52", len(cat.Items))
	}
	groupCounts := map[string]int{}
	for _, item := range cat.Items {
		if item.Key == "" || item.Group == "" {
			t.Fatalf("incomplete item: %+v", item)
		}
		groupCounts[item.Group]++
		for _, loc := range Locales {
			if !ValidLocale(loc) {
				t.Fatalf("ValidLocale(%q) = false", loc)
			}
			if item.Names[loc] == "" {
				t.Errorf("item %q missing %s display name", item.Key, loc)
			}
		}
	}
	wantGroups := map[string]int{"play": 11, "setting": 12, "character": 15, "kink": 14}
	for g, want := range wantGroups {
		if groupCounts[g] != want {
			t.Errorf("group %q count = %d, want %d", g, groupCounts[g], want)
		}
	}
	if ValidLocale("zh") || ValidLocale("fr") || ValidLocale("") {
		t.Fatal("unexpected ValidLocale acceptance")
	}

	// Display names in any of the 5 locales match, with surrounding whitespace trimmed.
	for _, sample := range []string{"角色扮演", " 世界模拟器 ", "World simulator", "ロールプレイ", "롤플레이", "Women’s fiction (BL/ABO/NP)", "調教&強迫"} {
		if !HasPresetTag([]string{"custom-tag", sample}) {
			t.Errorf("HasPresetTag(%q) = false, want true", sample)
		}
	}
	// URL keys and custom tags do NOT count.
	if HasPresetTag([]string{"roleplay", "western-fantasy", "womens-fiction", "自訂標籤", ""}) {
		t.Fatal("URL keys or custom tags should not count as preset tags")
	}
	if got := CountPresetTags([]string{"Roleplay", "角色扮演", "懸疑"}); got != 2 {
		t.Fatalf("CountPresetTags deduplicated by catalog entry = %d, want 2", got)
	}

	// StoredTagsMax = 20 non-blank items: index 19 counts, index 20 does not; blank entries are skipped first.
	twenty := make([]string, 0, 22)
	twenty = append(twenty, "   ") // blank does not consume a slot
	for i := 0; i < 19; i++ {
		twenty = append(twenty, "custom")
	}
	with20th := append(append([]string{}, twenty...), "Roleplay")
	if !HasPresetTag(with20th) {
		t.Fatal("20th non-blank tag should still be checked")
	}
	with21st := append(append([]string{}, twenty...), "custom-20", "Roleplay")
	if HasPresetTag(with21st) {
		t.Fatal("21st non-blank tag exceeds StoredTagsMax=20 and must not count")
	}
}

func TestSharedTagCatalogSourceParity(t *testing.T) {
	candidates := []string{
		filepath.Join("..", "..", "..", "hearthroom", "shared", "tag-catalog.ts"),
		filepath.Join("..", "..", "..", "..", "hearthroom", "shared", "tag-catalog.ts"),
	}
	var raw []byte
	for _, p := range candidates {
		if b, err := os.ReadFile(p); err == nil {
			raw = b
			break
		}
	}
	if len(raw) == 0 {
		t.Skip("shared/tag-catalog.ts not present in sibling checkout")
	}
	re := regexp.MustCompile(`t\("([^"]+)",\s*"([^"]+)",\s*"([^"]+)",\s*"([^"]+)",\s*"([^"]+)",\s*"([^"]+)"\)`)
	matches := re.FindAllStringSubmatch(string(raw), -1)
	if len(matches) != 52 {
		t.Fatalf("parsed %d entries from tag-catalog.ts, want 52", len(matches))
	}
	cat := Catalog("")
	if len(cat.Items) != len(matches) {
		t.Fatalf("Go catalog has %d items, TS has %d", len(cat.Items), len(matches))
	}
	for i, m := range matches {
		got := cat.Items[i]
		if got.Key != m[1] ||
			got.Names["zh-Hans"] != m[2] ||
			got.Names["zh-Hant"] != m[3] ||
			got.Names["en"] != m[4] ||
			got.Names["ja"] != m[5] ||
			got.Names["ko"] != m[6] {
			t.Errorf("entry %d mismatch: got %+v, want key=%q names=%v", i, got, m[1], m[2:7])
		}
	}
}
