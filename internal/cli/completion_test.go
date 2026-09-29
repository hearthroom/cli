package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/output"
)

func TestUpsertRCBlockIsIdempotentAndPreservesContent(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	_ = os.WriteFile(rc, []byte("export FOO=1\n"), 0o644)
	res := &completionResult{}
	if err := upsertRCBlock(rc, "line one", false, res); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(rc)
	if !strings.HasPrefix(string(first), "export FOO=1\n\n"+rcBegin+"\nline one\n"+rcEnd+"\n") {
		t.Fatalf("first write:\n%s", first)
	}
	// Same block again: unchanged.
	if err := upsertRCBlock(rc, "line one", false, res); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(rc)
	if string(first) != string(second) || !strings.Contains(strings.Join(res.Did, "\n"), "Already set up") {
		t.Fatalf("second write changed the file or was not reported as no-op: %v", res.Did)
	}
	// Different block: replaced in place, user content kept, trailing content kept.
	_ = os.WriteFile(rc, append(second, []byte("alias ll='ls -l'\n")...), 0o644)
	if err := upsertRCBlock(rc, "line two", false, res); err != nil {
		t.Fatal(err)
	}
	third, _ := os.ReadFile(rc)
	s := string(third)
	if strings.Count(s, rcBegin) != 1 || !strings.Contains(s, "line two") || strings.Contains(s, "line one") || !strings.HasSuffix(s, "alias ll='ls -l'\n") {
		t.Fatalf("replace:\n%s", s)
	}
}

func TestInstallAndUninstallCompletion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	app := &App{Out: output.Printer{Out: os.Stdout, Err: os.Stderr}}
	root := app.rootCommand()
	for _, sh := range []string{"bash", "zsh", "fish"} {
		if _, err := installCompletion(root, sh, home, "", false, false, app.Out); err != nil {
			t.Fatalf("%s: %v", sh, err)
		}
	}
	for _, f := range []string{
		filepath.Join(home, ".local", "share", "bash-completion", "completions", "hearthroom"),
		filepath.Join(home, ".config", "fish", "completions", "hearthroom.fish"),
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
	} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("missing %s", f)
		}
	}
	zrc, _ := os.ReadFile(filepath.Join(home, ".zshrc"))
	if !strings.Contains(string(zrc), "source <(hearthroom completion zsh)") {
		t.Fatalf("zshrc:\n%s", zrc)
	}
	// --print changes nothing.
	before, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	res, err := installCompletion(root, "bash", home, "", false, true, app.Out)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if string(before) != string(after) || !strings.HasPrefix(res.Did[0], "Would") {
		t.Fatalf("print mode wrote: %v", res.Did)
	}
	if _, err := installCompletion(root, "tcsh", home, "", false, false, app.Out); err == nil {
		t.Fatal("unsupported shell accepted")
	}
	removed := uninstallCompletion(home)
	if len(removed) != 4 {
		t.Fatalf("removed %v", removed)
	}
	zrc, _ = os.ReadFile(filepath.Join(home, ".zshrc"))
	if strings.Contains(string(zrc), rcBegin) {
		t.Fatalf("block not removed:\n%s", zrc)
	}
}
