package cli

import (
	"crypto/rand"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/output"
	"github.com/spf13/cobra"
)

const presetBindingPath = "/player/preset-default"

type cardPresetBinding struct {
	Mode           string `json:"mode"`
	PresetID       string `json:"presetId"`
	Version        int64  `json:"version"`
	BindingVersion *int64 `json:"bindingVersion"`
	State          string `json:"state"`
}

func validBindingID(id string) bool {
	return id != "00000000-0000-0000-0000-000000000000" && presetIDPattern.MatchString(id)
}

func (b cardPresetBinding) source() string {
	if b.BindingVersion == nil || *b.BindingVersion < 0 {
		return ""
	}
	switch b.Mode {
	case "card_default":
		if b.State == "ready" && b.PresetID == "" {
			return "platform-default"
		}
	case "preset", "revision":
		if validBindingID(b.PresetID) {
			if b.State == "ready" && b.Version > 0 {
				return "custom"
			}
			if b.State == "unavailable" {
				return "unavailable"
			}
		}
	}
	return ""
}

func (a *App) presetBindingCommand(action, use, short string, checkArgs cobra.PositionalArgs) *cobra.Command {
	var roleID string
	c := &cobra.Command{
		Use: use, Short: short, Args: checkArgs,
		Long: short + ". Your choice applies to new turns in all your conversations with that card.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !validBindingID(roleID) {
				return output.Exitf(2, "--role requires a card ID in UUID form")
			}
			presetID, mode := "", "card_default"
			if action == "bind" {
				if !validBindingID(args[0]) {
					return output.Exitf(2, "use a preset ID from `hearthroom preset list`")
				}
				presetID, mode = args[0], "preset"
			}
			if err := a.requirePresetAuth(); err != nil {
				return err
			}
			ctx := cmd.Context()
			client := a.Client()
			var binding cardPresetBinding
			if err := client.OpenGet(ctx, presetBindingPath, url.Values{"roleId": {roleID}}, &binding); err != nil {
				return reportPresetBindingError(err, false)
			}
			if binding.source() == "" {
				return errors.New("the provider did not return the card's preset status")
			}
			if action != "status" {
				input := struct {
					RoleID          string `json:"roleId"`
					Scope           string `json:"scope"`
					Mode            string `json:"mode"`
					PresetID        string `json:"presetId,omitempty"`
					ExpectedVersion int64  `json:"expectedVersion"`
				}{roleID, "card", mode, presetID, *binding.BindingVersion}
				// Decode into a clean value so a partial receipt cannot inherit the
				// pre-write status or the previous preset ID.
				var saved cardPresetBinding
				header := http.Header{"Idempotency-Key": {rand.Text()}}
				if err := client.OpenPostHeader(ctx, presetBindingPath+"/save", header, input, &saved); err != nil {
					return reportPresetBindingError(err, true)
				}
				if source := saved.source(); source == "" || source == "unavailable" ||
					*saved.BindingVersion != input.ExpectedVersion+1 || !strings.EqualFold(saved.PresetID, presetID) {
					return unconfirmedPresetBinding()
				}
				binding = saved
			}
			return a.printPresetBinding(roleID, binding)
		},
	}
	c.Flags().StringVar(&roleID, "role", "", "card ID (UUID)")
	_ = c.MarkFlagRequired("role")
	return c
}

func (a *App) printPresetBinding(roleID string, binding cardPresetBinding) error {
	source := binding.source()
	if a.Out.JSON {
		return a.Out.JSONValue(struct {
			RoleID         string `json:"roleId"`
			Source         string `json:"source"`
			PresetID       string `json:"presetId,omitempty"`
			Version        int64  `json:"version,omitempty"`
			BindingVersion int64  `json:"bindingVersion"`
		}{roleID, source, binding.PresetID, binding.Version, *binding.BindingVersion})
	}
	switch source {
	case "platform-default":
		a.Out.Line("Platform default")
	case "custom":
		a.Out.Line("Preset %s (version %d)", binding.PresetID, binding.Version)
	case "unavailable":
		a.Out.Line("Preset %s is unavailable", binding.PresetID)
	}
	return nil
}

func unconfirmedPresetBinding() error {
	return errors.New("the provider did not confirm the change. Run `hearthroom preset status --role <role-id>` to check")
}

func reportPresetBindingError(err error, writing bool) error {
	var remote *api.Error
	isRemote := errors.As(err, &remote)
	if writing && (!isRemote || remote.Status >= 500 || remote.Status == http.StatusRequestTimeout) {
		return unconfirmedPresetBinding()
	}
	if !isRemote {
		return errors.New("could not read the card's preset status")
	}
	result := presetRemoteError{Status: remote.Status}
	switch remote.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		result.Hint = "card preset access was refused. Use a supported provider and sign in with `hearthroom auth login --presets`"
		return output.Exit(4, result)
	case http.StatusNotFound:
		result.Hint = "the card or preset is unavailable"
	case http.StatusConflict:
		result.Hint = "the selection could not be saved. Check `hearthroom preset status --role <role-id>` before trying again"
		result.Code = "preset_conflict"
		if remote.Code == "preset_revision_unavailable" {
			result.Hint = "that preset version is unavailable. Run `hearthroom preset list` to choose a preset"
			result.Code = remote.Code
		}
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		result.Hint = "the provider rejected this card or preset selection. Check the card's preset options on the website"
	default:
		result.Hint = "the provider could not read the card's preset status"
	}
	if remote.Status < 500 {
		return output.Exit(2, result)
	}
	return result
}
