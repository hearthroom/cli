package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

// slowLibrary is a media library whose uploads take time and can fail on
// demand. reply decides each upload: the nth request overall (from 1) for the
// relative path; it returns the HTTP status and may block or sleep.
type slowLibrary struct {
	reply func(r *http.Request, n int64, relativePath string) int

	requests atomic.Int64
	inFlight atomic.Int64
	peak     atomic.Int64

	mu     sync.Mutex
	hits   map[string]int
	stored []string // relative paths that were answered 200, in order
}

func (l *slowLibrary) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/open/v1/image/list":
		l.mu.Lock()
		items := []map[string]any{}
		for _, rp := range l.stored {
			items = append(items, map[string]any{"id": rp, "fileName": rp, "imageUrl": prefix + "/" + escapePath(rp)})
		}
		l.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"total": len(items), "imageList": items, "libraryPrefix": prefix,
			"capabilities": map[string]any{"relativePaths": true, "overwrite": true},
		}})
	case "/open/v1/image/upload":
		cur := l.inFlight.Add(1)
		defer l.inFlight.Add(-1)
		for {
			p := l.peak.Load()
			if cur <= p || l.peak.CompareAndSwap(p, cur) {
				break
			}
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rp := r.FormValue("relativePath")
		l.mu.Lock()
		l.hits[rp]++
		l.mu.Unlock()
		status := http.StatusOK
		if l.reply != nil {
			status = l.reply(r, l.requests.Add(1), rp)
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": http.StatusText(status)})
			return
		}
		l.mu.Lock()
		l.stored = append(l.stored, rp)
		l.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"imageId": rp, "imageUrl": prefix + "/" + escapePath(rp), "fileName": rp,
		}})
	default:
		http.NotFound(w, r)
	}
}

// manyFiles builds a card whose display rule references assets/art/, holding
// n files, against lib.
func manyFiles(t *testing.T, lib *slowLibrary, n int) (*api.Client, *card.Folder) {
	t.Helper()
	lib.hits = map[string]int{}
	srv := httptest.NewServer(lib)
	t.Cleanup(srv.Close)
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets", "art"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range n {
		name := filepath.Join(dir, "assets", "art", fmt.Sprintf("layer-%03d.webp", i))
		if err := os.WriteFile(name, []byte(fmt.Sprintf("layer %d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f := &card.Folder{Dir: dir}
	f.Manifest.Name = "art"
	f.Rules = &card.Rules{Rules: []card.DisplayRule{{Find: "/<l>(\\S+)<\\/l>/", Replace: `<img src="assets/art/$1.webp">`, Enabled: true}}}
	return c, f
}

func fastRetries(t *testing.T) {
	t.Helper()
	old := retryBase
	retryBase = time.Millisecond
	t.Cleanup(func() { retryBase = old })
}

func savedState(t *testing.T, f *card.Folder) card.State {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.Dir, card.StateDir, card.StateFile))
	if err != nil {
		t.Fatal(err)
	}
	var st card.State
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func noTempFiles(t *testing.T, f *card.Folder) {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(f.Dir, card.StateDir))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temporary state file left behind: %s", e.Name())
		}
	}
}

func TestParallelSyncKeepsOutcomesInReferenceOrder(t *testing.T) {
	lib := &slowLibrary{reply: func(_ *http.Request, n int64, _ string) int {
		time.Sleep(time.Duration(10+(n*37)%60) * time.Millisecond) // finish out of order
		return http.StatusOK
	}}
	c, f := manyFiles(t, lib, 40)
	caps, _ := Probe(context.Background(), c)
	old := saveEvery
	saveEvery = 0
	t.Cleanup(func() { saveEvery = old })
	var saves atomic.Int64
	var last Progress
	urls, outcomes, err := Sync(context.Background(), c, f, caps, SyncOptions{
		Concurrency: 8,
		Save:        func() error { saves.Add(1); return f.SaveState() },
		Progress:    func(p Progress) { last = p },
	})
	if err != nil {
		t.Fatal(err)
	}
	present, _ := f.AssetRefs()
	if len(outcomes) != len(present) {
		t.Fatalf("%d outcomes for %d files", len(outcomes), len(present))
	}
	for i, o := range outcomes {
		if o.Path != present[i] || !o.Uploaded || o.Err != "" || o.URL == "" {
			t.Fatalf("outcome %d: %+v (want path %s)", i, o, present[i])
		}
		if urls[o.Path] != o.URL || f.State.Assets[o.Path].URL != o.URL {
			t.Fatalf("%s: url %q, state %q, outcome %q", o.Path, urls[o.Path], f.State.Assets[o.Path].URL, o.URL)
		}
	}
	if p := lib.peak.Load(); p < 2 || p > 8 {
		t.Fatalf("peak parallel uploads %d, want 2..8", p)
	}
	if last != (Progress{Done: 40, Total: 40}) {
		t.Fatalf("last progress %+v", last)
	}
	if saves.Load() < 40 {
		t.Fatalf("state saved %d times while uploading 40 files with saveEvery=0", saves.Load())
	}
	if st := savedState(t, f); len(st.Assets) != 40 || st.AssetFolder != "art" {
		t.Fatalf("saved state: %d assets, folder %q", len(st.Assets), st.AssetFolder)
	}
	noTempFiles(t, f)
}

func TestCancelledSyncSavesOnlyFinishedUploads(t *testing.T) {
	const finished = 5
	var resumed atomic.Bool
	lib := &slowLibrary{}
	lib.reply = func(r *http.Request, n int64, _ string) int {
		if n <= finished || resumed.Load() {
			return http.StatusOK
		}
		<-r.Context().Done() // never answers; the client gives up
		return http.StatusServiceUnavailable
	}
	c, f := manyFiles(t, lib, 30)
	caps, _ := Probe(context.Background(), c)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _, err := Sync(ctx, c, f, caps, SyncOptions{
		Concurrency: 4,
		Save:        f.SaveState,
		Progress: func(p Progress) {
			if p.Done == finished {
				cancel()
			}
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	st := savedState(t, f)
	if len(st.Assets) != finished {
		t.Fatalf("saved %d assets, want the %d that finished", len(st.Assets), finished)
	}
	for rel, a := range st.Assets {
		if a.URL == "" || a.SHA256 == "" {
			t.Fatalf("%s saved without url or digest: %+v", rel, a)
		}
	}
	noTempFiles(t, f)

	// The next push sends only what is left.
	resumed.Store(true)
	f2 := &card.Folder{Dir: f.Dir, Manifest: f.Manifest, Rules: f.Rules, State: st}
	_, outcomes, err := Sync(context.Background(), c, f2, caps, SyncOptions{Concurrency: 4, Save: f2.SaveState})
	if err != nil {
		t.Fatal(err)
	}
	uploaded := 0
	for _, o := range outcomes {
		if o.Uploaded {
			uploaded++
		}
	}
	if uploaded != 30-finished {
		t.Fatalf("resumed push uploaded %d files, want %d", uploaded, 30-finished)
	}
}

func TestUploadRetriesGatewayErrorsAndRateLimits(t *testing.T) {
	fastRetries(t)
	var calls atomic.Int64
	lib := &slowLibrary{reply: func(_ *http.Request, _ int64, _ string) int {
		switch calls.Add(1) {
		case 1:
			return http.StatusServiceUnavailable
		case 2:
			return http.StatusTooManyRequests
		}
		return http.StatusOK
	}}
	c, f := manyFiles(t, lib, 1)
	caps, _ := Probe(context.Background(), c)
	_, outcomes, err := Sync(context.Background(), c, f, caps, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || !outcomes[0].Uploaded || outcomes[0].Err != "" {
		t.Fatalf("outcomes %+v", outcomes)
	}
	if calls.Load() != 3 {
		t.Fatalf("%d requests, want 3", calls.Load())
	}
}

func TestUploadDoesNotRetryARefusal(t *testing.T) {
	fastRetries(t)
	lib := &slowLibrary{reply: func(_ *http.Request, _ int64, _ string) int { return http.StatusBadRequest }}
	c, f := manyFiles(t, lib, 1)
	caps, _ := Probe(context.Background(), c)
	_, outcomes, err := Sync(context.Background(), c, f, caps, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || outcomes[0].Err == "" {
		t.Fatalf("outcomes %+v", outcomes)
	}
	if n := lib.requests.Load(); n != 1 {
		t.Fatalf("%d requests for a 400, want 1", n)
	}
}

func TestUploadGivesUpAfterLimitedAttempts(t *testing.T) {
	fastRetries(t)
	lib := &slowLibrary{reply: func(_ *http.Request, _ int64, _ string) int { return http.StatusBadGateway }}
	c, f := manyFiles(t, lib, 1)
	caps, _ := Probe(context.Background(), c)
	_, outcomes, err := Sync(context.Background(), c, f, caps, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || outcomes[0].Err == "" || outcomes[0].Uploaded {
		t.Fatalf("outcomes %+v", outcomes)
	}
	if n := lib.requests.Load(); n != int64(uploadAttempts) {
		t.Fatalf("%d requests, want %d", n, uploadAttempts)
	}
}

func TestRetryDelayFollowsRetryAfter(t *testing.T) {
	if d := retryDelay(1, &api.Error{Status: 429, RetryAfter: 7}); d != 7*time.Second {
		t.Fatalf("retry_after 7: %v", d)
	}
	if d := retryDelay(1, &api.Error{Status: 429, RetryAfter: 600}); d != time.Minute {
		t.Fatalf("retry_after capped: %v", d)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		base := retryBase << (attempt - 1)
		if d := retryDelay(attempt, &api.Error{Status: 503}); d < base/2 || d > base*3/2 {
			t.Fatalf("attempt %d: %v outside %v±50%%", attempt, d, base)
		}
	}
}

func TestRetryAfterHeaderIsRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", nil)
	err := c.OpenGet(context.Background(), "/x", nil, nil)
	var e *api.Error
	if !errors.As(err, &e) || e.RetryAfter != 3 {
		t.Fatalf("err %v, want RetryAfter 3", err)
	}
}

// TestParallelUploadTiming measures one batch at 200–300 ms per upload with
// one worker and with the default. Set HEARTHROOM_UPLOAD_BENCH=1 to run it.
func TestParallelUploadTiming(t *testing.T) {
	if os.Getenv("HEARTHROOM_UPLOAD_BENCH") == "" {
		t.Skip("set HEARTHROOM_UPLOAD_BENCH=1 to measure")
	}
	const files = 60
	run := func(concurrency int) time.Duration {
		lib := &slowLibrary{reply: func(_ *http.Request, n int64, _ string) int {
			time.Sleep(time.Duration(200+(n*53)%100) * time.Millisecond)
			return http.StatusOK
		}}
		c, f := manyFiles(t, lib, files)
		caps, _ := Probe(context.Background(), c)
		start := time.Now()
		_, outcomes, err := Sync(context.Background(), c, f, caps, SyncOptions{Concurrency: concurrency, Save: f.SaveState})
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range outcomes {
			if !o.Uploaded {
				t.Fatalf("%+v", o)
			}
		}
		return time.Since(start)
	}
	seq := run(1)
	for _, n := range []int{1, 4, DefaultConcurrency, 8, MaxConcurrency} {
		d := seq
		if n != 1 {
			d = run(n)
		}
		t.Logf("%d files, concurrency %2d: %6.2fs  %5.1f files/s  %.1fx", files, n, d.Seconds(), files/d.Seconds(), seq.Seconds()/d.Seconds())
	}
}

func TestParallelUploadIsFaster(t *testing.T) {
	const files, delay = 16, 50 * time.Millisecond
	lib := &slowLibrary{reply: func(_ *http.Request, _ int64, _ string) int {
		time.Sleep(delay)
		return http.StatusOK
	}}
	c, f := manyFiles(t, lib, files)
	caps, _ := Probe(context.Background(), c)
	start := time.Now()
	if _, _, err := Sync(context.Background(), c, f, caps, SyncOptions{Concurrency: 8}); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > files*delay*3/4 {
		t.Fatalf("16 uploads of 50ms with 8 workers took %v; sequential would be %v", d, files*delay)
	}
}

func TestSyncStopsWhenTheProviderIsDown(t *testing.T) {
	fastRetries(t)
	lib := &slowLibrary{reply: func(_ *http.Request, _ int64, _ string) int { return http.StatusServiceUnavailable }}
	c, f := manyFiles(t, lib, 200)
	caps, _ := Probe(context.Background(), c)
	_, _, err := Sync(context.Background(), c, f, caps, SyncOptions{Concurrency: 4, Save: f.SaveState})
	if err == nil || !strings.Contains(err.Error(), "not answering") {
		t.Fatalf("err = %v", err)
	}
	// giveUpAfter files, plus what the other workers had in flight.
	if n := lib.requests.Load(); n > int64((giveUpAfter+4)*uploadAttempts) {
		t.Fatalf("%d upload requests before giving up", n)
	}
	noTempFiles(t, f)
}
