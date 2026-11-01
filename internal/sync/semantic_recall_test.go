package sync

import (
	"github.com/hearthroom/cli/internal/card"
	"strings"
	"testing"
)

func TestSemanticRecallOffFollowsEverySplitPiece(t *testing.T) {
	off := false
	f := &card.Folder{Manifest: card.Manifest{FormatVersion: 1, Name: "Synthetic"}, Lorebook: &card.Lorebook{Entries: []card.LorebookEntry{{Name: "Secret", Content: strings.Repeat("Later reveal. ", 20), Keywords: []string{"reveal"}, SemanticRecall: &off}}}}
	payload, err := f.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	prepared, _, _ := prepareEntries(f, payload.Worldbook, 40)
	entries := prepared["entries"].([]map[string]any)
	if len(entries) < 2 {
		t.Fatal("fixture did not split")
	}
	for _, entry := range entries {
		if entry["semanticRecall"] != false {
			t.Fatalf("split lost off: %v", entry)
		}
	}
	if !matchesEntry(map[string]any{"semanticRecall": false}, card.RemoteEntry{SemanticRecall: &off}) {
		t.Fatal("resume does not recognize saved off")
	}
}
