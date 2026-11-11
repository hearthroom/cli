package cli

import (
	"errors"
	"fmt"
	"github.com/hearthroom/cli/internal/api"
	"net/url"

	"github.com/spf13/cobra"
)

type approvedDraft struct {
	OperationID      string `json:"operationId"`
	ApprovedRoleID   string `json:"approvedRoleId"`
	ApprovalRevision string `json:"approvalRevision"`
	PolicyRevision   string `json:"policyRevision"`
}

func (a *App) cardRevert() *cobra.Command {
	var confirm bool
	c := &cobra.Command{Use: "revert <roleId>", Short: "Revert a card's draft to its published version", Long: `Replace this card's saved draft with its published version. Preview by default; add --confirm to save the change. This command has no undo. The published version stays unchanged. Linked library Lorebooks use their current content.

Local card folders are unchanged. Pull the saved draft afterwards if you keep a local copy.`, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.RequireAuth(); err != nil {
			return err
		}
		path := "/role/" + url.PathEscape(args[0]) + "/approved"
		var version approvedDraft
		if err := a.Client().OpenGet(cmd.Context(), path, nil, &version); err != nil {
			return revertError(err, false)
		}
		if confirm {
			if err := a.Client().OpenPost(cmd.Context(), path, version, &version); err != nil {
				return revertError(err, true)
			}
		}
		if a.Out.JSON {
			return a.Out.JSONValue(struct {
				approvedDraft
				Saved bool `json:"saved"`
			}{version, confirm})
		}
		if confirm {
			a.Out.Line("Draft restored to the published version. The published version is unchanged. Linked library Lorebooks use their current content.")
		} else {
			a.Out.Line("Preview only. Add --confirm to replace and save this card's draft with its published version. This command has no undo. Linked library Lorebooks use their current content.")
		}
		return nil
	}}
	c.Flags().BoolVar(&confirm, "confirm", false, "Replace and save the draft; this command has no undo")
	return c
}

func revertError(err error, saving bool) error {
	var remote *api.Error
	if saving && (!errors.As(err, &remote) || remote.Status >= 500) {
		return fmt.Errorf("The save result could not be confirmed. Check the card's draft before trying again.")
	}
	if errors.As(err, &remote) {
		switch remote.Status {
		case 401:
			return fmt.Errorf("Your sign-in has expired. Please sign in again.")
		case 403:
			return fmt.Errorf("You don't have permission to revert this card. Check your account and access.")
		case 409:
			return fmt.Errorf("The published version is unavailable or the card's status has changed. Check the card's status again.")
		case 503:
			return fmt.Errorf("Reverting is temporarily unavailable. Your draft is unchanged. Please try again later.")
		}
	}
	return err
}
