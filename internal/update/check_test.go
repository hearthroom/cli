package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.2.0", "0.1.0", 1}, {"0.1.0", "0.1.0", 0}, {"0.1.0", "0.1.1", -1},
		{"1.0.0", "0.9.9", 1}, {"1.0.0", "1.0.0-rc1", 1}, {"1.0.0-rc1", "1.0.0", -1}, {"v0.3.0", "0.2.9", 1},
	}
	for _, c := range cases {
		if got := compare(c.a, c.b); got != c.want {
			t.Errorf("compare(%s,%s)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestEnabled(t *testing.T) {
	t.Setenv(EnvDisable, "")
	t.Setenv("CI", "")
	if !Enabled("0.1.0", false, true) {
		t.Fatal("should be enabled")
	}
	if Enabled("dev", false, true) || Enabled("0.1.0-SNAPSHOT-abc", false, true) || Enabled("0.1.0", true, true) || Enabled("0.1.0", false, false) {
		t.Fatal("dev/snapshot/json/non-tty must disable")
	}
	t.Setenv(EnvDisable, "1")
	if Enabled("0.1.0", false, true) {
		t.Fatal("env must disable")
	}
}

func TestStartFetchesOncePerDayAndCaches(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Location", "https://github.com/hearthroom/cli/releases/tag/v0.9.0")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()
	saved := LatestURL
	LatestURL = srv.URL + "/releases/latest"
	defer func() { LatestURL = saved }()

	dir := t.TempDir()
	r, ok := <-Start(context.Background(), dir, "0.1.0", 5*time.Second)
	if !ok || r.Latest != "0.9.0" || !r.Newer() {
		t.Fatalf("first check: %+v %v", r, ok)
	}
	raw, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		t.Fatal(err)
	}
	var st state
	_ = json.Unmarshal(raw, &st)
	if st.Latest != "0.9.0" || time.Since(st.CheckedAt) > time.Minute {
		t.Fatalf("state: %+v", st)
	}
	// Second call within the interval: cached, no network.
	r, ok = <-Start(context.Background(), dir, "0.1.0", 5*time.Second)
	if !ok || r.Latest != "0.9.0" || hits != 1 {
		t.Fatalf("cached check: %+v %v hits=%d", r, ok, hits)
	}
	// Up to date: not newer.
	r, _ = <-Start(context.Background(), dir, "0.9.0", 5*time.Second)
	if r.Newer() {
		t.Fatal("same version reported as newer")
	}
}

func TestStartSwallowsNetworkErrors(t *testing.T) {
	saved := LatestURL
	LatestURL = "http://127.0.0.1:1/releases/latest"
	defer func() { LatestURL = saved }()
	_, ok := <-Start(context.Background(), t.TempDir(), "0.1.0", 2*time.Second)
	if ok {
		t.Fatal("expected no result on network error")
	}
}

func TestNotice(t *testing.T) {
	r := Result{Current: "0.1.0", Latest: "0.2.0"}
	if got := Notice(r, "/usr/local/bin/hearthroom"); !strings.Contains(got, "hearthroom upgrade") {
		t.Fatal(got)
	}
	if got := Notice(r, "/opt/homebrew/Cellar/hearthroom/0.1.0/bin/hearthroom"); !strings.Contains(got, "brew upgrade") {
		t.Fatal(got)
	}
	if got := Notice(r, `C:\Users\me\scoop\apps\hearthroom\current\hearthroom.exe`); !strings.Contains(got, "scoop update") && filepath.Separator == '\\' {
		t.Fatal(got)
	}
}
