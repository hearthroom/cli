package importer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/card"
)

func TestParseRelaxedJSON(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`{"a": 1}`, `{"a":1}`},
		{"// initial values\n{\n  a: 1, /* note */ 'b': 'two',\n  好感度: [50, '+1'],\n}", `{"a":1,"b":"two","好感度":[50,"+1"]}`},
		{"\ufeff{z: +1.50, y: .5, x: [1,2,],}", `{"z":1.5,"y":0.5,"x":[1,2]}`},
		{`{s: 'it\'s \u4e2d\ud83d\ude00'}`, `{"s":"it's 中😀"}`},
		{`{a: 1, a: 2, b: 3}`, `{"a":2,"b":3}`},
	} {
		n, err := ParseRelaxedJSON(tc.in)
		if err != nil || n.Compact() != tc.want {
			got := ""
			if n != nil {
				got = n.Compact()
			}
			t.Errorf("%q: got %s, %v; want %s", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"好感度: 50\n背包:\n  - 短刀", "{a: 0x10}", "{a: Infinity}", "{a: 'line\nbreak'}", "{a: 1", "<% if (x) { %>"} {
		if _, err := ParseRelaxedJSON(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func mvuCard() []byte {
	b, _ := json.Marshal(map[string]any{
		"spec": "chara_card_v2", "spec_version": "2.0",
		"data": map[string]any{
			"name": "Vars", "first_mes": "Hello.<UpdateVariable>_.set('好感度', 50, 55);</UpdateVariable>",
			"character_book": map[string]any{"entries": []any{
				map[string]any{"keys": []string{}, "comment": "[InitVar] base", "content": "{\n  // 數值\n  好感度: [50, '+1 per kind act'],\n  背包: {錢: [10, 'spent'], 物品: {}},\n}", "enabled": false},
				map[string]any{"keys": []string{}, "comment": "more [initvar]", "content": `{"背包": {"物品": {"短刀": 1}}, "地點": "港口"}`, "enabled": true},
				map[string]any{"keys": []string{}, "comment": "[InitVar] yaml", "content": "好感度: 50\n背包:\n  - 短刀"},
				map[string]any{"keys": []string{"status"}, "comment": "status", "content": "好感度：{{get_message_variable::stat_data.好感度}}"},
				map[string]any{"keys": []string{"harbor"}, "comment": "Harbor", "content": "A busy port."},
			}},
		},
	})
	return b
}

// [InitVar] entries become the card variables (merged in order) and leave the
// Lorebook; YAML ones stay as disabled entries; macro readers are disabled;
// the greeting keeps its <UpdateVariable> block.
func TestTavernInitVarBecomesVariables(t *testing.T) {
	p, err := ParseCardFile(mvuCard())
	if err != nil {
		t.Fatal(err)
	}
	r := FromTavern(p, "en")
	if r.Variables == nil || r.Variables.Convention != "mvu" {
		t.Fatalf("variables = %+v", r.Variables)
	}
	want := `{"好感度":[50,"+1 per kind act"],"背包":{"錢":[10,"spent"],"物品":{"短刀":1}},"地點":"港口"}`
	if string(r.Variables.Data) != want {
		t.Fatalf("data = %s\nwant %s", r.Variables.Data, want)
	}
	if !strings.Contains(r.Welcome, "<UpdateVariable>") {
		t.Fatalf("greeting changed: %q", r.Welcome)
	}
	if r.Lorebook == nil || len(r.Lorebook.Entries) != 3 {
		t.Fatalf("lorebook = %+v", r.Lorebook)
	}
	enabled := map[string]bool{}
	for _, e := range r.Lorebook.Entries {
		enabled[e.Name] = e.Enabled
	}
	if enabled["[InitVar] yaml"] || enabled["status"] || !enabled["Harbor"] {
		t.Fatalf("entry states = %v", enabled)
	}
	notes := strings.Join(r.Notes, "\n")
	for _, want := range []string{"2 initial-variable entries", "1 initial-variable entries are not JSON", "1 Lorebook entries read variables"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes miss %q:\n%s", want, notes)
		}
	}

	dir := filepath.Join(t.TempDir(), "card")
	rep, err := WriteFolder(r, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(rep.Files, ","), card.VariablesFile) {
		t.Fatalf("report files = %v", rep.Files)
	}
	b, _ := os.ReadFile(filepath.Join(dir, card.VariablesFile))
	if !strings.Contains(string(b), `"好感度": [50, "+1 per kind act"]`) {
		t.Fatalf("variables.json:\n%s", b)
	}
	f, err := card.Load(dir)
	if err != nil || len(card.CheckVariables(f.Variables, nil)) != 0 {
		t.Fatalf("load: %v %v", err, card.CheckVariables(f.Variables, nil))
	}
}

// A card without [InitVar] entries keeps its Lorebook as before and gets no variables.
func TestTavernWithoutInitVar(t *testing.T) {
	p, err := ParseCardFile(mustJSON(v3Card()))
	if err != nil {
		t.Fatal(err)
	}
	if r := FromTavern(p, "en"); r.Variables != nil || r.Lorebook == nil {
		t.Fatalf("variables = %+v lorebook = %v", r.Variables, r.Lorebook)
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestFengyueVariablePanel(t *testing.T) {
	base := func(t *testing.T) map[string]any {
		raw, ok := DecodeFengyue(loadFengyueCases(t)[0].Input)
		if !ok {
			t.Fatal("sample not detected")
		}
		return raw
	}
	codes := func(notes []fengyueNote) string {
		var out []string
		for _, n := range notes {
			out = append(out, n.code)
		}
		return strings.Join(out, ",")
	}

	raw := base(t)
	raw["ai_v_json"] = `{"好感度": 10, "物品": {}}`
	raw["ai_v_template"] = "<b>{{好感度}}</b>"
	r, notes := convertFengyue(raw, "zh-Hans")
	v := r.Variables
	if v == nil || v.Convention != "plain" || string(v.Data) != `{"好感度":10,"物品":{}}` || v.Panel != "<b>{{好感度}}</b>" || v.Rules == "" {
		t.Fatalf("variables = %+v", v)
	}
	if c := codes(notes); !strings.Contains(c, "variablesConverted") || strings.Contains(c, "variablesPromptOnly") {
		t.Fatalf("notes = %s", c)
	}

	raw = base(t)
	raw["ai_v_json"] = "好感度: 10"
	r, notes = convertFengyue(raw, "zh-Hans")
	if r.Variables == nil || string(r.Variables.Data) != `"好感度: 10"` || !strings.Contains(codes(notes), "variablesPartial") {
		t.Fatalf("partial = %+v %s", r.Variables, codes(notes))
	}
	if p := card.CheckVariables(r.Variables, nil); len(p) == 0 || !strings.Contains(p[0], "holds text") {
		t.Fatalf("check = %v", p)
	}

	raw = base(t)
	raw["ai_variable_template"] = "<b>skin</b>"
	r, notes = convertFengyue(raw, "zh-Hans")
	if r.Variables != nil || !strings.Contains(codes(notes), "variablesPromptOnly") || !strings.Contains(codes(notes), "variablesTemplateOnly") {
		t.Fatalf("no skeleton = %+v %s", r.Variables, codes(notes))
	}

	raw = base(t)
	raw["ai_v_json"] = map[string]any{"b": json.Number("1"), "a": "x"}
	r, _ = convertFengyue(raw, "zh-Hans")
	if r.Variables == nil || string(r.Variables.Data) != `{"a":"x","b":1}` {
		t.Fatalf("object skeleton = %+v", r.Variables)
	}
}
