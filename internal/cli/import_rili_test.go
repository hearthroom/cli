package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/check"
)

func TestCardImportRiliCardAndLorebook(t *testing.T) {
	b, err := os.ReadFile("../importer/testdata/rili-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Cases []struct {
			Input json.RawMessage `json:"input"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &fixtures); err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cardSrc := filepath.Join(tmp, "TEST-TALKAICC-CARD.json")
	if err := os.WriteFile(cardSrc, fixtures.Cases[0].Input, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{nil, {"--mmd"}} {
		cardOut := filepath.Join(t.TempDir(), "imported-card")
		root := NewRoot(BuildInfo{})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(append([]string{"card", "import", cardSrc, "--language", "zh-Hans", "--out", cardOut}, extra...))
		if err := root.Execute(); err != nil {
			t.Fatalf("card import whole card (%v) failed: %v\n%s", extra, err, out.String())
		}
		manifest, _ := os.ReadFile(filepath.Join(cardOut, "card.json"))
		if !strings.Contains(string(manifest), `"name": "TEST-TALKAICC-CARD"`) {
			t.Fatalf("unexpected card.json (%v):\n%s", extra, manifest)
		}
		rulesJSON, _ := os.ReadFile(filepath.Join(cardOut, "rules.json"))
		if !strings.Contains(string(rulesJSON), `"cardCssDialect": "rili"`) {
			t.Fatalf("unexpected rules.json (%v):\n%s", extra, rulesJSON)
		}
		rep, err := check.Card(cardOut)
		if err != nil {
			t.Fatalf("check.Card: %v", err)
		}
		if rep.Errors() > 0 {
			t.Fatalf("expected 0 check errors on imported Rili card, got %d: %+v", rep.Errors(), rep.Findings)
		}
	}

	// Standalone Rili lorebook.json imports cleanly as a Lorebook folder part.
	lbSrc := filepath.Join(tmp, "lorebook.json")
	if err := os.WriteFile(lbSrc, fixtures.Cases[1].Input, 0o644); err != nil {
		t.Fatal(err)
	}
	lbOut := filepath.Join(tmp, "imported-lorebook")
	root2 := NewRoot(BuildInfo{})
	var out2 bytes.Buffer
	root2.SetOut(&out2)
	root2.SetErr(&out2)
	root2.SetArgs([]string{"card", "import", lbSrc, "--language", "zh-Hans", "--out", lbOut})
	if err := root2.Execute(); err != nil {
		t.Fatalf("card import standalone lorebook failed: %v\n%s", err, out2.String())
	}
	fLb, err := card.Load(lbOut)
	if err != nil {
		t.Fatalf("card.Load lorebook folder: %v", err)
	}
	if fLb.Lorebook == nil || len(fLb.Lorebook.Entries) != 2 {
		t.Fatalf("expected 2 lorebook entries, got %+v", fLb.Lorebook)
	}
}
