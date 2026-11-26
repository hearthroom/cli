package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/output"
	"github.com/hearthroom/cli/internal/preset"
	"github.com/spf13/cobra"
)

var presetOperationID = regexp.MustCompile(`^[A-Za-z0-9_-]{8,80}$`)

type presetWriteInput struct {
	Title             string          `json:"title"`
	Document          json.RawMessage `json:"document"`
	RecommendedModels []string        `json:"recommendedModels"`
	ExpectedVersion   int64           `json:"expectedVersion,omitempty"`
}

type presetPreview struct {
	ID              string `json:"previewId"`
	Version         int64  `json:"version"`
	SelectionDigest string `json:"selectionDigest"`
}

type pendingPresetWrite struct {
	Fingerprint       string         `json:"fingerprint"`
	Title             string         `json:"title"`
	RecommendedModels []string       `json:"recommendedModels"`
	Preview           *presetPreview `json:"preview,omitempty"`
}

type presetWriteReceipt struct {
	ID         string `json:"presetId"`
	RevisionID string `json:"revisionId"`
	Version    int64  `json:"version"`
}

func (a *App) presetPushCommand() *cobra.Command {
	var create bool
	var id, title, key string
	var profile, version int64
	c := &cobra.Command{
		Use:   "push <file.json> (--create --title <name> | --id <id> --expected-version <n>)",
		Short: "Create or update a prompt preset",
		Long: `Create a private preset from a SillyTavern file or a Hearthroom document.
SillyTavern files use the web import flow, including suki_ fields and slots.
To update a preset, pull it with --format hearthroom and use the version you
started editing. When a preset is already public, a successful push
updates the content that others can use or see according to its sharing settings.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if create {
				if strings.TrimSpace(title) == "" || cmd.Flags().Changed("id") || cmd.Flags().Changed("expected-version") {
					return output.Exitf(2, "--create requires --title and cannot be combined with --id or --expected-version")
				}
			} else if !presetIDPattern.MatchString(id) || version < 1 {
				return output.Exitf(2, "update requires --id and a positive --expected-version. Use --create for a new preset")
			} else if cmd.Flags().Changed("title") && strings.TrimSpace(title) == "" {
				return output.Exitf(2, "--title cannot be empty")
			}
			if key != "" && !presetOperationID.MatchString(key) {
				return output.Exitf(2, "--operation-id must contain 8 to 80 letters, digits, hyphens, or underscores")
			}
			raw, report, err := preset.Read(args[0], presetProfile(cmd, profile))
			if err != nil {
				return output.Exit(2, err)
			}
			if !create && report.Format != "hearthroom" {
				return output.Exitf(2, "updates need a Hearthroom document. Use `hearthroom preset pull --format hearthroom`")
			}
			if err := a.requirePresetAuth(); err != nil {
				return err
			}
			for _, warning := range report.Warnings {
				a.Out.Note("%s", warning)
			}
			if key == "" {
				key = rand.Text()
			}
			return a.pushPreset(cmd.Context(), args[0], raw, report, id, title, version, key)
		},
	}
	c.Flags().BoolVar(&create, "create", false, "create a private preset")
	c.Flags().StringVar(&id, "id", "", "preset ID to update")
	c.Flags().StringVar(&title, "title", "", "name of the new preset or replacement name for an update")
	c.Flags().StringVar(&key, "operation-id", "", "resume a save whose outcome could not be confirmed")
	c.Flags().Int64Var(&version, "expected-version", 0, "version of the preset when you started editing")
	c.Flags().Int64Var(&profile, "profile-id", 0, "SillyTavern prompt order profile ID for import")
	return c
}

func (a *App) pushPreset(ctx context.Context, file string, raw []byte, report preset.Report, id, title string, version int64, key string) error {
	var me Me
	if err := a.Client().OpenGet(ctx, "/me", nil, &me); err != nil {
		return reportPresetError(err)
	}
	if me.AccountNumID <= 0 {
		return errors.New("the provider did not identify the signed-in account")
	}
	write := presetWriteInput{Title: title, Document: raw, ExpectedVersion: version, RecommendedModels: []string{}}
	// Include account, provider, write mode, input bytes, and the chosen profile.
	// The recovery file stores the resulting fingerprint, never the input file.
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(fmt.Appendf(nil, "%q/%d/%q/%d/%q/%d/%x",
		a.API, me.AccountNumID, id, version, title, report.ProfileID, sha256.Sum256(raw))))
	dir := filepath.Join(a.Store.Dir, "preset-operations")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	statePath := filepath.Join(dir, key+".json")
	state, err := readPendingPresetWrite(statePath)
	if errors.Is(err, os.ErrNotExist) {
		state, err = a.preparePresetWrite(ctx, file, id, report, write)
		if err != nil {
			return err
		}
		state.Fingerprint = fingerprint
		encoded, err := json.Marshal(state)
		if err != nil {
			return err
		}
		if err := writeNewPrivateFile(statePath, encoded); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		state, err = readPendingPresetWrite(statePath)
	}
	if err != nil {
		return err
	}
	if state.Fingerprint != fingerprint {
		return output.Exitf(2, "this operation ID belongs to a different preset save. Retry with the original account, file, and options")
	}
	if (state.Preview != nil) != (id == "" && report.Format == "st") {
		return errors.New("the local preset recovery record does not match the save")
	}
	write.Title, write.RecommendedModels = state.Title, state.RecommendedModels
	a.Out.Note("Saving preset with operation ID %s.", key)
	receipt, err := a.sendPresetWrite(ctx, id, key, write, state.Preview)
	if err != nil {
		var remote *api.Error
		if !errors.As(err, &remote) || remote.Status >= 500 || remote.Status == http.StatusRequestTimeout {
			return fmt.Errorf("the provider did not confirm the save. Retry the same command with --operation-id %s", key)
		}
		// Retain the prepared request across reauthorization and rate limits.
		if remote.Status != http.StatusUnauthorized && remote.Status != http.StatusForbidden && remote.Status != http.StatusTooManyRequests {
			_ = os.Remove(statePath)
			return reportPresetError(err)
		}
		return fmt.Errorf("%w. To retry the same save, pass --operation-id %s", reportPresetError(err), key)
	}
	if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		a.Out.Note("Saved the preset. Could not remove its local recovery record.")
	}
	if a.Out.JSON {
		return a.Out.JSONValue(receipt)
	}
	a.Out.Line("Saved preset %s at version %d.", receipt.ID, receipt.Version)
	return nil
}

func readPendingPresetWrite(path string) (pendingPresetWrite, error) {
	var state pendingPresetWrite
	raw, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(raw, &state); err != nil || state.Fingerprint == "" ||
		(state.Preview != nil && !validPreview(*state.Preview)) {
		return state, errors.New("could not read the local preset recovery record")
	}
	return state, nil
}

func (a *App) preparePresetWrite(ctx context.Context, file, id string, report preset.Report, input presetWriteInput) (pendingPresetWrite, error) {
	state := pendingPresetWrite{Title: input.Title, RecommendedModels: input.RecommendedModels}
	if id != "" {
		var asset presetAsset
		if err := a.Client().OpenGet(ctx, "/presets/"+id, nil, &asset); err != nil {
			return state, reportPresetError(err)
		}
		if asset.Version != input.ExpectedVersion {
			return state, output.Exitf(2, "the preset changed (preset_conflict). Read the current version before trying another save")
		}
		if state.Title == "" {
			state.Title = asset.Title
		}
		state.RecommendedModels = asset.RecommendedModels
		return state, nil
	}
	if report.Format == "st" {
		var result struct {
			presetPreview
			Report struct {
				ReviewRequired bool `json:"reviewRequired"`
			} `json:"report"`
		}
		fields := map[string][]string{"profileId": {strconv.FormatInt(report.ProfileID, 10)}}
		if err := a.Client().UploadFile(ctx, "/preset-imports/preview", fields, "file", filepath.Base(file), "application/json", bytes.NewReader(input.Document), &result); err != nil {
			return state, reportPresetError(err)
		}
		if !validPreview(result.presetPreview) {
			return state, errors.New("the provider did not return a complete import preview")
		}
		if result.Report.ReviewRequired {
			return state, output.Exitf(2, "the import needs compatibility adjustments. Run `hearthroom preset check` or review the import preview in the web editor")
		}
		state.Preview = &result.presetPreview
	}
	return state, nil
}

func validPreview(preview presetPreview) bool {
	return presetIDPattern.MatchString(preview.ID) && preview.Version > 0 && preview.SelectionDigest != ""
}

func (a *App) sendPresetWrite(ctx context.Context, id, key string, write presetWriteInput, preview *presetPreview) (presetWriteReceipt, error) {
	path := "/presets"
	var body any = write
	if id != "" {
		path += "/" + id + "/revisions"
	}
	if preview != nil {
		path = "/preset-imports/" + preview.ID + "/confirm"
		body = struct {
			ExpectedPreviewVersion int64    `json:"expectedPreviewVersion"`
			SelectionDigest        string   `json:"selectionDigest"`
			Title                  string   `json:"title"`
			RecommendedModels      []string `json:"recommendedModels"`
		}{preview.Version, preview.SelectionDigest, write.Title, write.RecommendedModels}
	}
	var receipt presetWriteReceipt
	header := http.Header{"Idempotency-Key": {key}}
	if err := a.Client().OpenPostHeader(ctx, path, header, body, &receipt); err != nil {
		return receipt, err
	}
	if !presetIDPattern.MatchString(receipt.ID) || !presetIDPattern.MatchString(receipt.RevisionID) ||
		(id == "" && receipt.Version != 1) ||
		(id != "" && (!strings.EqualFold(receipt.ID, id) || receipt.Version <= write.ExpectedVersion)) {
		return receipt, errors.New("the provider did not return a complete save receipt")
	}
	return receipt, nil
}
