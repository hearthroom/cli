package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/card"
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
	if f.CardCSS == nil || *f.CardCSS != " #ai-chat-input{color:red} " {
		t.Fatalf("card.css missing: %v", f.CardCSS)
	}
	if f.Rules == nil || len(f.Rules.Rules) != 0 || f.Rules.CardCSSDialect == nil || *f.Rules.CardCSSDialect != "fengyue-v2" {
		t.Fatalf("fake CSS rule retained: %+v", f.Rules)
	}
}

func TestFengyueCSSPreservesRawBytesThroughFolder(t *testing.T) {
	for _, tc := range []struct{ name, css string }{
		{"LF", "\n#ai-chat-input{color:red}\n\n"},
		{"CRLF", "\r\n#ai-chat-input{\r\n color:red;\r\n}\r\n"},
		{"edge spaces", " \t#ai-chat-input{color:red}\t "},
		{"whitespace only", "\r\n \t\r\n"},
		{"UTF-8 BOM", "\ufeff\r\n#ai-chat-input{color:red}\r\n"},
		{"empty", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := FromFengyue(map[string]any{"title": "Synthetic", "pre_prompt": "fixture", "builtInCss": tc.css}, "en")
			if r.CardCSS != tc.css {
				t.Errorf("imported CSS bytes=%q want %q", r.CardCSS, tc.css)
			}
			dir := t.TempDir()
			if _, err := WriteFolder(r, dir, false); err != nil {
				t.Fatal(err)
			}
			f, err := card.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, card.CardCSSFile)
			if tc.css == "" {
				if _, err := os.Stat(path); !os.IsNotExist(err) || f.CardCSS != nil || r.CardCSSDialect != "" {
					t.Fatalf("empty source must remain absent: css=%v dialect=%q stat=%v", f.CardCSS, r.CardCSSDialect, err)
				}
				return
			}
			assertFile := func() {
				t.Helper()
				got, err := os.ReadFile(path)
				if err != nil || string(got) != tc.css {
					t.Errorf("card.css bytes=%q want %q: %v", got, tc.css, err)
				}
			}
			assertFile()
			if f.CardCSS == nil || *f.CardCSS != tc.css || f.Rules == nil || f.Rules.CardCSSDialect == nil || *f.Rules.CardCSSDialect != "fengyue-v2" {
				t.Fatalf("loaded CSS bytes or import hint changed: css=%v rules=%+v", f.CardCSS, f.Rules)
			}
			if err := f.Save(); err != nil {
				t.Fatal(err)
			}
			assertFile()
			payload, err := f.Build(nil)
			if err != nil || payload.AuthorAsset["cardCss"] != tc.css {
				t.Fatalf("payload must preserve source bytes: %+v %v", payload.AuthorAsset, err)
			}
		})
	}
}

func TestFengyueCSSFolderStillRejectsInvalidUTF8(t *testing.T) {
	r := FromFengyue(map[string]any{"title": "Synthetic", "pre_prompt": "fixture", "builtInCss": "p{color:red}"}, "en")
	dir := t.TempDir()
	if _, err := WriteFolder(r, dir, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, card.CardCSSFile), []byte{0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := card.Load(dir); err == nil || !strings.Contains(err.Error(), "card.css: not valid UTF-8") {
		t.Fatalf("CSS UTF-8 validation changed: %v", err)
	}
}
