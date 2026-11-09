package card

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// opening.html is the author asset's opening page: it round-trips through the folder, is pushed
// with rules.json only when the file exists (an empty file clears the page), and pull writes it.
func TestOpeningPageTravelsWithTheAuthorAsset(t *testing.T) {
	f := sample(t)
	f.Rules = &Rules{Rules: []DisplayRule{}, PageMode: "sandbox", CardFormat: "fengyue"}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := loaded.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := payload.AuthorAsset["openingPage"]; ok {
		t.Fatalf("no opening.html must leave the stored page alone: %v", payload.AuthorAsset)
	}
	before := payload.Digests()[SectionAuthorAsset]

	if err := os.WriteFile(filepath.Join(f.Dir, OpeningFile), []byte("<p>首屏</p>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ = loaded.Build(nil)
	if got := payload.AuthorAsset["openingPage"]; got != "<p>首屏</p>\n" || payload.Digests()[SectionAuthorAsset] == before {
		t.Fatalf("openingPage = %q", got)
	}
	if err := os.WriteFile(filepath.Join(f.Dir, OpeningFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, _ = Load(f.Dir)
	payload, _ = loaded.Build(nil)
	if got, ok := payload.AuthorAsset["openingPage"]; !ok || got != "" {
		t.Fatalf("an empty opening.html should clear the page, got %v %v", got, ok)
	}

	var asset RemoteAsset
	json.Unmarshal([]byte(`{"rules":[],"pageMode":"classic","openingPage":"<div>x</div>","cardFormat":"fengyue"}`), &asset)
	pulled := FromRemote(t.TempDir(), RemoteDetail{RoleName: "Mira"}, "", "", nil, &asset)
	if pulled.Rules == nil || pulled.OpeningPage == nil || *pulled.OpeningPage != "<div>x</div>" || pulled.Rules.CardFormat != "fengyue" {
		t.Fatalf("pull: rules %+v page %v", pulled.Rules, pulled.OpeningPage)
	}
}

// rules.json "openingMode" is the opening presentation. It is pushed only when written (folders
// without it keep their digests and the site's choice), and pull writes only page, the non-default.
func TestOpeningModeTravelsWithTheAuthorAsset(t *testing.T) {
	f := sample(t)
	f.Rules = &Rules{Rules: []DisplayRule{}}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, _ := Load(f.Dir)
	payload, _ := loaded.Build(nil)
	if _, ok := payload.AuthorAsset["openingMode"]; ok {
		t.Fatalf("no openingMode must leave the stored presentation alone: %v", payload.AuthorAsset)
	}
	before := payload.Digests()[SectionAuthorAsset]
	loaded.Rules.OpeningMode = "page"
	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, _ = Load(f.Dir)
	payload, _ = loaded.Build(nil)
	if payload.AuthorAsset["openingMode"] != "page" || payload.Digests()[SectionAuthorAsset] == before {
		t.Fatalf("openingMode = %v", payload.AuthorAsset["openingMode"])
	}

	for remote, want := range map[string]string{"page": "page", "greeting": ""} {
		var asset RemoteAsset
		json.Unmarshal([]byte(`{"rules":[],"pageMode":"classic","openingMode":"`+remote+`"}`), &asset)
		pulled := FromRemote(t.TempDir(), RemoteDetail{RoleName: "Mira"}, "", "", nil, &asset)
		got := ""
		if pulled.Rules != nil {
			got = pulled.Rules.OpeningMode
		}
		if got != want || (want == "page") != (pulled.Rules != nil) {
			t.Fatalf("pull %s: rules %+v", remote, pulled.Rules)
		}
	}
}
