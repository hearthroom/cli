package sync

import (
	"github.com/hearthroom/cli/internal/card"
	"strings"
	"testing"
)

func TestCardCSSPreflightBoundsAfterRewriting(t *testing.T) {
	for _, tc := range []struct {
		css, dialect string
		valid        bool
	}{
		{strings.Repeat("x", 524288), "", true},
		{strings.Repeat("界", 174763), "", false},
		{strings.Repeat("x", 524260) + "url('assets/bg.png')", "", false},
		{"p{color:red}", "fengyue", true},
		{"p{color:red}", "unknown", false},
	} {
		f, err := card.Init(t.TempDir(), "Synthetic", "en")
		if err != nil {
			t.Fatal(err)
		}
		f.CardCSS = &tc.css
		f.Rules = &card.Rules{Rules: []card.DisplayRule{}, CardCSSDialect: &tc.dialect}
		p, err := f.Build(map[string]string{"assets/bg.png": "https://cdn.example/library/long-folder/background.png"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = preflight(f, p, "en", true)
		if (err == nil) != tc.valid {
			t.Errorf("valid=%v err=%v", tc.valid, err)
		}
	}
}
