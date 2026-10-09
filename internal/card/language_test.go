package card

import (
	"strings"
	"testing"
)

func TestCheckLanguage(t *testing.T) {
	for _, l := range Languages {
		if err := CheckLanguage(l); err != nil {
			t.Fatalf("%s: %v", l, err)
		}
	}
	for _, l := range []string{"", "zh", "zh-TW", "zh-hant", "en-US", "fr"} {
		err := CheckLanguage(l)
		if err == nil {
			t.Fatalf("%q accepted", l)
		}
		if !strings.Contains(err.Error(), "zh-Hant, zh-Hans, en, ja, ko") {
			t.Fatalf("%q: error does not list the choices: %v", l, err)
		}
	}
}
