package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/hearthroom/cli/internal/media"
)

func TestUploadConcurrencyFromFlagOrEnvironment(t *testing.T) {
	t.Setenv(envUploadConcurrency, "")
	if n, err := uploadConcurrency(media.DefaultConcurrency, false); err != nil || n != media.DefaultConcurrency {
		t.Fatalf("default: %d %v", n, err)
	}
	t.Setenv(envUploadConcurrency, "3")
	if n, err := uploadConcurrency(media.DefaultConcurrency, false); err != nil || n != 3 {
		t.Fatalf("environment: %d %v", n, err)
	}
	if n, err := uploadConcurrency(10, true); err != nil || n != 10 {
		t.Fatalf("flag over environment: %d %v", n, err)
	}
	for _, bad := range []int{0, media.MaxConcurrency + 1} {
		if _, err := uploadConcurrency(bad, true); err == nil {
			t.Fatalf("flag %d accepted", bad)
		}
	}
	t.Setenv(envUploadConcurrency, "many")
	if _, err := uploadConcurrency(media.DefaultConcurrency, false); err == nil {
		t.Fatal("non-numeric environment accepted")
	}
}

func TestUploadProgressLinesWithoutATerminal(t *testing.T) {
	var out strings.Builder
	clock := time.Unix(0, 0)
	u := newUploadProgress(&out, false)
	u.now = func() time.Time { return clock }
	u.update(media.Progress{Total: 100})
	clock = clock.Add(5 * time.Second)
	u.update(media.Progress{Done: 10, Total: 100}) // within the interval: no line
	clock = clock.Add(15 * time.Second)
	u.update(media.Progress{Done: 40, Failed: 1, Total: 100})
	clock = clock.Add(10 * time.Second)
	u.update(media.Progress{Done: 100, Failed: 1, Total: 100}) // the last one always prints
	u.done()
	want := "uploading 0/100 files\n" +
		"uploading 40/100 files (1 failed) · 2.0/s · about 30s left\n" +
		"uploading 100/100 files (1 failed) · 3.3/s · 30s\n"
	if out.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestUploadProgressRewritesOneTerminalLine(t *testing.T) {
	var out strings.Builder
	clock := time.Unix(0, 0)
	u := newUploadProgress(&out, true)
	u.now = func() time.Time { return clock }
	u.update(media.Progress{Total: 2})
	clock = clock.Add(time.Second)
	u.update(media.Progress{Done: 1, Total: 2})
	clock = clock.Add(time.Second)
	u.update(media.Progress{Done: 2, Total: 2})
	u.done() // already ended by the last update
	got := out.String()
	if strings.Count(got, "\r\033[K") != 3 || strings.Count(got, "\n") != 1 || !strings.HasSuffix(got, "uploading 2/2 files · 1.0/s · 2s\n") {
		t.Fatalf("terminal output %q", got)
	}
}
