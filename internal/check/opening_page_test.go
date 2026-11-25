package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/card"
)

func TestContractOpeningPageLimit(t *testing.T) {
	if got := ContractFacts().Provider.OpeningPageMaxBytes; got != 524288 {
		t.Fatalf("openingPageMaxBytes = %d, want 524288", got)
	}
}

// The check measures what push sends: opening.html after the stored asset URLs replace assets/… paths.
func TestOpeningPageCheckBounds(t *testing.T) {
	const assetURL = "https://cdn.example/library/long-folder/background.png"
	for _, tc := range []struct {
		name  string
		page  string
		valid bool
	}{
		{"at limit", strings.Repeat("x", 524288), true},
		{"one byte over", strings.Repeat("x", 524289), false},
		{"multibyte over", strings.Repeat("界", 174763), false},
		{"over after rewrite", strings.Repeat("x", 524260) + `<img src="assets/bg.png">`, false},
		{"at limit after rewrite", strings.Repeat("x", 524288-len(`<img src="`+assetURL+`">`)) + `<img src="assets/bg.png">`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := card.Init(t.TempDir(), "Synthetic", "en")
			if err != nil {
				t.Fatal(err)
			}
			f.State.Assets = map[string]card.Asset{"assets/bg.png": {SHA256: "x", URL: assetURL}}
			if err := f.SaveState(); err != nil {
				t.Fatal(err)
			}
			write(t, f.Dir, map[string]string{"opening.html": tc.page, "rules.json": `{"rules":[]}`, "assets/bg.png": "png"})
			r, err := Card(f.Dir)
			if err != nil {
				t.Fatal(err)
			}
			var hit *Finding
			for i, x := range r.Findings {
				if x.Where == card.OpeningFile && x.Level == "error" {
					hit = &r.Findings[i]
				}
			}
			if (hit == nil) != tc.valid {
				t.Fatalf("valid=%v findings=%v", tc.valid, r.Findings)
			}
			if hit != nil {
				for _, want := range []string{"524288", "512 KB", "card.css", "hearthroom media upload", "assets/"} {
					if !strings.Contains(hit.Msg, want) {
						t.Errorf("message lacks %q: %s", want, hit.Msg)
					}
				}
			}
		})
	}
}

// A folder that does not load still has its opening page measured as written.
func TestOpeningPageCheckWithoutLoadableFolder(t *testing.T) {
	dir := write(t, t.TempDir(), map[string]string{"card.json": `{"formatVersion":1,"name":"Synthetic","language":"en"}`, "rules.json": `{not json`, "opening.html": strings.Repeat("x", 524289)})
	if _, err := os.Stat(filepath.Join(dir, "opening.html")); err != nil {
		t.Fatal(err)
	}
	r, err := Card(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range r.Findings {
		found = found || (x.Where == card.OpeningFile && strings.Contains(x.Msg, "524289"))
	}
	if !found {
		t.Fatalf("no opening page finding: %v", r.Findings)
	}
}
