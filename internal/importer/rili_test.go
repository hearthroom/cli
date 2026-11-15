package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/card"
)

// testdata/rili-cases.json is identical to hearthroom/web/test/fixtures/rili-cases.json.
// Both Web and CLI importers must produce the exact same projection.
type riliCase struct {
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input"`
	Expect json.RawMessage `json:"expect"`
}

func loadRiliCases(t *testing.T) []riliCase {
	t.Helper()
	b, err := os.ReadFile("testdata/rili-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct{ Cases []riliCase }
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatal(err)
	}
	return file.Cases
}

func riliLorebookProjection(t *testing.T, entries []EntryDraft) []any {
	t.Helper()
	out := []any{}
	for _, e := range entries {
		if _, hasExt := e.MatchOptions["extensions"]; hasExt {
			t.Fatalf("unexpected internal extensions leaked into matchOptions: %+v", e.MatchOptions)
		}
		selective, _ := e.MatchOptions["selective"].(bool)
		selectiveLogic, _ := e.MatchOptions["selectiveLogic"].(int)
		scanDepth, _ := e.MatchOptions["scanDepth"].(int)
		order, _ := e.MatchOptions["order"].(int)
		sec := e.SecondaryKeywords
		if sec == nil {
			sec = []string{}
		}
		keys := e.Keywords
		if keys == nil {
			keys = []string{}
		}
		out = append(out, map[string]any{
			"name":              e.Name,
			"content":           e.Content,
			"keywords":          keys,
			"secondaryKeywords": sec,
			"selective":         selective,
			"selectiveLogic":    selectiveLogic,
			"scanDepth":         scanDepth,
			"order":             order,
			"triggerRegion":     e.TriggerRegion,
			"enabled":           e.Enabled,
		})
	}
	return out
}

func riliProjection(t *testing.T, r *Result, notes []riliNote) map[string]any {
	t.Helper()
	lore := []any{}
	if r.Lorebook != nil {
		lore = riliLorebookProjection(t, r.Lorebook.Entries)
	}
	rules := []any{}
	if r.Rules != nil {
		for _, rr := range r.Rules.Rules {
			var targets any
			if rr.Targets != nil {
				targets = *rr.Targets
			}
			rules = append(rules, map[string]any{
				"name":    rr.Name,
				"find":    rr.Find,
				"replace": rr.Replace,
				"enabled": rr.Enabled,
				"targets": targets,
			})
		}
	}
	var state any
	if r.Variables != nil {
		var data any
		if err := json.Unmarshal(r.Variables.Data, &data); err != nil {
			data = string(r.Variables.Data)
		}
		pathRules := r.Variables.PathRules
		if pathRules == nil {
			pathRules = map[string]string{}
		}
		state = map[string]any{
			"convention": r.Variables.Convention,
			"data":       data,
			"rulesText":  r.Variables.Rules,
			"template":   r.Variables.Panel,
			"pathRules":  pathRules,
		}
	}
	codes := []string{}
	for _, n := range notes {
		if n.code != "split" {
			codes = append(codes, n.code)
		}
	}
	sort.Strings(codes)
	tags := r.Tags
	if tags == nil {
		tags = []string{}
	}
	return map[string]any{
		"rili":           true,
		"name":           r.Name,
		"summary":        r.Summary,
		"tags":           tags,
		"definition":     r.Definition,
		"outputContract": r.OutputContract,
		"custom":         r.Custom,
		"intro":          r.OpeningPage,
		"css":            r.CardCSS,
		"cssDialect":     r.CardCSSDialect,
		"lorebook":       lore,
		"rules":          rules,
		"state":          state,
		"notes":          codes,
	}
}

func TestRiliSharedCases(t *testing.T) {
	for _, c := range loadRiliCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			var expect map[string]any
			if err := json.Unmarshal(c.Expect, &expect); err != nil {
				t.Fatal(err)
			}
			raw, ok := DecodeRili(c.Input)
			if ok != expect["rili"].(bool) {
				t.Fatalf("DecodeRili = %v, want %v", ok, expect["rili"])
			}
			wantLorebook, _ := expect["riliLorebook"].(bool)
			if gotLb := IsRiliLorebookBytes(c.Input); gotLb != wantLorebook {
				t.Fatalf("IsRiliLorebookBytes = %v, want %v", gotLb, wantLorebook)
			}
			if ok {
				r, notes := convertRili(raw, c.Input, "zh-Hans")
				got := riliProjection(t, r, notes)
				if !reflect.DeepEqual(normalise(t, got), normalise(t, expect)) {
					gb, _ := json.MarshalIndent(got, "", "  ")
					eb, _ := json.MarshalIndent(expect, "", "  ")
					t.Fatalf("got\n%s\nwant\n%s", gb, eb)
				}
			}
			if wantLorebook {
				f, err := ClassifyMMDFile("lorebook.json", c.Input)
				if err != nil {
					t.Fatalf("ClassifyMMDFile(lorebook.json): %v", err)
				}
				if f.Part != PartBook || f.Book == nil {
					t.Fatalf("ClassifyMMDFile part = %q, want %q", f.Part, PartBook)
				}
				drafts := BookEntriesToDrafts(f.Book.Entries, "zh-Hans")
				gotLore := riliLorebookProjection(t, drafts)
				if !reflect.DeepEqual(normalise(t, gotLore), normalise(t, expect["lorebook"])) {
					gb, _ := json.MarshalIndent(gotLore, "", "  ")
					eb, _ := json.MarshalIndent(expect["lorebook"], "", "  ")
					t.Fatalf("lorebook got\n%s\nwant\n%s", gb, eb)
				}
				codes := riliBookNoteCodes(f.Book.Entries)
				sort.Strings(codes)
				if !reflect.DeepEqual(normalise(t, codes), normalise(t, expect["notes"])) {
					t.Fatalf("lorebook notes = %v, want %v", codes, expect["notes"])
				}
			}
		})
	}
}

func TestRiliWritesValidCardFolder(t *testing.T) {
	cases := loadRiliCases(t)
	// Case 0: real Sample A whole card; Case 4: nested memorySchema with arrays & dotted keys.
	for _, idx := range []int{0, 4} {
		raw, ok := DecodeRili(cases[idx].Input)
		if !ok {
			t.Fatalf("case %d not detected", idx)
		}
		dir := filepath.Join(t.TempDir(), "card")
		res := FromRili(raw, cases[idx].Input, "zh-Hans")
		rep, err := WriteFolder(res, dir, false)
		if err != nil {
			t.Fatalf("case %d WriteFolder: %v", idx, err)
		}
		if rep.Spec != "rili" {
			t.Fatalf("rep.Spec = %q, want rili", rep.Spec)
		}
		f, err := card.Load(dir)
		if err != nil {
			t.Fatalf("case %d card.Load: %v", idx, err)
		}
		if f.Variables == nil {
			t.Fatalf("case %d expected variables.json", idx)
		}
		body, ok := f.Variables.Body(nil)
		if !ok {
			t.Fatalf("case %d Variables.Body failed", idx)
		}
		if problems := card.CheckVariables(f.Variables, body); len(problems) > 0 {
			t.Fatalf("case %d CheckVariables problems: %v", idx, problems)
		}
		if f.Rules == nil || f.Rules.CardFormat != "fengyue" || f.Rules.PageMode != "sandbox" || f.Rules.OpeningMode != "page" {
			t.Fatalf("case %d rules: %+v", idx, f.Rules)
		}
		if idx == 0 {
			if f.Rules.CardCSSDialect == nil || *f.Rules.CardCSSDialect != "rili" {
				t.Fatalf("case 0 CardCSSDialect = %v, want rili", f.Rules.CardCSSDialect)
			}
			if f.CardCSS == nil || *f.CardCSS != res.CardCSS {
				t.Fatalf("case 0 CardCSS mismatch: %v", f.CardCSS)
			}
			joinedNotes := strings.Join(rep.Notes, "\n")
			if !strings.Contains(joinedNotes, "https://public.talkaicc.com/cover/fplwg2n3qx01n7pxtjmth2w9.webp") ||
				!strings.Contains(joinedNotes, "https://public.talkaicc.com/background/jknjh9pn7ke2d4kc4rlfpwsk.webp") {
				t.Fatalf("expected talkaicc CDN URLs in CLI notes, got:\n%s", joinedNotes)
			}
		} else {
			if f.Rules.CardCSSDialect != nil || f.CardCSS != nil {
				t.Fatalf("case %d expected empty CardCSS/CardCSSDialect, got %v / %v", idx, f.CardCSS, f.Rules.CardCSSDialect)
			}
		}
	}
}

func TestRiliMemorySchemaValidationAndLimits(t *testing.T) {
	cases := loadRiliCases(t)
	base, ok := DecodeRili(cases[0].Input)
	if !ok {
		t.Fatal("case 0 decode failed")
	}

	for _, rawSchema := range []string{
		`{"type":123}`,
		`{"type":["object"]}`,
		`{"items":{"type":"string"}}`,
		`{"type":"object","default":"not-an-object"}`,
	} {
		in := map[string]any{
			"title":              base["title"],
			"description":        base["description"],
			"introPage":          base["introPage"],
			"prompt":             base["prompt"],
			"enableMemorySchema": true,
			"memorySchema":       rawSchema,
		}
		res, notes := convertRili(in, nil, "zh-Hans")
		if res.Variables == nil {
			t.Fatalf("schema %s: expected Variables to retain raw text", rawSchema)
		}
		if problems := card.CheckVariables(res.Variables, nil); len(problems) == 0 {
			t.Fatalf("schema %s: expected CheckVariables to block pushing", rawSchema)
		}
		var codes []string
		for _, n := range notes {
			codes = append(codes, n.code)
		}
		if !slices.Contains(codes, "memoryInvalid") || slices.Contains(codes, "memoryConverted") {
			t.Fatalf("schema %s: unexpected note codes %v", rawSchema, codes)
		}
	}

	props := map[string]any{}
	for i := range 60 {
		props[fmt.Sprintf("field_%02d", i)] = map[string]any{
			"type":        "number",
			"default":     i,
			"description": strings.Repeat("規", 60),
		}
	}
	overBytes, _ := json.Marshal(map[string]any{"type": "object", "properties": props})
	overIn := map[string]any{
		"title":              base["title"],
		"description":        base["description"],
		"introPage":          base["introPage"],
		"prompt":             base["prompt"],
		"enableMemorySchema": true,
		"memorySchema":       string(overBytes),
	}
	res, notes := convertRili(overIn, nil, "zh-Hans")
	if res.Variables == nil {
		t.Fatal("expected Variables for over-limit schema")
	}
	if len(res.Variables.PathRules) != 0 {
		t.Fatalf("expected PathRules > 8 KiB to spill into Rules, got %d entries", len(res.Variables.PathRules))
	}
	if !strings.Contains(res.Variables.Rules, "field_00:") {
		t.Fatalf("expected spilled field rules in Rules, got %q", res.Variables.Rules)
	}
	if problems := card.CheckVariables(res.Variables, nil); len(problems) == 0 {
		t.Fatal("expected CheckVariables problem for over-limit spilled rules")
	}
	var codes []string
	for _, n := range notes {
		codes = append(codes, n.code)
	}
	if !slices.Contains(codes, "memoryLimit") || slices.Contains(codes, "memoryConverted") {
		t.Fatalf("expected memoryLimit note without memoryConverted, got %v", codes)
	}
}
