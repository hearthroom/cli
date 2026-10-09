package card

import (
	"fmt"
	"strings"
)

// Languages a card can declare in card.json. The provider stores the card's
// own text under it, files the card in that language's leaderboard, and
// converts Chinese script from it to the player's (zh-Hant and zh-Hans are
// told apart, so a bare "zh" is not enough).
var Languages = []string{"zh-Hant", "zh-Hans", "en", "ja", "ko"}

// CheckLanguage reports a missing or unsupported card.json language.
func CheckLanguage(lang string) error {
	for _, l := range Languages {
		if lang == l {
			return nil
		}
	}
	if lang == "" {
		return fmt.Errorf(`"language" is required: one of %s`, strings.Join(Languages, ", "))
	}
	return fmt.Errorf(`language %q is not supported: use one of %s`, lang, strings.Join(Languages, ", "))
}
