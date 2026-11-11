package importer

import (
	"github.com/hearthroom/cli/internal/card"
	"testing"
)

func TestFengyueCSSUsesCardCSSFile(t *testing.T) {
	r := FromFengyue(map[string]any{"title": "Synthetic", "pre_prompt": "fixture", "builtInCss": " #ai-chat-input{color:red} "}, "en")
	dir := t.TempDir()
	if _, err := WriteFolder(r, dir, false); err != nil {
		t.Fatal(err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.CardCSS == nil || *f.CardCSS != "#ai-chat-input{color:red}" {
		t.Fatalf("card.css missing: %v", f.CardCSS)
	}
	if f.Rules == nil || len(f.Rules.Rules) != 0 || f.Rules.CardCSSDialect == nil || *f.Rules.CardCSSDialect != "fengyue" {
		t.Fatalf("fake CSS rule retained: %+v", f.Rules)
	}
}
