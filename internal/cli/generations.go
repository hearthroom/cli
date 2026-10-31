package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/output"
)

// genListed is one generation as "media generations" and the web studio see it:
// data[i].asset_id is set once that image is in the media library.
type genListed struct {
	ID        string `json:"id"`
	Model     string `json:"model"`
	Status    string `json:"status"`
	Size      string `json:"price_tier"`
	Credits   int64  `json:"credits"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
	Data      []struct {
		URL     string `json:"url"`
		AssetID string `json:"asset_id"`
	} `json:"data"`
}

func (a *App) mediaGenerationsCommand() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "generations",
		Short: "List your recent image generations and which images are saved",
		Long: `Lists the image generations of the last 7 days, newest first, from this CLI,
hearthroom.club and any other signed-in client. Each image shows its number
(for "media save --pick"), its result URL and whether it is already in your
media library. Results that are not saved are deleted after 7 days.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if limit < 1 || limit > 100 {
				return output.Exitf(2, "--limit must be between 1 and 100")
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			var raw json.RawMessage
			q := url.Values{"type": {"image"}, "limit": {strconv.Itoa(limit)}}
			if err := a.Client().OpenGet(cmd.Context(), "/generations", q, &raw); err != nil {
				return generationError(err)
			}
			if a.Out.JSON {
				return a.Out.JSONValue(raw)
			}
			var list struct {
				Data []genListed `json:"data"`
			}
			if err := json.Unmarshal(raw, &list); err != nil {
				return err
			}
			if len(list.Data) == 0 {
				a.Out.Line("No image generations in the last 7 days.")
				return nil
			}
			now := time.Now()
			for i, g := range list.Data {
				if i > 0 {
					a.Out.Line("")
				}
				saved := 0
				for _, d := range g.Data {
					if d.AssetID != "" {
						saved++
					}
				}
				meta := []string{time.Unix(g.CreatedAt, 0).Local().Format("2006-01-02 15:04"), g.Model, g.Size, g.Status}
				if g.Status == "completed" {
					meta = append(meta, fmt.Sprintf("%d credits", g.Credits), fmt.Sprintf("%d/%d saved", saved, len(g.Data)))
					if g.ExpiresAt > 0 && len(g.Data) > saved {
						meta = append(meta, "unsaved images deleted "+until(now, time.Unix(g.ExpiresAt, 0)))
					}
				}
				a.Out.Line("%s  %s", g.ID, strings.Join(meta, " · "))
				for n, d := range g.Data {
					mark := ""
					if d.AssetID != "" {
						mark = "  [saved]"
					}
					a.Out.Line("  %d. %s%s", n+1, d.URL, mark)
				}
			}
			a.Out.Line("")
			a.Out.Line("Save one with: hearthroom media save <id> --pick <n> --name <name> --dir <path>")
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "how many generations to list (1-100)")
	return cmd
}

func until(now, t time.Time) string {
	d := t.Sub(now)
	switch {
	case d <= 0:
		return "now"
	case d < 24*time.Hour:
		return fmt.Sprintf("in %dh", int(d.Hours())+1)
	}
	return fmt.Sprintf("in %dd", int(d.Hours()/24)+1)
}

func (a *App) mediaSaveCommand() *cobra.Command {
	var picks []int
	var names []string
	var folder, dir string
	cmd := &cobra.Command{
		Use:   "save <generation-id>",
		Short: "Save picked images of a generation to your media library, under names you choose",
		Long: `Saves images of a generation (see "media generations") into your media
library with --dir <path> (created if missing, e.g. the card's folder name) or
--folder <name or id> (an existing folder).

--pick takes the image numbers shown by "media generations", starting at 1;
repeat it or separate numbers with commas. Without --pick every image is saved.
--name gives the file name for each picked image, in the same order; the
extension is added for you. A name already used in that folder is refused,
never overwritten. Saving an image again returns the file it was saved as.
Saving is free and uses your media library storage.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if (folder == "") == (dir == "") {
				return output.Exitf(2, "choose where to save with --dir <path> or --folder <name or id>")
			}
			if len(names) > 0 && len(names) != len(picks) {
				return output.Exitf(2, "give one --name per --pick (%d picks, %d names)", len(picks), len(names))
			}
			for _, p := range picks {
				if p < 1 {
					return output.Exitf(2, "--pick numbers start at 1")
				}
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			ctx := cmd.Context()
			c := a.Client()
			body, err := saveTarget(ctx, c, dir, folder)
			if err != nil {
				return err
			}
			id := args[0]
			if len(picks) == 0 {
				var task genListed
				if err := c.OpenGet(ctx, "/generations/"+url.PathEscape(id), nil, &task); err != nil {
					return generationError(err)
				}
				for i := range task.Data {
					picks = append(picks, i+1)
				}
				if len(picks) == 0 {
					return output.Exitf(1, "generation %s has no images to save (still running, failed, or older than 7 days)", id)
				}
			}
			items := make([]map[string]any, len(picks))
			for i, p := range picks {
				items[i] = map[string]any{"index": p - 1}
				if len(names) > 0 {
					items[i]["name"] = strings.TrimSpace(names[i])
				}
			}
			body["items"] = items
			saved, err := saveItems(ctx, c, id, body)
			if err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(map[string]any{"taskId": id, "images": saved})
			}
			for _, s := range saved {
				a.Out.Line("%d. saved → %s", s.Index+1, s.URL)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.IntSliceVar(&picks, "pick", nil, "image number from \"media generations\", starting at 1 (repeatable or comma-separated; default: all)")
	f.StringArrayVar(&names, "name", nil, "file name for each picked image, in --pick order (default: model and generation id)")
	f.StringVar(&dir, "dir", "", "save to this media library path, e.g. the card's folder name")
	f.StringVar(&folder, "folder", "", "save to an existing media library folder, by name or id")
	return cmd
}

// saveTarget is the save body for --dir or --folder; both empty means none.
func saveTarget(ctx context.Context, c *api.Client, dir, folder string) (map[string]any, error) {
	switch {
	case dir != "":
		return map[string]any{"directory": strings.Trim(dir, "/")}, nil
	case folder != "":
		id, err := folderID(ctx, c, folder)
		if err != nil {
			return nil, err
		}
		return map[string]any{"folderIds": []string{id}}, nil
	}
	return nil, nil
}

type savedImage struct {
	Index    int    `json:"index"`
	AssetID  string `json:"assetId"`
	URL      string `json:"url"`
	FileName string `json:"fileName,omitempty"`
}

// saveItems posts a save with items and returns the saved files with their library URLs.
func saveItems(ctx context.Context, c *api.Client, id string, body map[string]any) ([]savedImage, error) {
	var resp struct {
		Assets []struct {
			Index   int    `json:"index"`
			AssetID string `json:"asset_id"`
			URL     string `json:"url"`
		} `json:"assets"`
	}
	if err := c.OpenPost(ctx, "/generations/"+url.PathEscape(id)+"/save", body, &resp); err != nil {
		var e *api.Error
		if errors.As(err, &e) && e.Status == http.StatusConflict && e.Code == "name_taken" {
			return nil, output.Exit(2, hintError{"that folder already has a file with this name, and it was not replaced; choose another --name", err})
		}
		return nil, generationError(err)
	}
	out := make([]savedImage, len(resp.Assets))
	for i, a := range resp.Assets {
		out[i] = savedImage{Index: a.Index, AssetID: a.AssetID, URL: a.URL}
		if u, err := url.Parse(a.URL); err == nil {
			out[i].FileName = path.Base(u.Path)
		}
	}
	return out, nil
}
