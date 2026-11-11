package sync

import (
	"testing"

	"github.com/hearthroom/cli/internal/card"
)

// An owned card may push an empty welcome.md when the player speaks first: it has opening
// choices, or an opening page pushed with its author asset. The provider applies the same rule.
func TestPlayerSpeaksFirst(t *testing.T) {
	page := "<p>首屏</p>"
	empty := ""
	cases := []struct {
		name    string
		folder  card.Folder
		welcome map[string]any
		want    bool
	}{
		{"nothing", card.Folder{}, map[string]any{"prologue": []string{}}, false},
		{"opening choices", card.Folder{}, map[string]any{"prologue": []string{"我推開門。"}}, true},
		{"blank choices", card.Folder{}, map[string]any{"prologue": []string{"  "}}, false},
		{"opening page with rules.json", card.Folder{OpeningPage: &page, Rules: &card.Rules{}}, map[string]any{}, true},
		{"opening page without rules.json is not pushed", card.Folder{OpeningPage: &page}, map[string]any{}, false},
		{"empty opening page", card.Folder{OpeningPage: &empty, Rules: &card.Rules{}}, map[string]any{}, false},
		{"page presentation without choices", card.Folder{Rules: &card.Rules{OpeningMode: "page"}}, map[string]any{"prologue": []string{}}, true},
		{"greeting presentation without choices", card.Folder{Rules: &card.Rules{OpeningMode: "greeting"}}, map[string]any{}, false},
	}
	for _, c := range cases {
		if got := playerSpeaksFirst(&c.folder, c.welcome); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestLocalCheckAllowsPageOpeningWithoutGreeting(t *testing.T) {
	for _, tc := range []struct {
		mode        string
		wantProblem bool
	}{{"page", false}, {"greeting", true}} {
		f, err := card.Init(t.TempDir(), "Synthetic opening", "en")
		if err != nil {
			t.Fatal(err)
		}
		f.Definition = "Synthetic definition."
		f.Rules = &card.Rules{OpeningMode: tc.mode}
		found := false
		for _, problem := range LocalCheck(f) {
			if problem == card.WelcomeFile+": opening message is empty" {
				found = true
			}
		}
		if found != tc.wantProblem {
			t.Errorf("%s empty greeting problem=%v want=%v", tc.mode, found, tc.wantProblem)
		}
	}
}
