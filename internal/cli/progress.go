package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hearthroom/cli/internal/media"
)

// envUploadConcurrency sets the default of `card push --concurrency`.
const envUploadConcurrency = "HEARTHROOM_UPLOAD_CONCURRENCY"

// uploadConcurrency resolves --concurrency: the flag when given, else the
// environment, else the media default; always within 1..media.MaxConcurrency.
func uploadConcurrency(flag int, flagSet bool) (int, error) {
	n := flag
	if !flagSet {
		v := strings.TrimSpace(os.Getenv(envUploadConcurrency))
		if v == "" {
			return media.DefaultConcurrency, nil
		}
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("%s=%q is not a number", envUploadConcurrency, v)
		}
		n = parsed
	}
	if n < 1 || n > media.MaxConcurrency {
		return 0, fmt.Errorf("upload concurrency must be between 1 and %d, got %d", media.MaxConcurrency, n)
	}
	return n, nil
}

// uploadProgress renders media upload progress on stderr: one line rewritten
// in place on a terminal, a line every interval otherwise.
type uploadProgress struct {
	w        io.Writer
	terminal bool
	now      func() time.Time
	interval time.Duration // between lines when not a terminal

	start, last time.Time
	drawn       bool
}

func newUploadProgress(w io.Writer, terminal bool) *uploadProgress {
	interval := 15 * time.Second
	if terminal {
		interval = 200 * time.Millisecond
	}
	return &uploadProgress{w: w, terminal: terminal, now: time.Now, interval: interval}
}

func (u *uploadProgress) update(p media.Progress) {
	now := u.now()
	if u.start.IsZero() {
		u.start = now
	}
	finished := p.Done == p.Total
	if p.Done > 0 && !finished && now.Sub(u.last) < u.interval {
		return
	}
	u.last = now
	line := fmt.Sprintf("uploading %d/%d files", p.Done, p.Total)
	if p.Failed > 0 {
		line += fmt.Sprintf(" (%d failed)", p.Failed)
	}
	if elapsed := now.Sub(u.start); p.Done > 0 && elapsed > 0 {
		rate := float64(p.Done) / elapsed.Seconds()
		line += fmt.Sprintf(" · %.1f/s", rate)
		if left := p.Total - p.Done; left > 0 {
			eta := time.Duration(float64(left) / rate * float64(time.Second))
			line += " · about " + eta.Round(time.Second).String() + " left"
		} else {
			line += " · " + elapsed.Round(time.Second).String()
		}
	}
	if u.terminal {
		fmt.Fprintf(u.w, "\r\033[K%s", line)
		u.drawn = true
		if finished {
			u.done()
		}
		return
	}
	fmt.Fprintln(u.w, line)
}

// done ends the in-place line so later output starts on a fresh one.
func (u *uploadProgress) done() {
	if u.terminal && u.drawn {
		fmt.Fprintln(u.w)
		u.drawn = false
	}
}
