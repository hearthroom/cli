package card

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWelcomeFieldsDistinguishAbsentFromEmpty(t *testing.T) {
	for mask := 0; mask < 8; mask++ {
		t.Run(string(rune('0'+mask)), func(t *testing.T) {
			dir := t.TempDir()
			manifest := `{"formatVersion":1,"name":"Synthetic","language":"en"`
			if mask&4 != 0 {
				manifest += `,"prologue":[]`
			}
			if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(manifest+`}`), 0644); err != nil {
				t.Fatal(err)
			}
			if mask&1 != 0 {
				if err := os.WriteFile(filepath.Join(dir, WelcomeFile), nil, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if mask&2 != 0 {
				if err := os.Mkdir(filepath.Join(dir, OpeningsDir), 0755); err != nil {
					t.Fatal(err)
				}
			}
			f, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			p, err := f.Build(nil)
			if err != nil {
				t.Fatal(err)
			}
			for i, key := range []string{"roleWelcome", "alternates", "prologue"} {
				got, exists := p.Welcome[key]
				if exists != (mask&(1<<i) != 0) {
					t.Errorf("%s presence = %v, payload %v", key, exists, p.Welcome)
				}
				if exists {
					raw, _ := json.Marshal(got)
					want := "[]"
					if i == 0 {
						want = `""`
					}
					if string(raw) != want {
						t.Errorf("%s = %s want %s", key, raw, want)
					}
				}
			}
			if err := f.Save(); err != nil {
				t.Fatal(err)
			}
			back, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			again, err := back.Build(nil)
			if err != nil {
				t.Fatal(err)
			}
			if Digest(p.Welcome) != Digest(again.Welcome) {
				t.Errorf("Save changed field presence: %v -> %v", p.Welcome, again.Welcome)
			}
		})
	}
}
