package card

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeIntro(t *testing.T, f *Folder, text string) *Folder {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.Dir, IntroFile), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	back, err := Load(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return back
}

// intro.md is the public card-page introduction: it goes out as roleReadme,
// and the images it embeds are uploaded and rewritten like the definition's.
func TestIntroIsSentAsRoleReadmeWithAssetsRewritten(t *testing.T) {
	f := writeIntro(t, sample(t), "# Mira\n\n![](assets/mira.png)\n")
	if f.Intro == nil || *f.Intro != "# Mira\n\n![](assets/mira.png)\n" {
		t.Fatalf("Load: intro = %v", f.Intro)
	}
	p, err := f.Build(map[string]string{"assets/mira.png": "https://cdn/m.png"})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Card["roleReadme"]; got != "# Mira\n\n![](https://cdn/m.png)" {
		t.Fatalf("roleReadme = %q", got)
	}
}

// An image referenced only from intro.md must still be uploaded.
func TestIntroAssetsAreReferenced(t *testing.T) {
	f := sample(t)
	if err := os.WriteFile(filepath.Join(f.Dir, "assets", "page.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	f = writeIntro(t, f, "![](assets/page.png)")
	present, _ := f.AssetRefs()
	found := false
	for _, p := range present {
		found = found || p == "assets/page.png"
	}
	if !found {
		t.Fatalf("present = %v, want assets/page.png", present)
	}
}

// Without intro.md the field is not sent, so the card page keeps whatever it has.
func TestNoIntroFileSendsNothing(t *testing.T) {
	f, err := Load(sample(t).Dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.Intro != nil {
		t.Fatalf("intro = %q, want nil", *f.Intro)
	}
	p, err := f.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Card["roleReadme"]; ok {
		t.Fatalf("roleReadme sent without intro.md: %v", p.Card)
	}
}

// An existing but empty (or placeholder-only) intro.md sends an empty
// roleReadme, which clears the card page introduction.
func TestEmptyIntroFileClearsRoleReadme(t *testing.T) {
	for _, text := range []string{"", "\n  \n", "<!-- card page introduction -->\n"} {
		f := writeIntro(t, sample(t), text)
		p, err := f.Build(nil)
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := p.Card["roleReadme"]; !ok || got != "" {
			t.Fatalf("intro %q: roleReadme = %v (sent=%v), want empty string", text, got, ok)
		}
	}
}

// Editing intro.md changes the card section digest, so push sees a change.
func TestIntroChangesTheCardDigest(t *testing.T) {
	f := sample(t)
	base, _ := f.Build(nil)
	one, _ := writeIntro(t, f, "first").Build(nil)
	two, _ := writeIntro(t, f, "second").Build(nil)
	d0, d1, d2 := base.Digests()[SectionCard], one.Digests()[SectionCard], two.Digests()[SectionCard]
	if d0 == d1 || d1 == d2 {
		t.Fatalf("card digest did not change: %s %s %s", d0, d1, d2)
	}
}

// Pull writes intro.md from roleReadme only when the remote has one.
func TestFromRemoteIntro(t *testing.T) {
	dir := t.TempDir()
	f := FromRemote(dir, RemoteDetail{RoleName: "Mira", RoleReadme: "# About\n"}, "", "", nil, nil)
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, IntroFile))
	if err != nil || string(b) != "# About\n" {
		t.Fatalf("intro.md = %q, %v", b, err)
	}

	dir = t.TempDir()
	f = FromRemote(dir, RemoteDetail{RoleName: "Mira"}, "", "", nil, nil)
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, IntroFile)); !os.IsNotExist(err) {
		t.Fatalf("intro.md written for an empty roleReadme: %v", err)
	}
}

// intro.md must never be the working-notes README.md under another case:
// on case-insensitive file systems the two names would be one file.
func TestIntroFileDoesNotCollideWithFolderFiles(t *testing.T) {
	seen := map[string]string{}
	for _, n := range []string{ManifestFile, DefinitionFile, WelcomeFile, LorebookFile, RulesFile, ReadmeFile, AgentsFile, IntroFile} {
		k := strings.ToLower(n)
		if other, dup := seen[k]; dup {
			t.Fatalf("%s and %s are the same file on a case-insensitive file system", n, other)
		}
		seen[k] = n
	}
}
