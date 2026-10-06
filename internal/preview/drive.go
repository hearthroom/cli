package preview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// FindChrome returns an executable for headless Chrome: HEARTHROOM_CHROME, then the
// usual names on PATH, then the standard install locations.
func FindChrome() (string, error) {
	if p := strings.TrimSpace(os.Getenv("HEARTHROOM_CHROME")); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("HEARTHROOM_CHROME=%q: %w", p, err)
		}
		return p, nil
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome", "msedge", "microsoft-edge"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(home, "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"))
		}
	case "windows":
		for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LocalAppData")} {
			if base == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(base, `Google\Chrome\Application\chrome.exe`),
				filepath.Join(base, `Microsoft\Edge\Application\msedge.exe`),
				filepath.Join(base, `Chromium\Application\chrome.exe`))
		}
	default:
		candidates = []string{"/usr/bin/google-chrome", "/usr/bin/chromium", "/usr/bin/chromium-browser", "/snap/bin/chromium", "/usr/bin/microsoft-edge"}
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("no Chrome, Chromium or Edge found")
}

// DriveOptions is what Run needs beyond the server.
type DriveOptions struct {
	Chrome   string        // executable (FindChrome)
	Origin   string        // the preview server's origin
	States   []State       // from Plan
	ShotsDir string        // where PNGs go
	Timeout  time.Duration // per state; 0 = 45s
	Progress func(string)  // optional
}

func allocator(ctx context.Context, chrome string) (context.Context, context.CancelFunc) {
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(chrome), chromedp.Flag("hide-scrollbars", true), chromedp.Flag("force-device-scale-factor", "1"))
	return chromedp.NewExecAllocator(ctx, opts...)
}

// Run drives every state in its own tab and returns the results; the caller derives the
// findings and screenshots the contact sheet with Contact.
func Run(ctx context.Context, opts DriveOptions) ([]StateResult, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 45 * time.Second
	}
	if err := os.MkdirAll(opts.ShotsDir, 0o755); err != nil {
		return nil, err
	}
	allocCtx, cancelAlloc := allocator(ctx, opts.Chrome)
	defer cancelAlloc()
	// one browser for the run; the first context owns it
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()
	if _, err := chromedp.Run(browserCtx, chromedp.Evaluate[int](`1`)); err != nil {
		return nil, fmt.Errorf("start %s: %w", opts.Chrome, err)
	}
	var results []StateResult
	for _, st := range opts.States {
		if opts.Progress != nil {
			opts.Progress(st.Name)
		}
		results = append(results, driveState(browserCtx, opts, st))
	}
	return results, nil
}

func driveState(browserCtx context.Context, opts DriveOptions, st State) StateResult {
	res := StateResult{State: st, HydrationMs: -1, ConsoleErrors: []string{}}
	tabCtx, cancelTab := chromedp.NewContext(browserCtx)
	defer cancelTab()
	ctx, cancel := context.WithTimeout(tabCtx, opts.Timeout)
	defer cancel()
	fail := func(step string, err error) StateResult {
		res.Error = step + ": " + err.Error()
		return res
	}
	// Runtime events carry the card scripts' exceptions and console.error calls; the
	// same-origin iframe runs in this target.
	if err := chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		_, err := cdp.Call(ctx, t, cdpruntime.Enable, cdp.Empty{})
		return err
	})); err != nil {
		return fail("runtime", err)
	}
	var mu sync.Mutex
	exceptions := chromedp.Events(ctx, cdpruntime.ExceptionThrown)
	consoles := chromedp.Events(ctx, cdpruntime.ConsoleAPICalled)
	go func() {
		for ev, err := range exceptions {
			if err != nil {
				return
			}
			mu.Lock()
			res.ConsoleErrors = append(res.ConsoleErrors, exceptionText(ev))
			mu.Unlock()
		}
	}()
	go func() {
		for ev, err := range consoles {
			if err != nil {
				return
			}
			if string(ev.Type) != "error" {
				continue
			}
			var parts []string
			for _, a := range ev.Args {
				if a == nil {
					continue
				}
				if len(a.Value) > 0 {
					parts = append(parts, strings.Trim(string(a.Value), `"`))
				} else if a.Description != "" {
					parts = append(parts, a.Description)
				}
			}
			mu.Lock()
			res.ConsoleErrors = append(res.ConsoleErrors, strings.Join(parts, " "))
			mu.Unlock()
		}
	}()

	url := opts.Origin + "/bench/card-preview/?card=/card/&bare=1"
	if st.RulesOff {
		url += "&rules=off"
	}
	ready := `typeof __preview === "object" && __preview.sent.some(m => m.type === "ready")`
	var readyErr error
	for attempt := 0; attempt < 2; attempt++ {
		if err := chromedp.Do(ctx,
			chromedp.EmulateViewport(int64(st.Size.W), int64(st.Size.H)),
			chromedp.Navigate(url),
		); err != nil {
			return fail("navigate", err)
		}
		if readyErr = waitFor(ctx, ready, 30*time.Second); readyErr == nil {
			break
		}
	}
	if readyErr != nil {
		return fail("shell ready", readyErr)
	}
	if st.Theme != "dark" {
		if err := chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`(() => { const s = document.getElementById('theme'); s.value = `+jsString(st.Theme)+`; s.dispatchEvent(new Event('change')); })()`)); err != nil {
			return fail("theme", err)
		}
		time.Sleep(300 * time.Millisecond)
	}
	// stream the sample, then time the panel from the moment the reply is done
	if _, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(async () => { await __preview.stream(`+jsString(st.Text)+`, 8); return true })()`, chromedp.EvalAwaitPromise)); err != nil {
		return fail("stream", err)
	}
	t0 := time.Now()
	deadline := t0.Add(3 * time.Second)
	for {
		panels, err := chromedp.Run(ctx, chromedp.Evaluate[int](`(() => { const d = document.getElementById('frame').contentDocument; return d.querySelectorAll('.hr-status--done').length + d.querySelectorAll('.lt-status').length })()`))
		if err != nil {
			return fail("hydration", err)
		}
		if panels > 0 {
			res.HydrationMs = int(time.Since(t0) / time.Millisecond)
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(400 * time.Millisecond)
	snap, err := chromedp.Run(ctx, chromedp.Evaluate[Snapshot](`__preview.snapshot()`))
	if err != nil {
		return fail("snapshot", err)
	}
	res.Snapshot = snap
	facts, err := chromedp.Run(ctx, chromedp.Evaluate[Facts](factsJS))
	if err != nil {
		return fail("facts", err)
	}
	res.Facts = facts
	png, err := chromedp.Run(ctx, chromedp.CaptureScreenshot())
	if err != nil {
		return fail("screenshot", err)
	}
	name := st.Name + ".png"
	if err := os.WriteFile(filepath.Join(opts.ShotsDir, name), png, 0o644); err != nil {
		return fail("write", err)
	}
	res.Shot = name
	mu.Lock()
	defer mu.Unlock()
	return res
}

// Contact screenshots the contact sheet the server generates at /contact.html.
func Contact(ctx context.Context, chrome, origin, outPath string) error {
	allocCtx, cancelAlloc := allocator(ctx, chrome)
	defer cancelAlloc()
	tabCtx, cancelTab := chromedp.NewContext(allocCtx)
	defer cancelTab()
	if _, err := chromedp.Run(tabCtx, chromedp.Evaluate[int](`1`)); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(tabCtx, 60*time.Second)
	defer cancel()
	if err := chromedp.Do(ctx,
		chromedp.EmulateViewport(1400, 900),
		chromedp.Navigate(origin+"/contact.html"),
		chromedp.WaitReady("body"),
		chromedp.Sleep(800*time.Millisecond),
	); err != nil {
		return err
	}
	png, err := chromedp.Run(ctx, chromedp.FullScreenshot(90))
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, png, 0o644)
}

const factsJS = `(() => {
  const f = document.getElementById('frame'); const d = f.contentDocument; const w = f.contentWindow;
  const q = (s) => d.querySelectorAll(s).length;
  let small = 0;
  d.querySelectorAll('[data-chat="message-body"] *').forEach((el) => {
    const own = Array.from(el.childNodes).some((n) => n.nodeType === 3 && n.textContent.trim());
    if (!own) return;
    const fs = parseFloat(w.getComputedStyle(el).fontSize);
    if (fs && fs < 12) small++;
  });
  return {
    overflow: d.documentElement.scrollWidth > w.innerWidth + 1,
    choiceButtons: q('.hr-choice:not(.hr-choice--own)') + q('.lt-choice'),
    nativePanels: q('.lt-status'),
    smallText: small,
  };
})()`

func waitFor(ctx context.Context, expr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](expr))
		if err == nil && ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for %s", timeout, expr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func exceptionText(e cdpruntime.EventExceptionThrown) string {
	if e.ExceptionDetails == nil {
		return "exception"
	}
	d := e.ExceptionDetails
	if d.Exception != nil && d.Exception.Description != "" {
		return d.Exception.Description
	}
	return d.Text
}
