package importer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/card"
)

// testdata/fengyue-cases.json is the same file as hearthroom/web/test/fixtures/fengyue-cases.json:
// one test card re-keyed with every alias Fengyue was seen to use, plus edge and negative cases.
// Both importers must produce the same projection.
type fengyueCase struct {
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input"`
	Expect json.RawMessage `json:"expect"`
}

func loadFengyueCases(t *testing.T) []fengyueCase {
	t.Helper()
	b, err := os.ReadFile("testdata/fengyue-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct{ Cases []fengyueCase }
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatal(err)
	}
	return file.Cases
}

var styleBody = regexp.MustCompile(`<style>([\s\S]*)</style>`)

func fengyueProjection(r *Result, notes []fengyueNote) map[string]any {
	lore := []any{}
	if r.Lorebook != nil {
		for _, e := range r.Lorebook.Entries {
			order, _ := e.MatchOptions["order"].(int)
			lore = append(lore, map[string]any{"name": e.Name, "content": e.Content, "keywords": e.Keywords, "triggerRegion": e.TriggerRegion, "enabled": e.Enabled, "order": order})
		}
	}
	rules, css := []any{}, ""
	if r.Rules != nil {
		for _, rr := range r.Rules.Rules {
			if rr.Name == FengyueCSSRuleName && rr.Find == fengyueCSSFind {
				if m := styleBody.FindStringSubmatch(rr.Replace); m != nil {
					css = m[1]
				}
				continue
			}
			var targets any
			if rr.Targets != nil {
				targets = *rr.Targets
			}
			rules = append(rules, map[string]any{"name": rr.Name, "find": rr.Find, "replace": rr.Replace, "enabled": rr.Enabled, "targets": targets})
		}
	}
	codes := []string{}
	for _, n := range notes {
		if n.code != "split" {
			codes = append(codes, n.code)
		}
	}
	sort.Strings(codes)
	prologue := r.Prologue
	if prologue == nil {
		prologue = []string{}
	}
	out := map[string]any{
		"fengyue": true, "name": r.Name, "summary": r.Summary, "definition": r.Definition,
		"outputContract": r.OutputContract, "custom": r.Custom, "prologue": prologue, "css": css,
		"lorebook": lore, "rules": rules, "notes": codes,
	}
	return out
}

// normalise round-trips through JSON so Go slices and maps compare equal to decoded fixtures.
func normalise(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestFengyueSharedCases(t *testing.T) {
	for _, c := range loadFengyueCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			var expect map[string]any
			if err := json.Unmarshal(c.Expect, &expect); err != nil {
				t.Fatal(err)
			}
			raw, ok := DecodeFengyue(c.Input)
			if ok != expect["fengyue"].(bool) {
				t.Fatalf("detected fengyue = %v, want %v", ok, expect["fengyue"])
			}
			if !ok {
				return
			}
			r, notes := convertFengyue(raw, "zh-Hans")
			got := fengyueProjection(r, notes)
			intro, _ := expect["intro"].(string)
			delete(expect, "intro") // the folder has no place for it yet; checked through the note
			if !reflect.DeepEqual(normalise(t, got), normalise(t, expect)) {
				gb, _ := json.MarshalIndent(got, "", " ")
				eb, _ := json.MarshalIndent(expect, "", " ")
				t.Fatalf("got\n%s\nwant\n%s", gb, eb)
			}
			if intro == "" {
				t.Fatal("fixture should carry an intro")
			}
		})
	}
}

func TestFengyueTooLongIsKeptWhole(t *testing.T) {
	raw, ok := DecodeFengyue(loadFengyueCases(t)[0].Input)
	if !ok {
		t.Fatal("sample not detected")
	}
	raw["posttxt"] = strings.Repeat("長", 600)
	r, notes := convertFengyue(raw, "zh-Hans")
	if got := len([]rune(r.Custom)); got != 600 {
		t.Fatalf("custom instructions cut to %d", got)
	}
	found := false
	for _, n := range notes {
		found = found || n.code == "tooLong.custom"
	}
	if !found {
		t.Fatalf("no tooLong.custom note in %+v", notes)
	}
}

func TestFengyueWritesFolder(t *testing.T) {
	raw, ok := DecodeFengyue(loadFengyueCases(t)[0].Input)
	if !ok {
		t.Fatal("sample not detected")
	}
	dir := filepath.Join(t.TempDir(), "card")
	rep, err := WriteFolder(FromFengyue(raw, "zh-Hans"), dir, false)
	if err != nil {
		t.Fatal(err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.Manifest.Name != "xB_au9VW" || f.Manifest.CustomInstructions != "TEST-POST-TEXT 后置词" || !reflect.DeepEqual(f.Manifest.Prologue, []string{"TEST-OPENING 开场白选项一"}) {
		t.Fatalf("manifest: %+v", f.Manifest)
	}
	if f.Intro != nil {
		t.Fatal("the detailed introduction is HTML and must not land in intro.md (Markdown)")
	}
	if f.Lorebook == nil || f.Lorebook.Entries[0].TriggerRegion != "user_only" {
		t.Fatalf("lorebook: %+v", f.Lorebook)
	}
	if f.Rules == nil || f.Rules.PageMode != "sandbox" || f.Rules.Rules[0].Targets == nil || !reflect.DeepEqual(*f.Rules.Rules[0].Targets, []string{"ai", "user"}) {
		t.Fatalf("rules: %+v", f.Rules)
	}
	if len(rep.Notes) == 0 || rep.Spec != "fengyue" {
		t.Fatalf("report: %+v", rep)
	}
}
