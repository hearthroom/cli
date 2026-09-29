// Package update tells users when a newer release exists. It checks at most
// once a day, in the background of a normal command, and only prints when
// someone is likely to read it (a terminal, not --json, not CI).
package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LatestURL redirects to the latest release tag. Following the redirect
// avoids the rate-limited GitHub API.
var LatestURL = "https://github.com/hearthroom/cli/releases/latest"

// Interval between checks.
const Interval = 24 * time.Hour

// EnvDisable turns the notifier off when set to any non-empty value.
const EnvDisable = "HEARTHROOM_NO_UPDATE_NOTIFIER"

const stateFile = "update-check.json"

type state struct {
	CheckedAt time.Time `json:"checkedAt"`
	Latest    string    `json:"latest"`
}

// Result of a check.
type Result struct {
	Current string
	Latest  string
}

// Newer reports whether Latest is a higher version than Current.
func (r Result) Newer() bool { return r.Latest != "" && compare(r.Latest, r.Current) > 0 }

// Enabled decides whether to run at all for this invocation.
func Enabled(currentVersion string, jsonOut bool, stderrIsTerminal bool) bool {
	if os.Getenv(EnvDisable) != "" || os.Getenv("CI") != "" || jsonOut || !stderrIsTerminal {
		return false
	}
	v := strings.TrimPrefix(currentVersion, "v")
	return v != "" && v != "dev" && !strings.Contains(v, "SNAPSHOT")
}

// Start begins a check when one is due and returns a channel that yields the
// result (or nothing) within timeout. The caller reads it at exit without
// blocking longer than that. dir is the config directory.
func Start(ctx context.Context, dir, currentVersion string, timeout time.Duration) <-chan Result {
	ch := make(chan Result, 1)
	st := load(dir)
	now := time.Now()
	if now.Sub(st.CheckedAt) < Interval {
		// Use the cached answer; no network.
		if st.Latest != "" {
			ch <- Result{Current: strings.TrimPrefix(currentVersion, "v"), Latest: st.Latest}
		}
		close(ch)
		return ch
	}
	go func() {
		defer close(ch)
		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		latest, err := fetchLatest(cctx)
		if err != nil {
			return
		}
		save(dir, state{CheckedAt: now, Latest: latest})
		ch <- Result{Current: strings.TrimPrefix(currentVersion, "v"), Latest: latest}
	}()
	return ch
}

// PackageManager reports which package manager installed the binary at
// installPath (after symlinks are resolved): "brew", "scoop" or "" for a
// script, manual or source install. Homebrew keeps casks under
// <prefix>/Caskroom and formulae under <prefix>/Cellar; the prefix is
// /opt/homebrew on Apple silicon but /usr/local on Intel Macs, so the
// prefix alone is not a signal. Scoop keeps apps under <root>/scoop/apps.
func PackageManager(installPath string) string {
	p := strings.ToLower(strings.ReplaceAll(installPath, "\\", "/"))
	switch {
	case strings.Contains(p, "/caskroom/") || strings.Contains(p, "/cellar/") || strings.Contains(p, "/homebrew/"):
		return "brew"
	case strings.Contains(p, "/scoop/"):
		return "scoop"
	}
	return ""
}

// UpgradeCommand is what the user should run to update an install of the
// given kind (see PackageManager).
func UpgradeCommand(pkg string) string {
	switch pkg {
	case "brew":
		return "brew upgrade hearthroom"
	case "scoop":
		return "scoop update hearthroom"
	}
	return "hearthroom upgrade"
}

// Notice is the line printed when a newer release exists. installPath lets
// package-manager installs point at their own upgrade command.
func Notice(r Result, installPath string) string {
	return "A new release of hearthroom is available: " + r.Current + " → " + r.Latest + ". Run `" + UpgradeCommand(PackageManager(installPath)) + "`."
}

func fetchLatest(ctx context.Context) (string, error) {
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, LatestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "hearthroom-cli update-check")
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	i := strings.LastIndex(loc, "/tag/")
	if resp.StatusCode/100 != 3 || i < 0 {
		return "", errors.New("no release redirect")
	}
	tag := strings.TrimPrefix(loc[i+len("/tag/"):], "v")
	if !versionRe.MatchString(tag) {
		return "", errors.New("unexpected tag " + tag)
	}
	return tag, nil
}

var versionRe = regexp.MustCompile(`^\d+\.\d+\.\d+([-.][0-9A-Za-z.-]+)?$`)

// compare orders two dotted versions numerically; pre-release suffixes sort
// below the plain version.
func compare(a, b string) int {
	pa, sa := split(a)
	pb, sb := split(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return 1
			}
			return -1
		}
	}
	switch {
	case sa == sb:
		return 0
	case sa == "":
		return 1
	case sb == "":
		return -1
	}
	return strings.Compare(sa, sb)
}

func split(v string) ([3]int, string) {
	var nums [3]int
	v = strings.TrimPrefix(v, "v")
	suffix := ""
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		suffix = v[i+1:]
		v = v[:i]
	}
	for i, p := range strings.SplitN(v, ".", 3) {
		nums[i], _ = strconv.Atoi(p)
	}
	return nums, suffix
}

func load(dir string) state {
	var st state
	raw, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err == nil {
		_ = json.Unmarshal(raw, &st)
	}
	return st
}

func save(dir string, st state) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	raw, _ := json.Marshal(st)
	_ = os.WriteFile(filepath.Join(dir, stateFile), raw, 0o644)
}
