package check

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/card"
)

// The limit is read from the embedded contract; a missing or renamed key would silently disable the check.
func TestContractOpeningPageLimit(t *testing.T) {
	if got := ContractFacts().Provider.OpeningPageMaxBytes; got <= 0 {
		t.Fatalf("openingPageMaxBytes = %d; the embedded contract must state the opening page limit", got)
	}
}

// The check measures what push sends: opening.html after the stored asset URLs replace assets/… paths.
func TestOpeningPageCheckBounds(t *testing.T) {
	const assetURL = "https://cdn.example/library/long-folder/background.png"
	max := ContractFacts().Provider.OpeningPageMaxBytes
	for _, tc := range []struct {
		name  string
		page  string
		valid bool
	}{
		{"at limit", strings.Repeat("x", max), true},
		{"one byte over", strings.Repeat("x", max+1), false},
		{"multibyte over", strings.Repeat("界", max/3+1), false},
		{"over after rewrite", strings.Repeat("x", max-len(`<img src="assets/bg.png">`)) + `<img src="assets/bg.png">`, false},
		{"at limit after rewrite", strings.Repeat("x", max-len(`<img src="`+assetURL+`">`)) + `<img src="assets/bg.png">`, true},
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
				for _, want := range []string{fmt.Sprint(max), card.HumanBytes(max), "card.css", "hearthroom media upload", "assets/"} {
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
	dir := write(t, t.TempDir(), map[string]string{"card.json": `{"formatVersion":1,"name":"Synthetic","language":"en"}`, "rules.json": `{not json`, "opening.html": strings.Repeat("x", ContractFacts().Provider.OpeningPageMaxBytes+1)})
	if _, err := os.Stat(filepath.Join(dir, "opening.html")); err != nil {
		t.Fatal(err)
	}
	r, err := Card(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range r.Findings {
		found = found || (x.Where == card.OpeningFile && strings.Contains(x.Msg, fmt.Sprint(ContractFacts().Provider.OpeningPageMaxBytes+1)))
	}
	if !found {
		t.Fatalf("no opening page finding: %v", r.Findings)
	}
}
