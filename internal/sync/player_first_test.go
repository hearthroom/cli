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
	}
	for _, c := range cases {
		if got := playerSpeaksFirst(&c.folder, c.welcome); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
