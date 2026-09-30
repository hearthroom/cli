package cli

import (
	"testing"

	"github.com/hearthroom/cli/internal/card"
)

// The language on a turn comes from --language, else from card.json, else nothing.
func TestTurnLanguage(t *testing.T) {
	f := &card.Folder{Manifest: card.Manifest{Language: "zh-Hant"}}
	if got := turnLanguage("", f); got != "zh-Hant" {
		t.Errorf("folder language = %q", got)
	}
	if got := turnLanguage("ja", f); got != "ja" {
		t.Errorf("flag should win: %q", got)
	}
	if got := turnLanguage("", nil); got != "" {
		t.Errorf("no folder, no flag: %q", got)
	}
}
