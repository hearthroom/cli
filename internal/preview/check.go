package preview

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// --check gives an agent without a browser tool the two things a browser would: a picture
// of each state that matters and the facts a picture does not show (when the panel was
// drawn, whether the phone width overflows, what the card's scripts logged). The states
// are fixed so two runs of the same card produce files with the same names and can be
// compared.

// Size is a viewport the shots are taken at.
type Size struct {
	Name string `json:"name"`
	W    int    `json:"w"`
	H    int    `json:"h"`
}

// DefaultSizes are the two widths a card must work at.
var DefaultSizes = []Size{{"phone", 390, 844}, {"desktop", 1280, 800}}

// ParseSizes reads "390x844,1280x800" (names are derived: the narrowest is "phone",
// the widest "desktop", others "WxH").
func ParseSizes(spec string) ([]Size, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return DefaultSizes, nil
	}
	var out []Size
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		wh := strings.Split(strings.ToLower(part), "x")
		if len(wh) != 2 {
			return nil, fmt.Errorf("size %q is not WxH", part)
		}
		w, err1 := strconv.Atoi(wh[0])
		h, err2 := strconv.Atoi(wh[1])
		if err1 != nil || err2 != nil || w < 200 || h < 200 || w > 4000 || h > 4000 {
			return nil, fmt.Errorf("size %q is not WxH between 200 and 4000", part)
		}
		out = append(out, Size{Name: part, W: w, H: h})
	}
	if len(out) == 0 {
		return DefaultSizes, nil
	}
	narrow, wide := 0, 0
	for i, s := range out {
		if s.W < out[narrow].W {
			narrow = i
		}
		if s.W > out[wide].W {
			wide = i
		}
	}
	out[narrow].Name = "phone"
	if wide != narrow {
		out[wide].Name = "desktop"
	}
	return out, nil
}

// State is one shot: a size, a theme, rules on or off, and which sample reply is streamed.
type State struct {
	Name     string `json:"name"`
	Size     Size   `json:"size"`
	Theme    string `json:"theme"`
	RulesOff bool   `json:"rulesOff"`
	Sample   string `json:"sample"` // first | last
	Text     string `json:"-"`      // the reply streamed in this state
}

// Plan lists the states for the given samples: sizes × dark/light × rules on/off × first/last.
// With one sample "last" is skipped; the rules-off states only need the dark theme
// (text-only reading does not change with the theme).
func Plan(sizes []Size, samples []string) []State {
	var states []State
	if len(samples) == 0 {
		samples = []string{"（預覽用回覆）"}
	}
	picks := []struct{ name, text string }{{"first", samples[0]}}
	if len(samples) > 1 {
		picks = append(picks, struct{ name, text string }{"last", samples[len(samples)-1]})
	}
	for _, sz := range sizes {
		for _, theme := range []string{"dark", "light"} {
			for _, off := range []bool{false, true} {
				if off && theme != "dark" {
					continue
				}
				for _, p := range picks {
					if off && p.name != "first" {
						continue
					}
					rules := "on"
					if off {
						rules = "off"
					}
					states = append(states, State{
						Name:     fmt.Sprintf("%s-%s-rules-%s-%s", sz.Name, theme, rules, p.name),
						Size:     sz,
						Theme:    theme,
						RulesOff: off,
						Sample:   p.name,
						Text:     p.text,
					})
				}
			}
		}
	}
	return states
}

// SplitSamples reads preview/replies.md: one sample per "## " heading.
func SplitSamples(md string) []string {
	var out []string
	for _, part := range regexp.MustCompile(`(?m)^## .*$`).Split(md, -1) {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

var exampleBlock = regexp.MustCompile(`(?s)\[status\].*?\[/status\]`)

// Synthesize makes a sample reply for a card that ships none: the example block from the
// output contract when it has one, otherwise a short plain line. The note says which.
func Synthesize(outputContract string) (sample string, note string) {
	blocks := exampleBlock.FindAllString(outputContract, -1)
	if len(blocks) > 0 {
		// the last block in the contract is the worked example ("Example of an ordinary turn")
		b := blocks[len(blocks)-1]
		if strings.Contains(b, "<") && strings.Contains(b, ">") {
			// a shape block (<current>/<max>); fall back to the first block without placeholders
			for _, c := range blocks {
				if !strings.Contains(c, "<") {
					b = c
					break
				}
			}
		}
		return "她看了你一眼，沒有說話，只是把杯子推到你面前。\n\n" + b, "no preview/replies.md; streamed the output contract's example block"
	}
	return "她看了你一眼，沒有說話，只是把杯子推到你面前。", "no preview/replies.md and no [status] example in the output contract; streamed one plain line"
}

// Snapshot is what the harness's __preview.snapshot() returns.
type Snapshot struct {
	StoryFirst struct {
		Viewport             string  `json:"viewport"`
		StoryTextShare       float64 `json:"storyTextShare"`
		UIShare              float64 `json:"uiShare"`
		InteractiveAboveFold int     `json:"interactiveAboveFold"`
		FirstChoiceAboveFold bool    `json:"firstChoiceAboveFold"`
		FreeInputVisible     bool    `json:"freeInputVisible"`
		RulesOff             bool    `json:"rulesOff"`
	} `json:"storyFirst"`
	ComposerValue *string  `json:"composerValue"`
	Bubbles       int      `json:"bubbles"`
	StatusPanels  int      `json:"statusPanels"`
	RawPanels     int      `json:"rawPanels"`
	Stage         *string  `json:"stage"`
	Theme         *string  `json:"theme"`
	LastBody      string   `json:"lastBody"`
	LogTail       []string `json:"logTail"`
}

// Facts are read from the shell's document after the reply settled.
type Facts struct {
	Overflow      bool `json:"overflow"`      // documentElement.scrollWidth > innerWidth
	ChoiceButtons int  `json:"choiceButtons"` // .hr-choice (not ✎) + .lt-choice
	NativePanels  int  `json:"nativePanels"`  // .lt-status drawn by the page itself
	SmallText     int  `json:"smallText"`     // elements in message bodies under 12px
}

// StateResult is one driven state.
type StateResult struct {
	State
	Shot          string   `json:"shot"`
	Snapshot      Snapshot `json:"snapshot"`
	Facts         Facts    `json:"facts"`
	HydrationMs   int      `json:"hydrationMs"` // ms from stream end to a drawn panel; -1 never
	ConsoleErrors []string `json:"consoleErrors"`
	Error         string   `json:"error,omitempty"` // the drive failed for this state
}

// Finding mirrors internal/check's shape so --json readers see one vocabulary.
type Finding struct {
	Level string `json:"level"` // error | warning | info
	Where string `json:"where"`
	Msg   string `json:"msg"`
}

// Report is the --check document.
type Report struct {
	Status   string        `json:"status"` // ok | error
	Card     string        `json:"card"`
	ShotsDir string        `json:"shotsDir"`
	Contact  string        `json:"contact,omitempty"`
	Notes    []string      `json:"notes,omitempty"`
	States   []StateResult `json:"states"`
	Findings []Finding     `json:"findings"`
}

// Errors counts the blocking findings.
func (r *Report) Errors() int {
	n := 0
	for _, f := range r.Findings {
		if f.Level == "error" {
			n++
		}
	}
	return n
}

// Declared is what the card's README says (uiRole assist | core | "").
type Declared struct {
	UIRole string
}

// Derive turns the driven states into findings. Rules-on states carry the panel and
// choices facts; the phone rules-off state carries the text-only reading check.
func Derive(decl Declared, states []StateResult) []Finding {
	var out []Finding
	add := func(level, where, msg string) { out = append(out, Finding{Level: level, Where: where, Msg: msg}) }
	seenOverflow, seenRaw, seenLate, seenConsole := false, false, false, false
	anyPanel, anyChoices, anyRulesOn := false, false, false
	for _, s := range states {
		where := "preview/" + s.Name
		if s.Error != "" {
			add("error", where, "this state could not be driven: "+s.Error)
			continue
		}
		panels := s.Snapshot.StatusPanels + s.Facts.NativePanels
		if !s.RulesOff {
			anyRulesOn = true
			if panels > 0 {
				anyPanel = true
			}
			if s.Facts.ChoiceButtons > 0 {
				anyChoices = true
			}
			if s.Snapshot.RawPanels > 0 && panels == 0 && !seenRaw {
				seenRaw = true
				add("error", where, "the reply carries a status block but no panel was drawn for it: the block's rule did not run or matched nothing (the player sees raw text)")
			}
			if panels > 0 {
				switch {
				case s.HydrationMs > 2000 && !seenLate:
					seenLate = true
					add("warning", where, fmt.Sprintf("the status panel appeared %d ms after the reply finished; a player sees raw text first (hydrate on message:done, not on a timer)", s.HydrationMs))
				case s.HydrationMs >= 0 && s.Sample == "first" && s.Theme == "dark":
					add("info", where, fmt.Sprintf("status panel drawn %d ms after the reply finished", s.HydrationMs))
				}
			}
			if s.Sample == "first" && s.Theme == "dark" {
				sf := s.Snapshot.StoryFirst
				add("info", where, fmt.Sprintf("story text %.0f%% of the screen, UI %.0f%%; %d choice button(s); free input %s", sf.StoryTextShare, sf.UIShare, s.Facts.ChoiceButtons, visibleWord(sf.FreeInputVisible)))
				if s.Snapshot.Stage != nil && *s.Snapshot.Stage != "" && *s.Snapshot.Stage != "closed" {
					add("info", where, fmt.Sprintf("an author stage (%s) covers the chat in this shot; the streamed reply is behind it and --check does not tap through it", *s.Snapshot.Stage))
				}
				if decl.UIRole == "assist" && sf.UIShare > 50 {
					add("warning", where, fmt.Sprintf("README declares uiRole assist but the UI covers %.0f%% of the first screen; the story is not what the player sees first", sf.UIShare))
				}
			}
		}
		if s.Size.Name == "phone" && s.Facts.Overflow && !seenOverflow {
			seenOverflow = true
			add("error", where, "the page scrolls sideways at phone width: something is wider than the screen (a fixed-width panel, a long unbroken token, an image without max-width)")
		}
		if len(s.ConsoleErrors) > 0 && !seenConsole {
			seenConsole = true
			first := s.ConsoleErrors[0]
			if len([]rune(first)) > 160 {
				first = string([]rune(first)[:160]) + "…"
			}
			add("error", where, fmt.Sprintf("the card's scripts logged %d error(s); first: %s", len(s.ConsoleErrors), first))
		}
		if s.Facts.SmallText > 0 && s.Size.Name == "phone" && !s.RulesOff && s.Sample == "first" && s.Theme == "dark" {
			add("warning", where, fmt.Sprintf("%d element(s) in the reply are set under 12px; hard to read on a phone", s.Facts.SmallText))
		}
		if s.RulesOff && s.Size.Name == "phone" {
			sf := s.Snapshot.StoryFirst
			if decl.UIRole == "assist" && sf.StoryTextShare == 0 {
				add("warning", where, "README declares uiRole assist but with the rules off the first screen shows no story text; the card does not read as plain text")
			} else {
				add("info", where, fmt.Sprintf("rules off: story text %.0f%% of the phone screen", sf.StoryTextShare))
			}
		}
	}
	if decl.UIRole == "core" && anyRulesOn && !anyPanel && !anyChoices {
		add("warning", "preview", "README declares uiRole core but no status panel and no choice button appeared with the rules on; the core screen is missing")
	}
	sort.SliceStable(out, func(i, j int) bool { return levelRank(out[i].Level) < levelRank(out[j].Level) })
	return out
}

func levelRank(l string) int {
	switch l {
	case "error":
		return 0
	case "warning":
		return 1
	}
	return 2
}

func visibleWord(b bool) string {
	if b {
		return "visible"
	}
	return "not visible"
}

// ContactHTML lays every shot out in two columns with its numbers, for one screenshot
// an agent can read at a glance. Image paths are relative to the preview origin.
func ContactHTML(states []StateResult) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>contact</title><style>
body{margin:0;background:#fff;color:#111;font:13px/1.4 system-ui,sans-serif}
h1{font-size:15px;margin:16px 20px 8px}
.grid{display:grid;grid-template-columns:1fr 1fr;gap:16px 20px;padding:0 20px 20px}
.cell{border:1px solid #ddd;border-radius:6px;overflow:hidden;background:#fafafa}
.cell img{display:block;width:100%;height:auto;background:#000}
.cap{padding:8px 10px;border-top:1px solid #ddd}
.cap b{font-family:ui-monospace,monospace;font-size:12px}
.cap span{color:#555;display:block;margin-top:2px}
.err{color:#b00020}
</style></head><body><h1>card preview --check</h1><div class="grid">`)
	for _, s := range states {
		b.WriteString(`<div class="cell">`)
		if s.Shot != "" {
			fmt.Fprintf(&b, `<img src="/shots/%s" alt="%s">`, html.EscapeString(s.Shot), html.EscapeString(s.Name))
		}
		fmt.Fprintf(&b, `<div class="cap"><b>%s</b>`, html.EscapeString(s.Name))
		if s.Error != "" {
			fmt.Fprintf(&b, `<span class="err">%s</span>`, html.EscapeString(s.Error))
		} else {
			sf := s.Snapshot.StoryFirst
			panels := s.Snapshot.StatusPanels + s.Facts.NativePanels
			hyd := "no panel"
			if panels > 0 {
				hyd = fmt.Sprintf("panel after %d ms", s.HydrationMs)
			}
			over := ""
			if s.Facts.Overflow {
				over = ` · <span class="err" style="display:inline">sideways overflow</span>`
			}
			stage := ""
			if s.Snapshot.Stage != nil && *s.Snapshot.Stage != "" && *s.Snapshot.Stage != "closed" {
				stage = " · stage " + html.EscapeString(*s.Snapshot.Stage)
			}
			fmt.Fprintf(&b, `<span>%s · story %.0f%% / UI %.0f%% · %d choice(s) · input %s%s%s</span>`, hyd, sf.StoryTextShare, sf.UIShare, s.Facts.ChoiceButtons, visibleWord(sf.FreeInputVisible), over, stage)
			if len(s.ConsoleErrors) > 0 {
				fmt.Fprintf(&b, `<span class="err">%d console error(s)</span>`, len(s.ConsoleErrors))
			}
		}
		b.WriteString(`</div></div>`)
	}
	b.WriteString(`</div></body></html>`)
	return b.String()
}

// MarshalReport is the --json document.
func MarshalReport(r *Report) ([]byte, error) { return json.MarshalIndent(r, "", "  ") }
