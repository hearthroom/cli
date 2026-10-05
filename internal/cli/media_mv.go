package cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/media"
	"github.com/hearthroom/cli/internal/output"
)

// moveReport is the provider's answer to a rename or move (dry run or real).
type moveReport struct {
	DryRun  bool   `json:"dryRun"`
	Moved   int    `json:"moved"`
	Pending int    `json:"pending"`
	Folders int    `json:"folders"`
	Name    string `json:"name,omitempty"`
	URL     string `json:"imageUrl,omitempty"`
	UsedBy  []struct {
		RoleID    string `json:"roleId"`
		Name      string `json:"name"`
		Published bool   `json:"published"`
	} `json:"usedBy"`
}

func (a *App) mediaMoveCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "mv <from> <to>",
		Short: "Rename or move a folder or file in your media library",
		Long: `Renames a folder (and everything below it) or a file. A file's URL is its
path, so moving it changes the URL and the old URL stops working.

Without --yes this only shows what would move and which of your cards still
mention the old URLs; fix those cards (or accept that their images break) and
run again with --yes. A published card version can no longer be edited.

<from> is a folder ("my-card", "my-card/art") or a file path
("my-card/art/bg.webp") as "media ls" shows it; <to> is the new full path.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			ctx := cmd.Context()
			c := a.Client()
			caps, err := media.Probe(ctx, c)
			if err != nil {
				return err
			}
			if !caps.Moves {
				return output.Exitf(1, "this provider cannot move library files")
			}
			from, to := strings.Trim(args[0], "/"), strings.Trim(args[1], "/")
			path, body, err := moveTarget(ctx, c, from)
			if err != nil {
				return err
			}
			if path == "move" {
				body["path"] = to
			} else {
				body["name"] = to
			}
			send := func(extra map[string]any) (moveReport, error) {
				req := map[string]any{}
				for k, v := range body {
					req[k] = v
				}
				for k, v := range extra {
					req[k] = v
				}
				var out struct {
					Data moveReport `json:"data"`
				}
				err := c.OpenPost(ctx, "/image/"+path, req, &out)
				return out.Data, err
			}
			plan, err := send(map[string]any{"dryRun": true})
			if err != nil {
				return err
			}
			if !yes {
				if a.Out.JSON {
					return a.Out.JSONValue(map[string]any{"plan": plan, "moved": false})
				}
				a.Out.Line("%d file(s) would move from %s to %s; their old URLs stop working.", plan.Moved, from, to)
				printUsers(a, plan)
				a.Out.Line("Run again with --yes to move.")
				return nil
			}
			done, err := send(map[string]any{"confirm": true})
			if err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(map[string]any{"plan": plan, "result": done, "moved": true})
			}
			a.Out.Line("Moved %d file(s) from %s to %s.", done.Moved, from, to)
			if done.Pending > 0 {
				a.Out.Line("%d file(s) are still being copied; their new URLs work within a few minutes.", done.Pending)
			}
			printUsers(a, plan)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "move now, even if cards still mention the old URLs")
	return cmd
}

func printUsers(a *App, r moveReport) {
	if len(r.UsedBy) == 0 {
		return
	}
	a.Out.Line("These cards still mention the old URLs; their images break until you update them:")
	for _, u := range r.UsedBy {
		note := ""
		if u.Published {
			note = " (published version, cannot be edited)"
		}
		a.Out.Line("    %s  %s%s", u.RoleID, u.Name, note)
	}
}

// moveTarget resolves <from> to the endpoint and body: a folder by name, a
// directory that only exists as part of longer paths, or a file by path.
func moveTarget(ctx context.Context, c *api.Client, from string) (string, map[string]any, error) {
	var folders struct {
		Data struct {
			Folders []struct {
				ID   any    `json:"id"`
				Name string `json:"name"`
			} `json:"folders"`
		} `json:"data"`
	}
	if err := c.OpenGet(ctx, "/image/folder/list", nil, &folders); err != nil {
		return "", nil, err
	}
	for _, f := range folders.Data.Folders {
		if f.Name == from {
			return "folder/rename", map[string]any{"folderId": fmt.Sprint(f.ID)}, nil
		}
	}
	var list struct {
		Data struct {
			ImageList []struct {
				ID       any    `json:"id"`
				FileName string `json:"fileName"`
			} `json:"imageList"`
		} `json:"data"`
	}
	if err := c.OpenGet(ctx, "/image/list", url.Values{"q": {from}, "kind": {"all"}, "pageSize": {"100"}}, &list); err != nil {
		return "", nil, err
	}
	dir := false
	for _, it := range list.Data.ImageList {
		if it.FileName == from {
			return "move", map[string]any{"imageId": fmt.Sprint(it.ID)}, nil
		}
		if strings.HasPrefix(it.FileName, from+"/") {
			dir = true
		}
	}
	for _, f := range folders.Data.Folders {
		if strings.HasPrefix(f.Name, from+"/") {
			dir = true
		}
	}
	if dir {
		return "folder/rename", map[string]any{"path": from}, nil
	}
	return "", nil, output.Exitf(1, "%s is neither a folder nor a file in your media library (see `media ls`)", from)
}
