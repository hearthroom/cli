package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/output"
)

// generatePollEvery is how often a slow generation is polled; tests shorten it.
var generatePollEvery = 3 * time.Second

type genModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	// Lane and LaneName tell apart same-name models served by different channels.
	Lane     string `json:"lane"`
	LaneName string `json:"lane_name"`
	Pricing  struct {
		Unit    string           `json:"unit"`
		Credits map[string]int64 `json:"credits"`
	} `json:"pricing"`
	Capabilities struct {
		Sizes     []string `json:"sizes"`
		MaxImages int      `json:"max_images"`
		// MaxReferenceImages is how many --image references the model takes; 0 means none.
		MaxReferenceImages int `json:"max_reference_images"`
	} `json:"capabilities"`
	ContentPolicy struct {
		NSFW       string `json:"nsfw"`
		RealPerson bool   `json:"real_person"`
	} `json:"content_policy"`
}

type genTask struct {
	ID     string `json:"id"`
	Model  string `json:"model"`
	Status string `json:"status"`
	Credit int64  `json:"credits"`
	Data   []struct {
		URL string `json:"url"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	ExpiresAt int64 `json:"expires_at"`
}

func (a *App) mediaModelsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "List image models for media generate, with price and sizes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			var q url.Values
			if l := envLocale(); l != "" {
				q = url.Values{"language": {l}} // lane names in the terminal's language
			}
			var raw json.RawMessage
			if err := a.Client().OpenGet(cmd.Context(), "/generations/models", q, &raw); err != nil {
				return generationError(err)
			}
			if a.Out.JSON {
				return a.Out.JSONValue(raw)
			}
			var list struct {
				Data []genModel `json:"data"`
			}
			if err := json.Unmarshal(raw, &list); err != nil {
				return err
			}
			rows := [][]string{}
			for _, m := range list.Data {
				if m.Type != "image" {
					continue // only image models can be generated through these endpoints today
				}
				rows = append(rows, []string{m.ID, clip(m.Name, 24), m.LaneName, priceOf(m), strings.Join(m.Capabilities.Sizes, " "),
					fmt.Sprint(m.Capabilities.MaxImages), fmt.Sprint(m.Capabilities.MaxReferenceImages), m.ContentPolicy.NSFW})
			}
			a.Out.Table([]string{"MODEL", "NAME", "LANE", "CREDITS/IMAGE", "SIZES", "MAX", "REFS", "ADULT"}, rows)
			a.Out.Line("Images are paid from purchased credits only; refused prompts and failures are free.")
			return nil
		},
	}
}

// priceOf renders the per-image price; several tiers print as tier=credits.
func priceOf(m genModel) string {
	if len(m.Pricing.Credits) == 1 {
		for _, n := range m.Pricing.Credits {
			return fmt.Sprint(n)
		}
	}
	keys := make([]string, 0, len(m.Pricing.Credits))
	for k := range m.Pricing.Credits {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m.Pricing.Credits[k]))
	}
	return strings.Join(parts, " ")
}

func (a *App) mediaGenerateCommand() *cobra.Command {
	var model, prompt, promptFile, size, folder, dir, outFile, idemKey string
	var n int
	var refs []string
	var allowSpend bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate images with a model and optionally save them to your media library",
		Long: `Generates images from a prompt (see "media models" for models, prices and
sizes). It spends purchased credits per image, so it requires --allow-spend;
granted or gift credits cannot pay for it. Refused prompts and failed
generations are not charged.

Results are kept for 7 days. To keep one, save it to your media library with
--dir <path> (created if missing, e.g. the card's folder name) or --folder
<name or id> (an existing folder), and/or download it with --out <file>.
A saved image prints its library URL, which a card can use directly, e.g. as
media.portrait in card.json; a file downloaded into the card's assets/ is
referenced by its relative path instead.

To keep a character looking the same across images, pass its portrait with
--image (a library URL, or its path in the library as "media ls" shows it;
repeatable, up to the model's REFS in "media models").
Reference images cost nothing extra.

Each run sends an Idempotency-Key. If the command stops waiting, run it again
with the printed --idempotency-key to pick up the same generation without
being charged twice.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if model == "" {
				return output.Exitf(2, "--model is required (see `hearthroom media models`)")
			}
			if promptFile != "" {
				var b []byte
				var err error
				if promptFile == "-" {
					b, err = io.ReadAll(a.In)
				} else {
					b, err = os.ReadFile(promptFile)
				}
				if err != nil {
					return output.Exit(2, err)
				}
				prompt = string(b)
			}
			prompt = strings.TrimSpace(prompt)
			if prompt == "" {
				return output.Exitf(2, "--prompt or --prompt-file is required")
			}
			if n < 1 {
				return output.Exitf(2, "-n must be at least 1")
			}
			if folder != "" && dir != "" {
				return output.Exitf(2, "use --folder or --dir, not both")
			}
			for _, r := range refs {
				if strings.Contains(r, "://") && !strings.HasPrefix(r, "https://") {
					return output.Exitf(2, "--image %s: use an https URL or a media library path", r)
				}
			}
			if !allowSpend {
				return output.Exitf(3, "media generate spends purchased credits; re-run with --allow-spend to confirm")
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			if idemKey == "" {
				var buf [12]byte
				if _, err := rand.Read(buf[:]); err != nil {
					return err
				}
				idemKey = "hearthroom-cli-" + hex.EncodeToString(buf[:])
			}
			ctx := cmd.Context()
			c := a.Client()

			var saveBody map[string]any
			if dir != "" {
				saveBody = map[string]any{"directory": strings.Trim(dir, "/")}
			} else if folder != "" {
				id, err := folderID(ctx, c, folder)
				if err != nil {
					return err
				}
				saveBody = map[string]any{"folderIds": []string{id}}
			}

			body := map[string]any{"model": model, "prompt": prompt, "n": n}
			if size != "" {
				body["size"] = size
			}
			if len(refs) > 0 {
				urls := make([]string, len(refs))
				for i, r := range refs {
					u, err := referenceURL(ctx, c, r)
					if err != nil {
						return err
					}
					urls[i] = u
				}
				body["images"] = urls
			}
			a.Out.Note("Generating with %s (idempotency key %s)…", model, idemKey)
			// 200 carries the task under "harbor"; 202 is the task itself.
			var resp struct {
				Harbor *genTask `json:"harbor"`
				genTask
			}
			if err := c.OpenPostHeader(ctx, "/generations/images", http.Header{"Idempotency-Key": {idemKey}}, body, &resp); err != nil {
				return generationError(err)
			}
			task := resp.genTask
			if resp.Harbor != nil {
				task = *resp.Harbor
			}
			waitCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			for task.Status != "completed" && task.Status != "failed" {
				select {
				case <-waitCtx.Done():
					return output.Exitf(1, "generation %s is still running after %s; run the same command again with --idempotency-key %s to keep waiting (it is not charged twice)", task.ID, timeout, idemKey)
				case <-time.After(generatePollEvery):
				}
				if err := c.OpenGet(waitCtx, "/generations/"+url.PathEscape(task.ID), nil, &task); err != nil {
					if waitCtx.Err() != nil {
						continue
					}
					return generationError(err)
				}
			}
			if task.Status == "failed" {
				msg := "the generation failed"
				if task.Error != nil && task.Error.Message != "" {
					msg += ": " + task.Error.Message
				}
				return output.Exitf(1, "%s; you were not charged", msg)
			}

			images := make([]map[string]any, len(task.Data))
			for i, d := range task.Data {
				images[i] = map[string]any{"resultUrl": d.URL}
			}
			if outFile != "" {
				for i, d := range task.Data {
					target := outFile
					if len(task.Data) > 1 {
						ext := filepath.Ext(outFile)
						target = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(outFile, ext), i+1, ext)
					}
					if err := download(ctx, c, d.URL, target); err != nil {
						return err
					}
					images[i]["file"] = target
				}
			}
			if saveBody != nil {
				var saved struct {
					AssetIDs []string `json:"asset_ids"`
				}
				if err := c.OpenPost(ctx, "/generations/"+url.PathEscape(task.ID)+"/save", saveBody, &saved); err != nil {
					return generationError(err)
				}
				lib := libraryEntries(ctx, c, task)
				for i, id := range saved.AssetIDs {
					if i >= len(images) {
						break
					}
					images[i]["assetId"] = id
					if e, ok := lib[id]; ok {
						images[i]["url"], images[i]["fileName"] = e.URL, e.FileName
					}
				}
			}

			if a.Out.JSON {
				return a.Out.JSONValue(map[string]any{"taskId": task.ID, "model": task.Model, "credits": task.Credit,
					"idempotencyKey": idemKey, "expiresAt": task.ExpiresAt, "images": images})
			}
			a.Out.Line("Generated %d image(s) with %s for %d credits.", len(images), task.Model, task.Credit)
			for i, img := range images {
				switch {
				case img["url"] != nil:
					a.Out.Line("%d. saved %s → %s", i+1, img["fileName"], img["url"])
				case img["assetId"] != nil:
					a.Out.Line("%d. saved as asset %s", i+1, img["assetId"])
				default:
					a.Out.Line("%d. %s (kept for 7 days)", i+1, img["resultUrl"])
				}
				if f, ok := img["file"]; ok {
					a.Out.Line("   downloaded to %s", f)
				}
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&model, "model", "", "model id from \"hearthroom media models\"")
	f.StringVar(&prompt, "prompt", "", "what to draw (at most 4000 characters)")
	f.StringVar(&promptFile, "prompt-file", "", "read the prompt from a file, or - for stdin")
	f.StringVar(&size, "size", "", "one of the model's sizes, e.g. 1024x1536 (default: the model's)")
	f.IntVarP(&n, "count", "n", 1, "number of images (up to the model's MAX)")
	f.StringArrayVar(&refs, "image", nil, "reference image from your media library, as a URL or library path (repeatable); keeps a character consistent")
	f.StringVar(&dir, "dir", "", "save to this media library path, e.g. the card's folder name")
	f.StringVar(&folder, "folder", "", "save to an existing media library folder, by name or id")
	f.StringVar(&outFile, "out", "", "also download to this file; with -n above 1, -1, -2… are added before the extension")
	f.StringVar(&idemKey, "idempotency-key", "", "reuse a key to resume the same generation without paying twice (default: a new key)")
	f.BoolVar(&allowSpend, "allow-spend", false, "confirm that this generation may spend purchased credits")
	f.DurationVar(&timeout, "timeout", 10*time.Minute, "how long to wait for a slow generation")
	return cmd
}

// folderID resolves a library folder by id or name.
func folderID(ctx context.Context, c *api.Client, want string) (string, error) {
	var folders struct {
		Data struct {
			Folders []struct {
				ID   any    `json:"id"`
				Name string `json:"name"`
			} `json:"folders"`
		} `json:"data"`
	}
	if err := c.OpenGet(ctx, "/image/folder/list", nil, &folders); err != nil {
		return "", err
	}
	want = strings.Trim(want, "/")
	for _, f := range folders.Data.Folders {
		if id := fmt.Sprint(f.ID); id == want || f.Name == want {
			return id, nil
		}
	}
	return "", output.Exitf(2, "no media library folder named %q; use --dir %s to save under that path instead", want, want)
}

// referenceURL turns a --image value into the library URL the provider accepts.
// An https URL passes through; a library path must name an existing file.
func referenceURL(ctx context.Context, c *api.Client, ref string) (string, error) {
	if strings.HasPrefix(ref, "https://") {
		return ref, nil
	}
	want := strings.Trim(ref, "/")
	var list struct {
		Data struct {
			ImageList []struct {
				URL      string `json:"imageUrl"`
				FileName string `json:"fileName"`
			} `json:"imageList"`
		} `json:"data"`
	}
	if err := c.OpenGet(ctx, "/image/list", url.Values{"q": {want}, "kind": {"image"}, "pageSize": {"100"}}, &list); err != nil {
		return "", err
	}
	for _, it := range list.Data.ImageList {
		if it.FileName == want {
			return it.URL, nil
		}
	}
	return "", output.Exitf(2, "--image %s: no image at that path in your media library (see `hearthroom media ls`)", ref)
}

type libraryEntry struct{ URL, FileName string }

// libraryEntries finds the saved files in the library; the provider names them
// <model>-<first 8 of the task id>-<n>. A miss only loses the printed URL.
func libraryEntries(ctx context.Context, c *api.Client, t genTask) map[string]libraryEntry {
	q := url.Values{"q": {t.Model + "-" + clip8(t.ID)}, "kind": {"image"}, "pageSize": {"100"}}
	var list struct {
		Data struct {
			ImageList []struct {
				ID       any    `json:"id"`
				URL      string `json:"imageUrl"`
				FileName string `json:"fileName"`
			} `json:"imageList"`
		} `json:"data"`
	}
	out := map[string]libraryEntry{}
	if c.OpenGet(ctx, "/image/list", q, &list) == nil {
		for _, it := range list.Data.ImageList {
			out[fmt.Sprint(it.ID)] = libraryEntry{URL: it.URL, FileName: it.FileName}
		}
	}
	return out
}

func clip8(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func download(ctx context.Context, c *api.Client, src, target string) error {
	body, _, err := c.Download(ctx, src)
	if err != nil {
		return fmt.Errorf("download %s: %w", src, err)
	}
	defer body.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	f, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// hintError replaces the message of a provider error with what to do next,
// keeping the structured detail for --json.
type hintError struct {
	msg string
	err error
}

func (e hintError) Error() string { return e.msg }
func (e hintError) Unwrap() error { return e.err }

func generationError(err error) error {
	var e *api.Error
	if !errors.As(err, &e) {
		return err
	}
	msg := ""
	if d, ok := e.Detail.(map[string]any); ok {
		msg, _ = d["message"].(string)
	}
	switch {
	case e.Status == http.StatusUnauthorized:
		return output.Exit(4, hintError{"the sign-in is missing, expired or for another provider; run `hearthroom auth login` again", err})
	case e.Status == http.StatusForbidden && e.Code == "permission_error":
		return output.Exit(4, hintError{strings.TrimSpace(msg + " Run `hearthroom auth login` again and approve the requested access; sign-ins from older CLI versions did not ask for image generation."), err})
	case e.Status == http.StatusPaymentRequired:
		return hintError{"not enough purchased credits for this generation: images are paid from purchased credits only, granted or gift credits do not count. Top up, then run the command again", err}
	case e.Code == "content_policy_violation":
		return hintError{"the prompt is not allowed on this model, so nothing was generated and you were not charged; rephrase it or pick a model whose content policy allows it (`hearthroom media models --json`)", err}
	case msg != "":
		return hintError{fmt.Sprintf("%s (%d)", msg, e.Status), err}
	}
	return err
}
