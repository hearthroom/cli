package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/output"
)

const releasesLatest = "https://github.com/hearthroom/cli/releases/latest"

func init() {
	extraCommands = append(extraCommands, func(a *App) []*cobra.Command { return []*cobra.Command{a.upgradeCommand()} })
}

func (a *App) upgradeCommand() *cobra.Command {
	var check bool
	c := &cobra.Command{
		Use:   "upgrade",
		Short: "Update hearthroom to the latest release",
		Long: `Downloads the latest release for this platform from GitHub Releases, verifies
it against the release's checksums.txt, and replaces the running binary.

If hearthroom was installed with a package manager (Homebrew, Scoop), use that
manager's upgrade command instead; this command will tell you when it notices.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			if real, err := filepath.EvalSymlinks(exe); err == nil {
				exe = real
			}
			if strings.Contains(exe, "/Cellar/") || strings.Contains(exe, "/homebrew/") {
				return output.Exitf(2, "installed with Homebrew; run `brew upgrade hearthroom`")
			}
			if strings.Contains(strings.ToLower(exe), string(filepath.Separator)+"scoop"+string(filepath.Separator)) {
				return output.Exitf(2, "installed with Scoop; run `scoop update hearthroom`")
			}
			hc := &http.Client{Timeout: 5 * time.Minute}
			tag, err := latestTag(cmd.Context(), a.userAgent())
			if err != nil {
				return fmt.Errorf("check releases: %w", err)
			}
			latest := strings.TrimPrefix(tag, "v")
			current := strings.TrimPrefix(a.Info.Version, "v")
			if a.Out.JSON && check {
				return a.Out.JSONValue(map[string]any{"current": current, "latest": latest, "upToDate": latest == current})
			}
			if latest == current {
				a.Out.Line("hearthroom %s is the latest release.", current)
				return nil
			}
			if check {
				a.Out.Line("hearthroom %s is available (you have %s). Run `hearthroom upgrade`.", latest, current)
				return nil
			}
			if current == "dev" {
				return output.Exitf(2, "this is a source build; install a release build first (see README)")
			}
			ext := "tar.gz"
			if runtime.GOOS == "windows" {
				ext = "zip"
			}
			want := fmt.Sprintf("hearthroom_%s_%s_%s.%s", latest, runtime.GOOS, runtime.GOARCH, ext)
			base := "https://github.com/hearthroom/cli/releases/download/" + tag + "/"
			assetURL, sumsURL := base+want, base+"checksums.txt"
			a.Out.Note("Downloading %s…", want)
			archive, err := fetch(hc, assetURL, a.userAgent())
			if err != nil {
				return err
			}
			sums, err := fetch(hc, sumsURL, a.userAgent())
			if err != nil {
				return err
			}
			if err := verifyChecksum(archive, want, string(sums)); err != nil {
				return err
			}
			bin, err := extractBinary(archive, ext)
			if err != nil {
				return err
			}
			if err := replaceExecutable(exe, bin); err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(map[string]any{"from": current, "to": latest, "path": exe})
			}
			a.Out.Line("Updated hearthroom %s → %s (%s)", current, latest, exe)
			return nil
		},
	}
	c.Flags().BoolVar(&check, "check", false, "only report whether a newer release exists")
	return c
}

// latestTag follows the releases/latest redirect; no GitHub API, no rate limit.
func latestTag(ctx context.Context, ua string) (string, error) {
	hc := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, releasesLatest, nil)
	req.Header.Set("User-Agent", ua)
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	i := strings.LastIndex(loc, "/tag/")
	if resp.StatusCode/100 != 3 || i < 0 {
		return "", fmt.Errorf("no release found (GitHub returned %d)", resp.StatusCode)
	}
	return loc[i+len("/tag/"):], nil
}

func fetch(hc *http.Client, url, ua string) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("User-Agent", ua)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("the release has no asset at %s (no build for this platform?)", url)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download %s: %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}

func verifyChecksum(data []byte, name, sums string) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			if strings.EqualFold(fields[0], got) {
				return nil
			}
			return fmt.Errorf("checksum mismatch for %s", name)
		}
	}
	return fmt.Errorf("no checksum for %s in checksums.txt", name)
}

func extractBinary(archive []byte, ext string) ([]byte, error) {
	name := "hearthroom"
	if ext == "zip" {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if f.Name == name+".exe" || f.Name == name {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(io.LimitReader(rc, 200<<20))
			}
		}
		return nil, fmt.Errorf("binary not found in archive")
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(h.Name) == name && h.Typeflag == tar.TypeReg {
			return io.ReadAll(io.LimitReader(tr, 200<<20))
		}
	}
	return nil, fmt.Errorf("binary not found in archive")
}

// replaceExecutable writes the new binary next to the old one and renames it
// into place. On Windows the running file cannot be replaced directly, so the
// old binary is moved aside first and removed on the next run.
func replaceExecutable(exe string, bin []byte) error {
	dir := filepath.Dir(exe)
	tmp := filepath.Join(dir, ".hearthroom.new")
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return fmt.Errorf("write new binary: %w (is %s writable?)", err, dir)
	}
	if runtime.GOOS == "windows" {
		old := exe + ".old"
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			os.Remove(tmp)
			return fmt.Errorf("move old binary aside: %w", err)
		}
		if err := os.Rename(tmp, exe); err != nil {
			_ = os.Rename(old, exe)
			return err
		}
		return nil
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replace binary: %w", err)
	}
	return nil
}
