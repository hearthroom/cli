package cli

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/media"
	"github.com/hearthroom/cli/internal/output"
)

func init() {
	extraCommands = append(extraCommands, func(a *App) []*cobra.Command { return []*cobra.Command{a.mediaCommand()} })
}

func (a *App) mediaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "media",
		Short: "Upload and list files in your media library",
	}

	var relPath, roleID string
	up := &cobra.Command{
		Use:   "upload <file>...",
		Short: "Upload files to your media library",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			caps, err := media.Probe(cmd.Context(), a.Client())
			if err != nil {
				return err
			}
			var results []map[string]any
			failed := false
			for _, file := range args {
				rp := ""
				if caps.RelativePaths {
					rp = relPath
					if rp == "" && len(args) > 1 {
						rp = filepath.Base(file)
					}
				}
				res, err := media.Upload(cmd.Context(), a.Client(), file, rp, roleID)
				entry := map[string]any{"file": file}
				if err != nil {
					entry["error"] = err.Error()
					failed = true
				} else {
					entry["url"] = res.ImageURL
					entry["fileName"] = res.FileName
					entry["kind"] = res.Kind
					entry["byteSize"] = res.ByteSize
					entry["replaced"] = res.Replaced
					if caps.RelativePaths && caps.LibraryPrefix != "" && res.FileName != "" {
						entry["servedUrl"] = caps.LibraryPrefix + "/" + res.FileName
					}
				}
				results = append(results, entry)
				if !a.Out.JSON {
					if err != nil {
						a.Out.Line("%s: FAILED: %v", file, err)
					} else {
						a.Out.Line("%s → %s", file, res.ImageURL)
					}
				}
			}
			if a.Out.JSON {
				if err := a.Out.JSONValue(map[string]any{"uploads": results}); err != nil {
					return err
				}
			}
			if failed {
				return output.Exitf(1, "some uploads failed")
			}
			return nil
		},
	}
	up.Flags().StringVar(&relPath, "path", "", "author-side relative path (single file), e.g. my-card/bg.png")
	up.Flags().StringVar(&roleID, "role", "", "attribute the upload to a card id")

	var q, kind, scope string
	var page, size int
	ls := &cobra.Command{
		Use:   "ls",
		Short: "List files in your media library",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			query := url.Values{"pageNum": {strconv.Itoa(page)}, "pageSize": {strconv.Itoa(size)}}
			if q != "" {
				query.Set("q", q)
			}
			if kind != "" {
				query.Set("kind", kind)
			}
			if scope != "" {
				query.Set("scope", scope)
			}
			var out struct {
				Code int `json:"code"`
				Data struct {
					Total         int64  `json:"total"`
					UsedBytes     int64  `json:"usedBytes"`
					ByteQuota     int64  `json:"byteQuota"`
					LibraryPrefix string `json:"libraryPrefix"`
					ImageList     []struct {
						ID       any    `json:"id"`
						URL      string `json:"imageUrl"`
						Kind     string `json:"kind"`
						FileName string `json:"fileName"`
						ByteSize int64  `json:"byteSize"`
						Created  string `json:"createTime"`
					} `json:"imageList"`
				} `json:"data"`
			}
			if err := a.Client().OpenGet(cmd.Context(), "/image/list", query, &out); err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(out.Data)
			}
			rows := make([][]string, 0, len(out.Data.ImageList))
			for _, it := range out.Data.ImageList {
				rows = append(rows, []string{fmt.Sprint(it.ID), it.Kind, it.FileName, strconv.FormatInt(it.ByteSize, 10), it.URL})
			}
			a.Out.Table([]string{"ID", "KIND", "FILE", "BYTES", "URL"}, rows)
			a.Out.Line("%d items, %d of %d bytes used", out.Data.Total, out.Data.UsedBytes, out.Data.ByteQuota)
			return nil
		},
	}
	ls.Flags().StringVar(&q, "q", "", "file name substring")
	ls.Flags().StringVar(&kind, "kind", "all", "image, video, audio, font, code (JS, WASM), data (JSON) or all")
	ls.Flags().StringVar(&scope, "scope", "", "role, folder or unfiled")
	ls.Flags().IntVar(&page, "page", 1, "page number")
	ls.Flags().IntVar(&size, "limit", 50, "entries per page")

	rm := &cobra.Command{
		Use:   "rm <id>...",
		Short: "Delete files from your media library by id (see `media ls`)",
		Long: `Deletes library items by the id shown in "media ls". An item that one of your
cards still uses as portrait or background cannot be deleted; change the card's
image first.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			var results []map[string]any
			failed := false
			for _, id := range args {
				var out struct {
					Data struct {
						ImageID any `json:"imageId"`
					} `json:"data"`
				}
				err := a.Client().OpenPost(cmd.Context(), "/image/delete", map[string]string{"imageId": id}, &out)
				entry := map[string]any{"id": id, "deleted": err == nil}
				if err != nil {
					failed = true
					entry["error"] = err.Error()
					if cards := cardsUsing(err); len(cards) > 0 {
						entry["usedBy"] = cards
						if !a.Out.JSON {
							a.Out.Line("%s: still used by %d card(s); change their image first:", id, len(cards))
							for _, c := range cards {
								a.Out.Line("    %s  %s", c["roleId"], c["name"])
							}
						}
					} else if !a.Out.JSON {
						a.Out.Line("%s: FAILED: %v", id, err)
					}
				} else if !a.Out.JSON {
					a.Out.Line("%s deleted", id)
				}
				results = append(results, entry)
			}
			if a.Out.JSON {
				if err := a.Out.JSONValue(map[string]any{"results": results}); err != nil {
					return err
				}
			}
			if failed {
				return output.Exitf(1, "some items could not be deleted")
			}
			return nil
		},
	}
	cmd.AddCommand(up, ls, rm, a.mediaMoveCommand())
	return cmd
}

// cardsUsing extracts detail.cards from an image_in_use error.
func cardsUsing(err error) []map[string]string {
	var e *api.Error
	if !errors.As(err, &e) || e.Code != "image_in_use" {
		return nil
	}
	detail, _ := e.Detail.(map[string]any)
	raw, _ := detail["cards"].([]any)
	var out []map[string]string
	for _, r := range raw {
		m, _ := r.(map[string]any)
		out = append(out, map[string]string{"roleId": fmt.Sprint(m["roleId"]), "name": fmt.Sprint(m["name"])})
	}
	return out
}
