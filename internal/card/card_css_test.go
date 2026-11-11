package card

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCardCSSTravelsWithTheAuthorAsset(t *testing.T) {
	f := sample(t)
	f.Rules = &Rules{Rules: []DisplayRule{}}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	loadPayload := func() Payload {
		t.Helper()
		loaded, err := Load(f.Dir)
		if err != nil {
			t.Fatal(err)
		}
		p, err := loaded.Build(nil)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := loadPayload()
	if _, ok := p.AuthorAsset["cardCss"]; ok {
		t.Fatal("missing card.css must preserve remote CSS")
	}
	before := p.Digests()[SectionAuthorAsset]
	css := "p{background:url('assets/bg.png')}\n"
	if err := os.WriteFile(filepath.Join(f.Dir, "card.css"), []byte(css), 0644); err != nil {
		t.Fatal(err)
	}
	p = loadPayload()
	if p.AuthorAsset["cardCss"] != css || p.Digests()[SectionAuthorAsset] == before {
		t.Fatalf("card CSS lost: %v", p.AuthorAsset)
	}
	if err := os.WriteFile(filepath.Join(f.Dir, "card.css"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	p = loadPayload()
	if got, ok := p.AuthorAsset["cardCss"]; !ok || got != "" {
		t.Fatalf("empty file must clear: %v", p.AuthorAsset)
	}

	var a RemoteAsset
	json.Unmarshal([]byte(`{"rules":[],"cardCss":"#ai-chat-input{color:red}","cardCssDialect":"fengyue"}`), &a)
	pulled := FromRemote(t.TempDir(), RemoteDetail{RoleName: "Synthetic"}, "", "", nil, &a)
	if err := pulled.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(pulled.Dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err = loaded.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.AuthorAsset["cardCss"] != "#ai-chat-input{color:red}" || p.AuthorAsset["cardCssDialect"] != "fengyue" {
		t.Fatalf("pull/push lost CSS: %v", p.AuthorAsset)
	}
}

func TestLegacyCardCSSMigrationIsConservative(t *testing.T) {
	base := DisplayRule{ID: legacyFengyueCSSID, Find: `/([\s\S])$/`, Replace: "$1<style>p{color:red}</style>", Enabled: true}
	for _, tc := range []struct {
		name    string
		edit    func(*Folder)
		migrate bool
	}{
		{"known", func(*Folder) {}, true},
		{"mmd hint", func(f *Folder) { f.Rules.Rules[0].CSSDialect = "mmd" }, true},
		{"tavern hint", func(f *Folder) { f.Rules.Rules[0].CSSDialect = "tavern" }, true},
		{"explicit empty", func(f *Folder) { s := ""; f.CardCSS = &s }, false},
		{"disabled", func(f *Folder) { f.Rules.Rules[0].Enabled = false }, false},
		{"generated CLI id", func(f *Folder) { f.Rules.Rules[0].ID = "rule-01" }, false},
		{"edited match", func(f *Folder) { f.Rules.Rules[0].Find = "x" }, false},
		{"extra HTML", func(f *Folder) { f.Rules.Rules[0].Replace = "$1<style></style><script>x()</script><style></style>" }, false},
		{"duplicate", func(f *Folder) { f.Rules.Rules = append(f.Rules.Rules, base) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &Folder{Rules: &Rules{Rules: []DisplayRule{base}}}
			tc.edit(f)
			before := len(f.Rules.Rules)
			f.migrateLegacyCardCSS()
			if tc.migrate {
				if f.CardCSS == nil || *f.CardCSS != "p{color:red}" || len(f.Rules.Rules) != before-1 {
					t.Fatalf("migration failed: %+v", f)
				}
				want := base.CSSDialect
				if f.Rules.CardCSSDialect == nil {
					t.Fatal("hint lost")
				}
				if tc.name == "known" {
					want = "fengyue"
				} else {
					want = strings.Split(tc.name, " ")[0]
				}
				if *f.Rules.CardCSSDialect != want {
					t.Fatalf("hint=%s", *f.Rules.CardCSSDialect)
				}
			} else if len(f.Rules.Rules) != before {
				t.Fatal("edited legacy rule was removed")
			}
		})
	}
}

func TestCardCSSWithoutRulesAndAssetRewrite(t *testing.T) {
	css := "p{background:url('assets/bg.png')}"
	f := sample(t)
	f.CardCSS = &css
	f.Rules = nil
	p, err := f.Build(map[string]string{"assets/bg.png": "https://cdn.example/bg.png"})
	if err != nil {
		t.Fatal(err)
	}
	if p.AuthorAsset["cardCss"] != "p{background:url('https://cdn.example/bg.png')}" {
		t.Fatalf("rewrite=%v", p.AuthorAsset)
	}
	_, missing := f.AssetRefs()
	if !strings.Contains(strings.Join(missing, "\n"), "assets/bg.png") {
		t.Fatalf("missing=%v", missing)
	}
}
