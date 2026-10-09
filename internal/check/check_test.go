package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func msgs(r *Result) []string {
	out := make([]string, 0, len(r.Findings))
	for _, f := range r.Findings {
		out = append(out, f.Level+":"+f.Msg)
	}
	return out
}

func has(list []string, parts ...string) bool {
	for _, s := range list {
		ok := true
		for _, p := range parts {
			if !strings.Contains(s, p) {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func TestCleanSandboxCard(t *testing.T) {
	dir := write(t, t.TempDir(), map[string]string{
		"card.json":     `{"formatVersion":1,"name":"Mira","language":"en"}`,
		"README.md":     "uiRole: assist\n",
		"definition.md": "# Role\nEnd every reply with a [status] block: hp: a/b, mood: word.",
		"welcome.md":    "Hello.\n[status]\nhp: 10/10\nmood: calm\n[/status]",
		"rules.json":    `{"pageMode":"sandbox","rules":[{"id":"s","name":"s","find":"/\\[status\\]([\\s\\S]*?)\\[\\/status\\]/g","replace":"<div class=\"hr-status hr-status--raw\">$1</div>","enabled":true}]}`,
	})
	r, err := Card(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Errors() != 0 {
		t.Fatalf("unexpected errors: %v", msgs(r))
	}
	if r.Declared.UIRole != "assist" {
		t.Fatalf("declared: %+v", r.Declared)
	}
}

func TestRuleErrors(t *testing.T) {
	big := strings.Repeat("y", contract.Provider.ReplaceMaxBytes+1)
	dir := write(t, t.TempDir(), map[string]string{
		"card.json":  `{"formatVersion":1,"name":"x","language":"en"}`,
		"rules.json": `{"pageMode":"sandbox","rules":[{"id":"a","find":"/(/","replace":"x","enabled":true},{"id":"b","find":"/a*/","replace":"x","enabled":true},{"id":"c","find":"  ","replace":"x","enabled":true},{"id":"d","find":"d","replace":"` + big + `","enabled":true},{"id":"e","find":"/e/v","replace":"x","enabled":true},{"id":"e","find":"f","replace":"x","enabled":true}]}`,
	})
	r, _ := Card(dir)
	m := msgs(r)
	for _, want := range []string{"invalid pattern", "empty string", "find is blank", "over 131072", `flag "v"`, "duplicate rule id"} {
		if !has(m, "error:", want) {
			t.Errorf("missing error %q in %v", want, m)
		}
	}
}

func TestSDKMisuseAndClassicPage(t *testing.T) {
	dir := write(t, t.TempDir(), map[string]string{
		"card.json":  `{"formatVersion":1,"name":"x","language":"en"}`,
		"rules.json": `{"pageMode":"classic","rules":[{"id":"k","find":"{{k}}","replace":"<script>sdk.on(\"message:finish\", f); sdk.save.put(\"a\", 1); sdk.once(\"ready\", f); sdk.vars.get(\"x\"); import x from \"y\"; save.set(\"bad:key\", 1)</script>","enabled":true}]}`,
	})
	r, _ := Card(dir)
	m := msgs(r)
	for _, want := range []string{`sdk.on("message:finish")`, "sdk.save.put does not exist", "sdk.off / sdk.once", "sdk.vars", "ES module syntax", `pageMode is not "sandbox"`, `save key "bad:key"`} {
		if !has(m, want) {
			t.Errorf("missing %q in %v", want, m)
		}
	}
}

func TestSanitizerTraps(t *testing.T) {
	dir := write(t, t.TempDir(), map[string]string{
		"card.json":  `{"formatVersion":1,"name":"x","language":"en"}`,
		"rules.json": `{"pageMode":"sandbox","rules":[{"id":"h","find":"{{h}}","replace":"<div data-x=\"1\"><svg onclick=\"a()\"></svg><状态>x</状态><hc-btn>b</hc-btn>{{random:a|b}}</div>","enabled":true}]}`,
	})
	r, _ := Card(dir)
	m := msgs(r)
	for _, want := range []string{"data-x", "inside <svg>", "<状态>", "hc-btn", "{{random:a|b}}"} {
		if !has(m, want) {
			t.Errorf("missing %q in %v", want, m)
		}
	}
}

func TestRenderIsNotGeneration(t *testing.T) {
	base := map[string]string{
		"card.json":     `{"formatVersion":1,"name":"x","language":"en"}`,
		"definition.md": "# Role\nDescribe the scene before each line; a scene is a place and a time.",
		"welcome.md":    "Hi.",
		"rules.json":    `{"pageMode":"sandbox","rules":[{"id":"s","find":"/\\[scene\\]([\\s\\S]*?)\\[\\/scene\\]/g","replace":"<b>$1</b>","enabled":true}]}`,
	}
	r, _ := Card(write(t, t.TempDir(), base))
	if !has(msgs(r), `consumes the marker "scene"`, "never appear") {
		t.Fatalf("expected the marker warning: %v", msgs(r))
	}
	ok := map[string]string{}
	for k, v := range base {
		ok[k] = v
	}
	ok["lorebook.json"] = `{"name":"b","entries":[{"name":"format","content":"Write [scene] each turn","keywords":[],"constant":true}]}`
	r, _ = Card(write(t, t.TempDir(), ok))
	if has(msgs(r), "consumes the marker") {
		t.Fatalf("a constant entry should count: %v", msgs(r))
	}
}

func TestAssetsAndDeclarations(t *testing.T) {
	dir := write(t, t.TempDir(), map[string]string{
		"card.json":     `{"formatVersion":1,"name":"x","language":"en"}`,
		"README.md":     "uiRole: core\n",
		"assets/a.webp": "x",
		"rules.json":    `{"pageMode":"sandbox","rules":[{"id":"i","find":"{{i}}","replace":"<img src=\"assets/a.webp\"><img src=\"assets/missing.webp\"><script>var p = \"assets/\" + id + \".webp\"</script>","enabled":true}]}`,
	})
	r, _ := Card(dir)
	m := msgs(r)
	if !has(m, "assets/missing.webp", "does not exist") || has(m, "assets/a.webp", "does not exist") {
		t.Errorf("asset check wrong: %v", m)
	}
	if !has(m, "concatenation") {
		t.Errorf("concatenation not flagged: %v", m)
	}
	if !has(m, "statusOverheadThreshold") {
		t.Errorf("core card without a threshold should warn: %v", m)
	}
}

func TestDeclarations(t *testing.T) {
	d := ReadDeclarations("# notes\nuiRole: Core\nstatusOverheadThreshold: 25%\n")
	if d.UIRole != "core" || d.Threshold != 0.25 {
		t.Fatalf("%+v", d)
	}
	if d := ReadDeclarations("statusOverheadThreshold: 0.2"); d.Threshold != 0.2 {
		t.Fatalf("%+v", d)
	}
}

func TestReplayHealthAndTranscripts(t *testing.T) {
	replies := []string{
		"A long reply about the harbour and the keeper, with weather and a decision.\n\n[status]\nhp: 70/100\nmood: wary\n[/status]",
		"Another reply, shorter.\n[status]\nhp: 65/100\nmood：calm\n",
		"No block here at all, just prose that goes on for a while to make the ratio small.",
	}
	h := ReplayHealth(replies, ReplayOptions{RequiredKeys: []string{"hp", "mood", "time"}})
	if h.WithBlock != 2 || h.MissingClose != 1 || h.FullWidthLines != 1 || h.Keys["hp"].Count != 2 || h.Keys["mood"].Count != 2 {
		t.Fatalf("%+v", h)
	}
	if strings.Join(h.RequiredKeysBelow, ",") != "hp,mood,time" {
		t.Fatalf("below: %v", h.RequiredKeysBelow)
	}
	if h.Overhead <= 0 || h.Overhead >= 1 {
		t.Fatalf("overhead %v", h.Overhead)
	}
	// a kit card's rules consume [status]: keys still tallied, characters counted once; a
	// card's own marker reports its share
	k := ReplayHealth(replies, ReplayOptions{RequiredKeys: []string{"hp", "mood"}, Markers: []Marker{{Name: "status"}, {Name: "choices"}}})
	if k.WithBlock != 2 || k.MissingClose != 1 || k.Keys["hp"].Count != 2 || k.Keys["mood"].Count != 2 {
		t.Fatalf("consumed: %+v", k)
	}
	if d := k.Overhead - h.Overhead; d > 0.02 || d < -0.02 {
		t.Fatalf("overhead %v vs %v", k.Overhead, h.Overhead)
	}
	if k.Markers["status"].Rate < 0.6 || k.Markers["status"].Share <= 0 || k.Markers["choices"].Rate != 0 {
		t.Fatalf("markers: %+v", k.Markers)
	}
	wrapped := []string{"Prose first. <shi>史官曰：有人來了。</shi> More prose.\n[tug]3[/tug]", "Only prose here, nothing else at all."}
	w := ReplayHealth(wrapped, ReplayOptions{Markers: []Marker{{Name: "shi", Angle: true}, {Name: "tug"}}})
	if w.Markers["shi"].Rate != 0.5 || w.Markers["shi"].PerReply != 0.5 || w.Markers["shi"].Share <= 0.1 || w.Markers["tug"].Rate != 0.5 {
		t.Fatalf("wrapped: %+v", w.Markers)
	}
	history := "Conversation x with y (3 messages)\n\n[AI]\nHello.\n[status]\nhp: 1\n[/status]\n\n[USER]\nhi\n\n[AI]\nBye.\n"
	if got := RepliesFrom(history); len(got) != 2 || !strings.HasPrefix(got[0], "Hello.") {
		t.Fatalf("history: %q", got)
	}
	jsonl := "{\"role\":\"user\",\"content\":\"hi\"}\n{\"role\":\"ai\",\"content\":\"[status]\\nhp: 1\\n[/status]\"}\n"
	if got := RepliesFrom(jsonl); len(got) != 1 || !strings.Contains(got[0], "hp: 1") {
		t.Fatalf("jsonl: %q", got)
	}
	// "hearthroom play --history --json": newest first, summary rows skipped.
	played := `{"conversationId":"c","history":{"total":4,"chats":[` +
		`{"chatRole":"AI","chatMessage":"Second.\n[status]\nhp: 2\n[/status]","isFirst":false},` +
		`{"chatRole":"AI","chatMessage":"recap","isSummary":true},` +
		`{"chatRole":"USER","chatMessage":"go on"},` +
		`{"chatRole":"AI","chatMessage":"Opening.","isFirst":true}]}}`
	if got := RepliesFrom(played); len(got) != 2 || got[0] != "Opening." || !strings.HasPrefix(got[1], "Second.") {
		t.Fatalf("play --history --json: %q", got)
	}
	samples := "## one\nFirst reply.\n\n## two\nSecond reply.\n"
	if got := RepliesFrom(samples); len(got) != 2 || got[1] != "Second reply." {
		t.Fatalf("samples: %q", got)
	}
	req, vol := KitFields([]byte(`{"schema":{"fields":[{"key":"hp"},{"key":"danger","volatile":true},{"key":"secret","hidden":true}]}}`))
	if strings.Join(req, ",") != "hp,secret" || strings.Join(vol, ",") != "danger" {
		t.Fatalf("fields: %v %v", req, vol)
	}
}

// The provider files an undeclared card as zh-Hant and cannot tell zh-Hant
// from zh-Hans by "zh", so check flags the language before push refuses it.
func TestCardLanguageIsRequired(t *testing.T) {
	for _, manifest := range []string{`{"formatVersion":1,"name":"x"}`, `{"formatVersion":1,"name":"x","language":"zh"}`} {
		dir := write(t, t.TempDir(), map[string]string{"card.json": manifest, "README.md": "uiRole: assist\n"})
		r, err := Card(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !has(msgs(r), "error:", "zh-Hant, zh-Hans, en, ja, ko") {
			t.Fatalf("%s: findings %v, want a language error", manifest, msgs(r))
		}
	}
}

// The sandbox shell keeps type="module" on rule scripts, so import/export
// there is valid. A classic <script> with module syntax still fails to parse.
func TestModuleSyntaxAllowedInSandboxModuleScripts(t *testing.T) {
	dir := write(t, t.TempDir(), map[string]string{
		"card.json":  `{"formatVersion":1,"name":"x","language":"en"}`,
		"README.md":  "uiRole: assist\n",
		"rules.json": `{"pageMode":"sandbox","rules":[{"id":"m","name":"m","find":"[[m]]","replace":"<script type=\"module\">import { go } from \"https://assets.harperharbor.com/u/x/lib.mjs\"; go()</script>","enabled":true},{"id":"c","name":"c","find":"[[c]]","replace":"<script>import x from \"y\"</script>","enabled":true}]}`,
	})
	r, _ := Card(dir)
	n := 0
	for _, f := range r.Findings {
		if strings.Contains(f.Msg, "ES module syntax") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("module findings = %d, want 1 (the classic script): %v", n, msgs(r))
	}
}
