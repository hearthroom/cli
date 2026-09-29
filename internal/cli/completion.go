package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/output"
)

// Markers bound the block this command manages inside an rc file, so it can
// be re-run or removed without touching anything else in the file.
const (
	rcBegin = "# >>> hearthroom completion >>>"
	rcEnd   = "# <<< hearthroom completion <<<"
)

// addCompletionInstall attaches `completion install` and `completion uninstall`
// to cobra's generated completion command. Called from rootCommand after the
// default completion command exists.
func (a *App) addCompletionInstall(root *cobra.Command) {
	var target *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "completion" {
			target = c
			break
		}
	}
	if target == nil {
		return
	}
	target.Short = "Shell completion: install, uninstall, or print the script"
	var shell, rc string
	var noRC, print bool
	install := &cobra.Command{
		Use:   "install",
		Short: "Set up tab completion for your shell",
		Long: `Detects your shell (or takes --shell), writes the completion script where the
shell loads it from, and adds a small marked block to your shell's rc file so
completion is active in new terminals. Running it again is safe; it replaces
the block it wrote before.

  bash        ~/.local/share/bash-completion/completions/hearthroom, plus a block in ~/.bashrc
  zsh         a block in ~/.zshrc that loads the script from the binary
  fish        ~/.config/fish/completions/hearthroom.fish (no rc change needed)
  powershell  prints the line to add to your $PROFILE`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sh := shell
			if sh == "" {
				sh = detectShell()
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			res, err := installCompletion(root, sh, home, rc, noRC, print, a.Out)
			if err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(res)
			}
			for _, line := range res.Did {
				a.Out.Line("%s", line)
			}
			if res.Next != "" {
				a.Out.Line("%s", res.Next)
			}
			return nil
		},
	}
	install.Flags().StringVar(&shell, "shell", "", "bash, zsh, fish or powershell (default: detected from $SHELL)")
	install.Flags().StringVar(&rc, "rc", "", "rc file to edit instead of the default (~/.bashrc or ~/.zshrc)")
	install.Flags().BoolVar(&noRC, "no-rc", false, "write the completion file but do not edit any rc file")
	install.Flags().BoolVar(&print, "print", false, "show what would be written without changing anything")

	uninstall := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the completion files and rc block written by install",
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			removed := uninstallCompletion(home)
			if a.Out.JSON {
				return a.Out.JSONValue(map[string]any{"removed": removed})
			}
			if len(removed) == 0 {
				a.Out.Line("Nothing to remove.")
			}
			for _, r := range removed {
				a.Out.Line("Removed %s", r)
			}
			return nil
		},
	}
	target.AddCommand(install, uninstall)
}

type completionResult struct {
	Shell string   `json:"shell"`
	Did   []string `json:"did"`
	Next  string   `json:"next,omitempty"`
}

func detectShell() string {
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	base := filepath.Base(os.Getenv("SHELL"))
	switch base {
	case "bash", "zsh", "fish":
		return base
	}
	return "bash"
}

// zshBlock loads completion from the binary at shell start, enabling compinit
// only when the user's rc has not already done so.
const zshBlock = `if command -v hearthroom >/dev/null 2>&1; then
  (( $+functions[compdef] )) || { autoload -Uz compinit && compinit -u; }
  source <(hearthroom completion zsh)
fi`

func installCompletion(root *cobra.Command, sh, home, rcPath string, noRC, print bool, out output.Printer) (*completionResult, error) {
	res := &completionResult{Shell: sh}
	write := func(path string, data []byte) error {
		if print {
			res.Did = append(res.Did, "Would write "+path)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		res.Did = append(res.Did, "Wrote "+path)
		return nil
	}
	switch sh {
	case "bash":
		var buf bytes.Buffer
		if err := root.GenBashCompletionV2(&buf, true); err != nil {
			return nil, err
		}
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
		file := filepath.Join(dataHome, "bash-completion", "completions", "hearthroom")
		if err := write(file, buf.Bytes()); err != nil {
			return nil, err
		}
		if !noRC {
			if rcPath == "" {
				rcPath = filepath.Join(home, ".bashrc")
			}
			block := fmt.Sprintf("[ -f %q ] && source %q", file, file)
			if err := upsertRCBlock(rcPath, block, print, res); err != nil {
				return nil, err
			}
		}
		res.Next = "Open a new terminal, or run: source " + rcPath
	case "zsh":
		if !noRC {
			if rcPath == "" {
				rcPath = filepath.Join(home, ".zshrc")
			}
			if err := upsertRCBlock(rcPath, zshBlock, print, res); err != nil {
				return nil, err
			}
			res.Next = "Open a new terminal, or run: source " + rcPath
		} else {
			res.Next = "Add to your .zshrc:\n" + zshBlock
		}
	case "fish":
		var buf bytes.Buffer
		if err := root.GenFishCompletion(&buf, true); err != nil {
			return nil, err
		}
		cfg := os.Getenv("XDG_CONFIG_HOME")
		if cfg == "" {
			cfg = filepath.Join(home, ".config")
		}
		if err := write(filepath.Join(cfg, "fish", "completions", "hearthroom.fish"), buf.Bytes()); err != nil {
			return nil, err
		}
		res.Next = "Fish loads it automatically in new sessions."
	case "powershell":
		res.Next = "Add this line to your PowerShell profile (run `notepad $PROFILE`):\n  hearthroom completion powershell | Out-String | Invoke-Expression"
	default:
		return nil, output.Exitf(2, "unsupported shell %q; use --shell bash, zsh, fish or powershell", sh)
	}
	return res, nil
}

// upsertRCBlock replaces or appends the marked block in an rc file.
func upsertRCBlock(rcPath, block string, print bool, res *completionResult) error {
	existing, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(existing)
	full := rcBegin + "\n" + block + "\n" + rcEnd + "\n"
	if i := strings.Index(content, rcBegin); i >= 0 {
		if j := strings.Index(content[i:], rcEnd); j >= 0 {
			end := i + j + len(rcEnd)
			if end < len(content) && content[end] == '\n' {
				end++
			}
			if content[i:end] == full {
				res.Did = append(res.Did, "Already set up in "+rcPath)
				return nil
			}
			content = content[:i] + full + content[end:]
		} else {
			content = content[:i] + full
		}
	} else {
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if content != "" {
			content += "\n"
		}
		content += full
	}
	if print {
		res.Did = append(res.Did, "Would update "+rcPath+" with:\n"+full)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(rcPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(rcPath, []byte(content), 0o644); err != nil {
		return err
	}
	res.Did = append(res.Did, "Updated "+rcPath)
	return nil
}

func uninstallCompletion(home string) []string {
	var removed []string
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	for _, f := range []string{
		filepath.Join(dataHome, "bash-completion", "completions", "hearthroom"),
		filepath.Join(cfg, "fish", "completions", "hearthroom.fish"),
	} {
		if err := os.Remove(f); err == nil {
			removed = append(removed, f)
		}
	}
	for _, rc := range []string{filepath.Join(home, ".bashrc"), filepath.Join(home, ".zshrc")} {
		raw, err := os.ReadFile(rc)
		if err != nil {
			continue
		}
		content := string(raw)
		i := strings.Index(content, rcBegin)
		if i < 0 {
			continue
		}
		j := strings.Index(content[i:], rcEnd)
		if j < 0 {
			continue
		}
		end := i + j + len(rcEnd)
		if end < len(content) && content[end] == '\n' {
			end++
		}
		// Drop the blank line install added before the block.
		start := i
		if start > 0 && content[start-1] == '\n' && start > 1 && content[start-2] == '\n' {
			start--
		}
		if err := os.WriteFile(rc, []byte(content[:start]+content[end:]), 0o644); err == nil {
			removed = append(removed, "block in "+rc)
		}
	}
	return removed
}
