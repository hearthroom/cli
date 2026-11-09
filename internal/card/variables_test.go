package card

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func mustNode(t *testing.T, s string) *JSONNode {
	t.Helper()
	n, err := ParseJSON([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// The provider's MVU rules: [value, "rule"] leaves, $meta opens objects
// (recursively too), $meta.required protects keys, list markers are dropped and
// a list that is the value of a pair stays a list.
func TestNormalizeVariablesMVU(t *testing.T) {
	data := mustNode(t, `{
	  "好感度": [50, "+1 per kind act"],
	  "背包": {"$meta": {"extensible": true, "required": ["錢"]}, "錢": [10, "spent on purchases"]},
	  "地圖": {"$meta": {"recursiveExtensible": true}, "城": {"門": "closed"}},
	  "裝備": [["短刀", "鐵護腕"], "what the hero carries"],
	  "日誌": ["$__META_EXTENSIBLE__$", "day 1"]
	}`)
	n, reason := NormalizeVariables("mvu", data, map[string]string{"日誌[0]": " first day "})
	if reason != "" {
		t.Fatal(reason)
	}
	want := `{"好感度":50,"背包":{"錢":10},"地圖":{"城":{"門":"closed"}},"裝備":["短刀","鐵護腕"],"日誌":["day 1"]}`
	if got := n.Data.Compact(); got != want {
		t.Fatalf("data = %s\nwant  %s", got, want)
	}
	wantPaths := map[string]string{"好感度": "+1 per kind act", "背包.錢": "spent on purchases", "裝備": "what the hero carries", "日誌[0]": "first day"}
	if !reflect.DeepEqual(n.Paths, wantPaths) {
		t.Fatalf("paths = %v", n.Paths)
	}
	if !reflect.DeepEqual(n.Open, []string{"地圖", "地圖.城", "背包"}) || !reflect.DeepEqual(n.Required, []string{"背包.錢"}) {
		t.Fatalf("open = %v required = %v", n.Open, n.Required)
	}
	if _, reason := NormalizeVariables("yaml", data, nil); reason != "convention" {
		t.Fatalf("unknown convention = %q", reason)
	}
	if _, reason := NormalizeVariables("plain", mustNode(t, `[1]`), nil); reason != "data" {
		t.Fatalf("list data = %q", reason)
	}
}

// What the provider stores comes back as plain values plus paths, open and
// required. Pull rebuilds the author's form; pushing it must store the same
// definition, and pulling again must write the same file.
func TestRebuildVariablesRoundTrip(t *testing.T) {
	stored := RemoteVariables{
		Data:     json.RawMessage(`{"好感度":50,"背包":{"錢":10,"物品":[]},"地圖":{},"裝備":["短刀","鐵護腕"],"日誌":["day 1"]}`),
		Open:     []string{"地圖", "背包"},
		Required: []string{"背包.錢"},
	}
	stored.Rules.Text = "Values follow the story."
	stored.Rules.Paths = map[string]string{"好感度": "+1 per kind act", "日誌[0]": "first day"}
	stored.Panel = &struct {
		Template string `json:"template"`
	}{Template: "<b>{{好感度}}</b>"}

	v, err := RebuildVariables(stored)
	if err != nil {
		t.Fatal(err)
	}
	if v.Convention != "mvu" || v.Rules != stored.Rules.Text || v.Panel != stored.Panel.Template {
		t.Fatalf("rebuilt = %+v", v)
	}
	if !reflect.DeepEqual(v.PathRules, map[string]string{"日誌[0]": "first day"}) {
		t.Fatalf("rules MVU cannot carry = %v", v.PathRules)
	}
	norm, reason := NormalizeVariables(v.Convention, mustNode(t, string(v.Data)), v.PathRules)
	if reason != "" {
		t.Fatal(reason)
	}
	if norm.Data.Compact() != mustNode(t, string(stored.Data)).Compact() {
		t.Fatalf("data after push = %s", norm.Data.Compact())
	}
	if !reflect.DeepEqual(norm.Paths, stored.Rules.Paths) || !reflect.DeepEqual(norm.Open, stored.Open) || !reflect.DeepEqual(norm.Required, stored.Required) {
		t.Fatalf("after push: paths %v open %v required %v", norm.Paths, norm.Open, norm.Required)
	}

	dir := t.TempDir()
	if err := v.save(dir); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(dir, VariablesFile))
	loaded, err := LoadVariables(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Rules != v.Rules+"\n" || loaded.Panel != v.Panel+"\n" {
		t.Fatalf("companion files: %q %q", loaded.Rules, loaded.Panel)
	}
	body, ok := loaded.Body(nil)
	if !ok {
		t.Fatal("body")
	}
	again, _ := NormalizeVariables(body["convention"].(string), mustNode(t, string(body["data"].(json.RawMessage))), loaded.PathRules)
	stored2 := RemoteVariables{Data: json.RawMessage(again.Data.Compact()), Open: again.Open, Required: again.Required}
	stored2.Rules.Paths = again.Paths
	v2, err := RebuildVariables(stored2)
	if err != nil {
		t.Fatal(err)
	}
	dir2 := t.TempDir()
	if err := v2.save(dir2); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(dir2, VariablesFile))
	if string(first) != string(second) {
		t.Fatalf("second pull differs:\n%s\n---\n%s", first, second)
	}
	if !strings.Contains(string(first), `"好感度": [50, "+1 per kind act"]`) || !strings.Contains(string(first), "[\"短刀\", \"鐵護腕\"],\n      \"\"") {
		t.Fatalf("file:\n%s", first)
	}
	if strings.Index(string(first), `"好感度"`) > strings.Index(string(first), `"背包"`) {
		t.Fatalf("key order lost:\n%s", first)
	}
}

// Keys that read as path syntax (a dot, a bracket, a quote, or empty) are written
// ["key"] the way the provider's Path.String does, so open, required and rules on
// them match what pull reads back and survive pull then push.
func TestVariablesBracketKeyPaths(t *testing.T) {
	data := mustNode(t, `{
	  "$meta": {"required": ["HP.max"]},
	  "HP.max": [100, "cap"],
	  "任務.a": {"$meta": {"extensible": true, "required": ["x.y"]}, "x.y": [1, "inner"]},
	  "背包": ["$__META_EXTENSIBLE__$", {"名.稱": ["短刀", "item"]}]
	}`)
	n, reason := NormalizeVariables("mvu", data, nil)
	if reason != "" {
		t.Fatal(reason)
	}
	wantPaths := map[string]string{`["HP.max"]`: "cap", `["任務.a"]["x.y"]`: "inner", `背包[0]["名.稱"]`: "item"}
	if !reflect.DeepEqual(n.Paths, wantPaths) {
		t.Fatalf("paths = %v", n.Paths)
	}
	if !reflect.DeepEqual(n.Open, []string{`["任務.a"]`}) || !reflect.DeepEqual(n.Required, []string{`["HP.max"]`, `["任務.a"]["x.y"]`}) {
		t.Fatalf("open = %v required = %v", n.Open, n.Required)
	}

	stored := RemoteVariables{
		Data:     json.RawMessage(`{"HP.max":100,"任務.a":{"x.y":1},"空":{},"背包":["短刀"]}`),
		Open:     []string{`["任務.a"]`, "空"},
		Required: []string{`["HP.max"]`, `["任務.a"]["x.y"]`},
	}
	stored.Rules.Paths = map[string]string{`["HP.max"]`: "cap", "背包[0]": "first item"}
	v, err := RebuildVariables(stored)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v.PathRules, map[string]string{"背包[0]": "first item"}) {
		t.Fatalf("pathRules = %v", v.PathRules)
	}
	again, reason := NormalizeVariables(v.Convention, mustNode(t, string(v.Data)), v.PathRules)
	if reason != "" {
		t.Fatal(reason)
	}
	if !reflect.DeepEqual(again.Paths, stored.Rules.Paths) || !reflect.DeepEqual(again.Open, stored.Open) || !reflect.DeepEqual(again.Required, stored.Required) {
		t.Fatalf("after push: data %s paths %v open %v required %v", again.Data.Compact(), again.Paths, again.Open, again.Required)
	}
	if again.Data.Compact() != mustNode(t, string(stored.Data)).Compact() {
		t.Fatalf("data after push = %s", again.Data.Compact())
	}
}

// Only empty objects open and no rules: the plain form is enough.
func TestRebuildVariablesPlain(t *testing.T) {
	v, err := RebuildVariables(RemoteVariables{Data: json.RawMessage(`{"b":1,"a":{}}`), Open: []string{"a"}})
	if err != nil || v.Convention != "plain" || string(v.Data) != `{"b":1,"a":{}}` {
		t.Fatalf("rebuilt = %+v, %v", v, err)
	}
	// An object opened by $meta while it has keys needs MVU to say so.
	v, _ = RebuildVariables(RemoteVariables{Data: json.RawMessage(`{"a":{"x":1}}`), Open: []string{"a"}})
	if v.Convention != "mvu" || string(v.Data) != `{"a":{"$meta":{"extensible":true},"x":1}}` {
		t.Fatalf("rebuilt = %s %s", v.Convention, v.Data)
	}
}

func TestLoadVariables(t *testing.T) {
	dir := t.TempDir()
	if v, err := LoadVariables(dir); v != nil || err != nil {
		t.Fatalf("absent = %v, %v", v, err)
	}
	os.WriteFile(filepath.Join(dir, VariablesFile), []byte("{\n  \"convention\": \"plain\",\n  \"data\": {\"a\": 1,}\n}\n"), 0o644)
	if _, err := LoadVariables(dir); err == nil || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("invalid JSON error = %v", err)
	}
	os.WriteFile(filepath.Join(dir, VariablesFile), []byte("  \n"), 0o644)
	v, err := LoadVariables(dir)
	if err != nil || v == nil || !v.Empty {
		t.Fatalf("empty file = %+v, %v", v, err)
	}
	f := &Folder{Manifest: Manifest{Name: "x"}, Variables: v}
	p, err := f.Build(nil)
	if err != nil || p.State != nil || !p.StateRemoved || p.Digests()[SectionState] == "" {
		t.Fatalf("removal payload = %+v, %v", p, err)
	}
}

// The wire body keeps the author's key order and sends rules and the panel
// only when they have content.
func TestVariablesBody(t *testing.T) {
	v := &Variables{Convention: "mvu", Data: json.RawMessage(`{ "z": [1, "r"], "a": 2 }`), Rules: "<!-- notes -->", Panel: "  \n"}
	body, ok := v.Body(nil)
	if !ok {
		t.Fatal("body")
	}
	if string(body["data"].(json.RawMessage)) != `{"z":[1,"r"],"a":2}` {
		t.Fatalf("data = %s", body["data"])
	}
	if _, ok := body["rules"]; ok {
		t.Fatalf("empty rules sent: %v", body)
	}
	if _, ok := body["panel"]; ok {
		t.Fatalf("empty panel sent: %v", body)
	}
	v.Rules, v.Panel = "Spend money.\n", "<img src=\"assets/bar.png\">\n"
	body, _ = v.Body(map[string]string{"assets/bar.png": "https://cdn/bar.png"})
	if body["rules"].(map[string]any)["text"] != "Spend money." || body["panel"].(map[string]any)["template"] != `<img src="https://cdn/bar.png">` {
		t.Fatalf("body = %v", body)
	}
}

func TestCheckVariablesLimits(t *testing.T) {
	deep := `{"a":` + strings.Repeat(`{"b":`, 8) + `1` + strings.Repeat(`}`, 8) + `}`
	many := map[string]int{}
	for i := 0; i < 401; i++ {
		many["k"+strings.Repeat("x", i%7)+string(rune('A'+i%26))+string(rune('a'+i/26))] = i
	}
	manyJSON, _ := json.Marshal(many)
	for _, tc := range []struct {
		name string
		v    Variables
		want string
	}{
		{"convention", Variables{Convention: "yaml", Data: json.RawMessage(`{}`)}, "convention"},
		{"text data", Variables{Convention: "plain", Data: json.RawMessage(`"hp: 3"`)}, "holds text"},
		{"list data", Variables{Convention: "plain", Data: json.RawMessage(`[1]`)}, "must be a JSON object"},
		{"string", Variables{Convention: "plain", Data: json.RawMessage(`{"s":"` + strings.Repeat("x", 2049) + `"}`)}, `text at "s"`},
		{"depth", Variables{Convention: "plain", Data: json.RawMessage(deep)}, "levels deep"},
		{"keys", Variables{Convention: "plain", Data: manyJSON}, "keys; the limit is 400"},
		{"size", Variables{Convention: "plain", Data: json.RawMessage(`{"a":"` + strings.Repeat("x", 2000) + `","b":"` + strings.Repeat("x", 2000) + `","c":"` + strings.Repeat("x", 2000) + `","d":"` + strings.Repeat("x", 2000) + `","e":"` + strings.Repeat("x", 2000) + `","f":"` + strings.Repeat("x", 2000) + `","g":"` + strings.Repeat("x", 2000) + `","h":"` + strings.Repeat("x", 2000) + `","i":"` + strings.Repeat("x", 2000) + `"}`)}, "limit is 16384"},
		{"rules", Variables{Convention: "plain", Data: json.RawMessage(`{}`), Rules: strings.Repeat("x", 8193)}, VariableRulesFile + " is 8193 bytes"},
		{"panel", Variables{Convention: "plain", Data: json.RawMessage(`{}`), Panel: strings.Repeat("x", 32769)}, PanelFile + " is 32769 bytes"},
		{"rule path", Variables{Convention: "plain", Data: json.RawMessage(`{"a":1}`), PathRules: map[string]string{"b": "gone"}}, `rule for "b"`},
		{"rule paths size", Variables{Convention: "mvu", Data: json.RawMessage(`{"a":[1,"` + strings.Repeat("r", 8200) + `"]}`)}, "rules attached to values"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			problems := CheckVariables(&tc.v, nil)
			if !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("problems = %v, want %q", problems, tc.want)
			}
		})
	}
	ok := Variables{Convention: "mvu", Data: json.RawMessage(`{"a":[1,"rule"],"b":{"$meta":{"extensible":true}}}`), PathRules: map[string]string{}}
	if p := CheckVariables(&ok, nil); len(p) != 0 {
		t.Fatalf("valid variables reported %v", p)
	}
}
