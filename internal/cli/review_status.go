package cli

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hearthroom/cli/internal/output"
	"github.com/spf13/cobra"
)

type reviewSubmission struct {
	ID              string  `json:"id"`
	VersionID       *string `json:"versionId"`
	SubmittedAt     int64   `json:"submittedAt"`
	Kind            string  `json:"kind"`
	Status          string  `json:"status"`
	RejectionReason *string `json:"rejectionReason"`
	UpdateKind      *string `json:"updateKind"`
	SilentUpdate    *bool   `json:"silentUpdate"`
}
type reviewStatusResult struct {
	PublicVersionID  *string           `json:"publicVersionId"`
	LatestSubmission *reviewSubmission `json:"latestSubmission"`
}

func (a *App) cardReviewStatus() *cobra.Command {
	var roleID string
	c := &cobra.Command{
		Use: "review-status [dir]", Short: "Read your card's published version and latest review submission",
		Long: `Reads the current public version and latest review submission for a card you own.
Use a linked owned card folder (current directory by default), or --role with
its provider role ID or community card number. This only reads status; it does
not push, submit, change visibility or wait for a review to finish.

The latest submission is separate from the public version: an approved listing
can still have a pending or rejected update. JSON includes the submission ID,
versionId, submittedAt (Unix milliseconds), kind (first or re), status,
rejectionReason, updateKind and silentUpdate. Missing history or unavailable
legacy version/update metadata is null. Superseded submissions keep their
actual status. Only the author's visible rejection reason is returned.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("role") {
				if len(args) > 0 || strings.TrimSpace(roleID) == "" {
					return output.Exitf(2, "choose a card folder or --role with a card ID")
				}
				roleID = strings.TrimSpace(roleID)
			} else {
				f, err := loadFolder(args)
				if err != nil {
					return err
				}
				if f.State.Target != "owned" || f.State.RoleID == "" {
					return output.Exitf(2, "review status needs a linked owned card; use card push --create first")
				}
				if f.State.API != "" && f.State.API != a.Client().API {
					return output.Exitf(2, "this folder is linked to a different provider; select its --api")
				}
				roleID = f.State.RoleID
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			var result reviewStatusResult
			if err := a.Client().SiteDo(cmd.Context(), http.MethodGet, "/me/cards/"+url.PathEscape(roleID)+"/review-status", nil, &result); err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(result)
			}
			public := "none"
			if result.PublicVersionID != nil {
				public = *result.PublicVersionID
			}
			a.Out.Line("Public version: %s", public)
			sub := result.LatestSubmission
			if sub == nil {
				a.Out.Line("No review submissions.")
				return nil
			}
			a.Out.Line("Latest submission: %s", sub.ID)
			version := "unknown"
			if sub.VersionID != nil {
				version = *sub.VersionID
			}
			a.Out.Line("  Version: %s", version)
			a.Out.Line("  Submitted: %s", time.UnixMilli(sub.SubmittedAt).Local().Format(time.RFC3339))
			a.Out.Line("  Kind: %s", sub.Kind)
			a.Out.Line("  Status: %s", sub.Status)
			silent := "unknown"
			if sub.SilentUpdate != nil {
				silent = fmt.Sprint(*sub.SilentUpdate)
			}
			a.Out.Line("  Silent update: %s", silent)
			if sub.RejectionReason != nil && *sub.RejectionReason != "" {
				a.Out.Line("  Rejection reason: %s", *sub.RejectionReason)
			}
			return nil
		},
	}
	c.Flags().StringVar(&roleID, "role", "", "owned provider role ID or community card number (instead of a folder)")
	return c
}
