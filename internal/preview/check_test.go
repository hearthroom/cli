package preview

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseSizesAndPlan(t *testing.T) {
	sz, err := ParseSizes("")
	if err != nil || len(sz) != 2 || sz[0].Name != "phone" || sz[1].Name != "desktop" {
		t.Fatalf("default sizes: %v %v", sz, err)
	}
	sz, err = ParseSizes("1280x800, 390x844,768x1024")
	if err != nil {
		t.Fatal(err)
	}
	if sz[1].Name != "phone" || sz[0].Name != "desktop" || sz[2].Name != "768x1024" {
		t.Fatalf("names: %+v", sz)
	}
	if _, err := ParseSizes("abc"); err == nil {
		t.Fatal("abc accepted")
	}
	if _, err := ParseSizes("10x10"); err == nil {
		t.Fatal("tiny accepted")
	}
	// one sample: sizes × (dark on, light on, dark off) = 3 per size
	states := Plan(DefaultSizes, []string{"a"})
	if len(states) != 6 {
		t.Fatalf("one sample: %d states", len(states))
	}
	// two samples: rules-on states double (first + last), rules-off stays first only
	states = Plan(DefaultSizes, []string{"a", "b", "c"})
	if len(states) != 10 {
		t.Fatalf("three samples: %d states", len(states))
	}
	names := map[string]bool{}
	for _, s := range states {
		if names[s.Name] {
			t.Fatalf("duplicate state %s", s.Name)
		}
		names[s.Name] = true
		if s.Sample == "last" && s.Text != "c" {
			t.Fatalf("last sample text %q", s.Text)
		}
		if s.RulesOff && (s.Theme != "dark" || s.Sample != "first") {
			t.Fatalf("rules-off state %s should be dark/first", s.Name)
		}
	}
	if !names["phone-dark-rules-on-first"] || !names["desktop-light-rules-on-last"] || !names["phone-dark-rules-off-first"] {
		t.Fatalf("expected state names missing: %v", names)
	}
}

func TestSplitAndSynthesize(t *testing.T) {
	s := SplitSamples("intro\n\n## one\n\nfirst reply\n\n## two\nsecond\n")
	if len(s) != 3 || s[1] != "first reply" || s[2] != "second" {
		t.Fatalf("split: %q", s)
	}
	contract := "End every reply with [status]\nhp: <current>/<max>\n[/status]\nExample:\n[status]\nhp: 72/100\nmood: wary\n[/status]"
	sample, note := Synthesize(contract)
	if !strings.Contains(sample, "hp: 72/100") || strings.Contains(sample, "<current>") {
		t.Fatalf("synthesized %q", sample)
	}
	if !strings.Contains(note, "example block") {
		t.Fatalf("note %q", note)
	}
	sample, note = Synthesize("just prose")
	if strings.Contains(sample, "[status]") || !strings.Contains(note, "plain line") {
		t.Fatalf("plain: %q %q", sample, note)
	}
}

func state(name, size string, rulesOff bool, panels, raw, choices, hyd int) StateResult {
	r := StateResult{State: State{Name: name, Size: Size{Name: size, W: 390, H: 844}, Theme: "dark", RulesOff: rulesOff, Sample: "first"}, HydrationMs: hyd, ConsoleErrors: []string{}}
	r.Snapshot.StatusPanels = panels
	r.Snapshot.RawPanels = raw
	r.Facts.ChoiceButtons = choices
	r.Snapshot.StoryFirst.StoryTextShare = 30
	r.Snapshot.StoryFirst.UIShare = 10
	r.Snapshot.StoryFirst.FreeInputVisible = true
	return r
}

func levels(f []Finding, level string) []string {
	var out []string
	for _, x := range f {
		if x.Level == level {
			out = append(out, x.Msg)
		}
	}
	return out
}

func TestDerive(t *testing.T) {
	// healthy core card
	ok := Derive(Declared{UIRole: "core"}, []StateResult{
		state("phone-dark-rules-on-first", "phone", false, 1, 0, 3, 120),
		state("phone-dark-rules-off-first", "phone", true, 0, 0, 0, -1),
		state("desktop-dark-rules-on-first", "desktop", false, 1, 0, 3, 90),
	})
	if e := levels(ok, "error"); len(e) != 0 {
		t.Fatalf("healthy card has errors: %v", e)
	}
	if w := levels(ok, "warning"); len(w) != 0 {
		t.Fatalf("healthy card has warnings: %v", w)
	}
	// raw block never drawn, overflow, console error, late panel
	bad := state("phone-dark-rules-on-first", "phone", false, 0, 1, 0, -1)
	bad.Facts.Overflow = true
	bad.ConsoleErrors = []string{"TypeError: x is not a function"}
	late := state("desktop-dark-rules-on-first", "desktop", false, 1, 0, 0, 2600)
	f := Derive(Declared{UIRole: "core"}, []StateResult{bad, late})
	e := levels(f, "error")
	if len(e) != 3 {
		t.Fatalf("expected 3 errors, got %v", e)
	}
	if w := levels(f, "warning"); len(w) != 1 || !strings.Contains(w[0], "2600 ms") {
		t.Fatalf("late panel warning: %v", w)
	}
	// errors sort first
	if f[0].Level != "error" || f[len(f)-1].Level == "error" {
		t.Fatalf("ordering: %+v", f)
	}
	// assist card whose UI drowns the story, and reads as nothing with rules off
	loud := state("phone-dark-rules-on-first", "phone", false, 1, 0, 4, 100)
	loud.Snapshot.StoryFirst.UIShare = 70
	off := state("phone-dark-rules-off-first", "phone", true, 0, 0, 0, -1)
	off.Snapshot.StoryFirst.StoryTextShare = 0
	f = Derive(Declared{UIRole: "assist"}, []StateResult{loud, off})
	w := strings.Join(levels(f, "warning"), " | ")
	if !strings.Contains(w, "70%") || !strings.Contains(w, "rules off") {
		t.Fatalf("assist warnings: %s", w)
	}
	// core card with no screen at all
	f = Derive(Declared{UIRole: "core"}, []StateResult{state("phone-dark-rules-on-first", "phone", false, 0, 0, 0, -1)})
	if w := strings.Join(levels(f, "warning"), " | "); !strings.Contains(w, "core screen is missing") {
		t.Fatalf("core missing: %s", w)
	}
	// a state that failed to drive is an error, not a crash
	broken := StateResult{State: State{Name: "phone-dark-rules-on-first"}, Error: "navigate: boom"}
	f = Derive(Declared{}, []StateResult{broken})
	if e := levels(f, "error"); len(e) != 1 || !strings.Contains(e[0], "boom") {
		t.Fatalf("broken state: %v", e)
	}
}

func TestContactAndRoutes(t *testing.T) {
	shots := t.TempDir()
	if err := os.WriteFile(filepath.Join(shots, "a.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := state("phone-dark-rules-on-first", "phone", false, 1, 0, 2, 150)
	s.Shot = "a.png"
	html := ContactHTML([]StateResult{s, {State: State{Name: "x"}, Error: "navigate: nope"}})
	for _, want := range []string{`src="/shots/a.png"`, "panel after 150 ms", "2 choice(s)", "navigate: nope"} {
		if !strings.Contains(html, want) {
			t.Fatalf("contact html lacks %q", want)
		}
	}
	card := t.TempDir()
	h := HandlerWith(fakeShell(t), card, Options{ShotsDir: shots, Samples: []string{"one", "two"}, Contact: func() string { return html }})
	srv := httptest.NewServer(h)
	defer srv.Close()
	get := func(p string) (int, string) {
		r, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		return r.StatusCode, string(b)
	}
	if code, body := get("/shots/a.png"); code != 200 || body != "png" {
		t.Fatalf("/shots/: %d %q", code, body)
	}
	if code, body := get("/contact.html"); code != 200 || !strings.Contains(body, "/shots/a.png") {
		t.Fatalf("/contact.html: %d", code)
	}
	if code, body := get("/card/preview/replies.md"); code != 200 || !strings.Contains(body, "## reply 2\n\ntwo") {
		t.Fatalf("/card/preview/replies.md override: %d %q", code, body)
	}
	if code, _ := get("/shots/../card.json"); code == 200 {
		t.Fatal("path escape served")
	}
}

// TestDriveIntegration runs a real headless Chrome against a local shell build.
// HEARTHROOM_TEST_CHROME=1 and HEARTHROOM_TEST_SHELL=<dist-sandbox dir> enable it.
func TestDriveIntegration(t *testing.T) {
	if os.Getenv("HEARTHROOM_TEST_CHROME") != "1" {
		t.Skip("set HEARTHROOM_TEST_CHROME=1 (and HEARTHROOM_TEST_SHELL) to run")
	}
	shell := os.Getenv("HEARTHROOM_TEST_SHELL")
	if shell == "" {
		t.Skip("HEARTHROOM_TEST_SHELL not set")
	}
	chrome, err := FindChrome()
	if err != nil {
		t.Skip(err)
	}
	card := t.TempDir()
	must := func(name, body string) {
		if err := os.WriteFile(filepath.Join(card, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("card.json", `{"name":"T","outputContract":"[status]\nhp: 1/2\n[/status]"}`)
	must("welcome.md", "她看了你一眼。\n\n[status]\nhp: 2/2\n[/status]")
	must("rules.json", `{"pageMode":"sandbox","rules":[]}`)
	shots := filepath.Join(card, "preview", "shots")
	var results []StateResult
	srv, err := Listen(shell, card, 0, Options{ShotsDir: shots, Contact: func() string { return ContactHTML(results) }})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	states := Plan([]Size{{"phone", 390, 844}}, []string{"回覆。\n\n[status]\nhp: 1/2\n[/status]"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	results, err = Run(ctx, DriveOptions{Chrome: chrome, Origin: srv.Origin, States: states, ShotsDir: shots})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Error != "" {
			t.Fatalf("%s: %s", r.Name, r.Error)
		}
		if _, err := os.Stat(filepath.Join(shots, r.Shot)); err != nil {
			t.Fatalf("%s: shot missing", r.Name)
		}
		if r.Snapshot.Bubbles < 2 {
			t.Fatalf("%s: %d bubbles", r.Name, r.Snapshot.Bubbles)
		}
	}
	if err := Contact(ctx, chrome, srv.Origin, filepath.Join(shots, "contact.png")); err != nil {
		t.Fatal(err)
	}
}
