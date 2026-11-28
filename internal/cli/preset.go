package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/output"
	"github.com/hearthroom/cli/internal/preset"
	"github.com/spf13/cobra"
)

func init() {
	extraCommands = append(extraCommands, func(a *App) []*cobra.Command {
		group := &cobra.Command{Use: "preset", Short: "Check and manage your prompt presets"}
		group.AddCommand(a.presetCheckCommand(), a.presetPushCommand(), a.presetPullCommand(), a.presetListCommand(),
			a.presetBindingCommand("bind", "bind <preset-id> --role <role-id>", "Use a saved prompt preset for a card", cobra.ExactArgs(1)),
			a.presetBindingCommand("unbind", "unbind --role <role-id>", "Use the platform default for a card", cobra.NoArgs),
			a.presetBindingCommand("status", "status --role <role-id>", "Show the prompt preset a card will use", cobra.NoArgs))
		return []*cobra.Command{group}
	})
}

var presetIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type presetAsset struct {
	ID                string   `json:"id"`
	HeadRevisionID    string   `json:"headRevisionId,omitempty"`
	Title             string   `json:"title"`
	Version           int64    `json:"version"`
	SourceKind        string   `json:"sourceKind"`
	Visibility        string   `json:"visibility"`
	ContentVisibility string   `json:"contentVisibility,omitempty"`
	RecommendedModels []string `json:"recommendedModels,omitempty"`
}

func presetProfile(cmd *cobra.Command, profile int64) *int64 {
	if cmd.Flags().Changed("profile-id") {
		return &profile
	}
	return nil
}

func (a *App) presetCheckCommand() *cobra.Command {
	var profile int64
	c := &cobra.Command{
		Use:   "check <file.json>",
		Short: "Check a prompt preset file offline",
		Long:  "Checks the file structure, suki_ fields, and platform slots. Harbor checks compatibility again when you save.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, report, err := preset.Read(args[0], presetProfile(cmd, profile))
			if a.Out.JSON {
				result := struct {
					preset.Report
					Valid bool   `json:"valid"`
					Error string `json:"error,omitempty"`
				}{Report: report, Valid: err == nil}
				if err != nil {
					result.Error = err.Error()
				}
				if printErr := a.Out.JSONValue(result); printErr != nil {
					return printErr
				}
				if err != nil {
					return output.ExitQuiet(2, "%s", err)
				}
				return nil
			}
			if err != nil {
				return output.Exit(2, err)
			}
			a.Out.Line("File checks passed. Profile %d. Tool reply %t.", report.ProfileID, report.ToolReply)
			for _, warning := range report.Warnings {
				a.Out.Note("%s", warning)
			}
			return nil
		},
	}
	c.Flags().Int64Var(&profile, "profile-id", 0, "SillyTavern prompt order profile ID")
	return c
}

func (a *App) requirePresetAuth() error {
	if err := a.RequireAuth(); err != nil {
		return output.Exitf(4, "sign in with `hearthroom auth login --presets`")
	}
	return nil
}

func (a *App) presetListCommand() *cobra.Command {
	type page struct {
		Items      []presetAsset `json:"items"`
		NextCursor string        `json:"nextCursor"`
		Quota      struct {
			Used  int `json:"used"`
			Limit int `json:"limit"`
		} `json:"quota"`
	}
	return &cobra.Command{
		Use:   "list",
		Short: "List your prompt presets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.requirePresetAuth(); err != nil {
				return err
			}
			result := page{Items: []presetAsset{}}
			query := url.Values{"limit": {"50"}}
			seen := map[string]bool{}
			for {
				var current page
				if err := a.Client().OpenGet(cmd.Context(), "/presets", query, &current); err != nil {
					return reportPresetError(err)
				}
				result.Items = append(result.Items, current.Items...)
				result.Quota = current.Quota
				cursor := current.NextCursor
				if cursor == "" {
					break
				}
				if seen[cursor] {
					return errors.New("the provider repeated a preset list cursor")
				}
				seen[cursor] = true
				query.Set("cursor", cursor)
			}
			if a.Out.JSON {
				return a.Out.JSONValue(result)
			}
			var rows [][]string
			for _, item := range result.Items {
				rows = append(rows, []string{item.ID, item.Title, strconv.FormatInt(item.Version, 10)})
			}
			if len(rows) == 0 {
				a.Out.Line("You have no prompt presets.")
			} else {
				a.Out.Table([]string{"ID", "TITLE", "VERSION"}, rows)
			}
			a.Out.Line("%d of %d preset slots used.", result.Quota.Used, result.Quota.Limit)
			return nil
		},
	}
}

func (a *App) presetPullCommand() *cobra.Command {
	var target, format string
	c := &cobra.Command{
		Use:   "pull <id> --out <file.json>",
		Short: "Export one of your prompt presets",
		Long:  "Exports the current content. Use --format hearthroom to keep the saved selection for editing and pushing it again.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if target == "" || !presetIDPattern.MatchString(args[0]) {
				return output.Exitf(2, "provide a preset ID from `hearthroom preset list` and an output file")
			}
			if format != "st" && format != "hearthroom" {
				return output.Exitf(2, "--format must be st or hearthroom")
			}
			if _, err := os.Lstat(target); err == nil {
				return output.Exitf(2, "output file already exists")
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := a.requirePresetAuth(); err != nil {
				return err
			}
			raw, version, err := a.presetExport(cmd.Context(), args[0], format)
			if err != nil {
				return err
			}
			if err := writeNewPrivateFile(target, raw); err != nil {
				return err
			}
			if a.Out.JSON {
				result := struct {
					ID      string `json:"id"`
					Format  string `json:"format"`
					Path    string `json:"path"`
					Version int64  `json:"version,omitempty"`
				}{args[0], format, target, version}
				return a.Out.JSONValue(result)
			}
			if version != 0 {
				a.Out.Line("Exported %s at version %d to %s.", args[0], version, target)
			} else {
				a.Out.Line("Exported %s to %s.", args[0], target)
			}
			return nil
		},
	}
	c.Flags().StringVar(&target, "out", "", "create a new private output file")
	c.Flags().StringVar(&format, "format", "st", "st for SillyTavern, or hearthroom for editing")
	_ = c.MarkFlagRequired("out")
	return c
}

func (a *App) presetExport(ctx context.Context, id, format string) (json.RawMessage, int64, error) {
	path := "/presets/" + id
	var raw json.RawMessage
	var version int64
	if format == "st" {
		if err := a.Client().OpenGet(ctx, path+"/export", nil, &raw); err != nil {
			return nil, 0, reportPresetError(err)
		}
	} else {
		var err error
		raw, version, err = a.presetEditSnapshot(ctx, path)
		if err != nil {
			return nil, 0, err
		}
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil || document == nil {
		return nil, 0, errors.New("the provider did not return a preset JSON object")
	}
	return raw, version, nil
}

func (a *App) presetEditSnapshot(ctx context.Context, path string) (json.RawMessage, int64, error) {
	// Publishing can advance the asset's edit version without changing its
	// content row. Match the asset's current head to the content read so a
	// concurrent save cannot pair old bytes with a newer edit version.
	for range 3 {
		var asset presetAsset
		if err := a.Client().OpenGet(ctx, path, nil, &asset); err != nil {
			return nil, 0, reportPresetError(err)
		}
		var result struct {
			Revision struct {
				ID       string          `json:"revisionId"`
				Document json.RawMessage `json:"document"`
			} `json:"revision"`
		}
		if err := a.Client().OpenGet(ctx, path+"/content", nil, &result); err != nil {
			return nil, 0, reportPresetError(err)
		}
		if asset.Version < 1 || !presetIDPattern.MatchString(asset.HeadRevisionID) || !presetIDPattern.MatchString(result.Revision.ID) {
			return nil, 0, errors.New("the provider did not return a complete preset version")
		}
		if asset.HeadRevisionID == result.Revision.ID {
			return result.Revision.Document, asset.Version, nil
		}
	}
	return nil, 0, output.Exitf(2, "the preset changed during export. Run `hearthroom preset pull` again")
}

// Publish a fully written private file at a new path without replacing an
// existing file or following a destination symlink.
func writeNewPrivateFile(path string, raw []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".hearthroom-preset-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(raw)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Link(file.Name(), path)
}

type presetRemoteError struct {
	Status int
	Code   string
	Hint   string
}

func (e presetRemoteError) Error() string {
	if e.Code != "" {
		return e.Hint + " (" + e.Code + ")"
	}
	return e.Hint
}

func (e presetRemoteError) ErrorDetail() any {
	return map[string]any{"status": e.Status, "code": e.Code}
}

func reportPresetError(err error) error {
	var remote *api.Error
	if !errors.As(err, &remote) {
		return errors.New("could not read the provider's preset response")
	}
	message := presetRemoteError{Status: remote.Status, Hint: "the provider could not complete the preset request"}
	switch remote.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		message.Hint = "preset access was refused. Sign in with `hearthroom auth login --presets` on a supported provider"
		return output.Exit(4, message)
	case http.StatusNotFound:
		message.Hint = "preset not found in your account"
	case http.StatusConflict:
		message.Hint = "the preset or the pending import changed. Read the current version before trying another save"
		message.Code = "preset_conflict"
	case http.StatusGone:
		message.Hint = "the import preview expired. Start the import again"
	case http.StatusRequestEntityTooLarge:
		message.Hint = "the preset exceeds the provider's size limit"
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		message.Hint = "the provider rejected the preset. Check the file and its compatibility in the web editor"
	case http.StatusTooManyRequests:
		message.Hint = "the preset request reached a provider limit. Check your preset quota before retrying"
	}
	if remote.Status >= 400 && remote.Status < 500 {
		return output.Exit(2, message)
	}
	return message
}
