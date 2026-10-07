package card

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResponseDefaultsLoadBuildAndPull(t *testing.T) {
	dir := t.TempDir()
	// lengthTarget may be written as a number; it is sent as the decimal string the API takes.
	manifest := `{"formatVersion": 1, "name": "Mira", "responseDefaults": {"agency": "coauthor", "length": "target", "lengthTarget": 1200, "paceNote": "End on a hook"}}`
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := ResponseDefaults{"agency": "coauthor", "length": "target", "lengthTarget": "1200", "paceNote": "End on a hook"}
	if !reflect.DeepEqual(f.Manifest.ResponseDefaults, want) || f.Manifest.Extra["responseDefaults"] != nil {
		t.Fatalf("loaded %v (extra %v)", f.Manifest.ResponseDefaults, f.Manifest.Extra)
	}
	p, err := f.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := p.Card["responseDefaults"].(map[string]string); !reflect.DeepEqual(got, map[string]string(want)) {
		t.Fatalf("payload = %v", p.Card["responseDefaults"])
	}
	before := p.Digests()[SectionCard]
	f.Manifest.ResponseDefaults["pace"] = "advance"
	if p2, _ := f.Build(nil); p2.Digests()[SectionCard] == before {
		t.Fatal("changing a default must change the card digest")
	}

	// A card without the key sends nothing, so existing digests stay put.
	f.Manifest.ResponseDefaults = nil
	if p3, _ := f.Build(nil); p3.Card["responseDefaults"] != nil {
		t.Fatalf("absent key sent %v", p3.Card["responseDefaults"])
	}
	// An empty object clears what the server has.
	f.Manifest.ResponseDefaults = ResponseDefaults{}
	if p4, _ := f.Build(nil); p4.Card["responseDefaults"] == nil {
		t.Fatal("empty object must be sent to clear")
	}

	pulled := FromRemote(t.TempDir(), RemoteDetail{RoleName: "Mira", ResponseDefaults: map[string]string{"perspective": "third_limited"}}, "", "", nil, nil)
	if err := pulled.Save(); err != nil {
		t.Fatal(err)
	}
	back, err := Load(pulled.Dir)
	if err != nil || back.Manifest.ResponseDefaults["perspective"] != "third_limited" {
		t.Fatalf("pull round trip = %v %v", back.Manifest.ResponseDefaults, err)
	}
}

func TestCheckResponseDefaults(t *testing.T) {
	for _, ok := range []ResponseDefaults{
		nil, {},
		{"agency": "lines", "perspective": "third_omniscient", "pace": "linger"},
		{"length": "target", "lengthTarget": "2000", "lengthNote": "count narration only"},
		{"style": "custom", "customStyle": "Terse noir"},
		{"styleNote": "No metaphors"},
	} {
		if problems := CheckResponseDefaults(ok); len(problems) != 0 {
			t.Fatalf("%v: %v", ok, problems)
		}
	}
	for wantText, bad := range map[string]ResponseDefaults{
		"agency":           {"agency": "everything"},
		"lengthTarget":     {"length": "target"},
		"length is target": {"lengthTarget": "1200"},
		"1200":             {"length": "target", "lengthTarget": "1100"},
		"customStyle":      {"style": "custom"},
		"style is custom":  {"customStyle": "x"},
		"plain":            {"style": "plain"},
		"brief":            {"length": "brief"},
		"blank":            {"paceNote": "  "},
		"200":              {"paceNote": strings.Repeat("字", 201)},
		"unknown key mood": {"mood": "x"},
	} {
		problems := CheckResponseDefaults(bad)
		if len(problems) == 0 || !strings.Contains(strings.Join(problems, "\n"), wantText) {
			t.Fatalf("%v: problems %v, want mention of %q", bad, problems, wantText)
		}
	}
}
