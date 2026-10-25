package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/output"
	"github.com/spf13/cobra"
)

type visibilityResult struct {
	Error          string         `json:"error,omitempty"`
	Hint           string         `json:"hint,omitempty"`
	WorkVisibility map[string]any `json:"workVisibility,omitempty"`
	Operation      *struct {
		OperationID string `json:"operationId"`
		State       string `json:"state"`
	} `json:"operation,omitempty"`
}

func (a *App) cardVisibility() *cobra.Command {
	c := &cobra.Command{Use: "visibility", Short: "Read or change who can access the linked card"}
	get := &cobra.Command{Use: "get [dir]", Short: "Read visibility or check an unresolved operation", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return a.runVisibility(cmd, args, "") }}
	set := &cobra.Command{Use: "set <private|unlisted|public> [dir]", Short: "Change visibility without submitting for review", Long: `Changes access for the linked owned card. private lets the author and previous
players continue conversations or start new ones, with normal charges. People
who have never played the card cannot open it; it stays out of public listings.
Previous players retain play access to versions they have played and approved
versions of the same work. A source version played while unlisted also remains
playable.
Other unplayed drafts or versions awaiting review remain private. Play access
does not grant access to private instructions or permission to export the
configuration. unlisted shares the latest saved content with anyone who has
the link, without listing it. public restores an unchanged approved version;
review_required means the author must explicitly request card submit.
This command never submits for review.

An unresolved operation is kept in .hearthroom/state.json. Use visibility get
to check it, or repeat the same set to resume with the original operation id.`, Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		if args[0] != "private" && args[0] != "unlisted" && args[0] != "public" {
			return output.Exitf(2, "visibility must be private, unlisted, or public")
		}
		return a.runVisibility(cmd, args[1:], args[0])
	}}
	c.AddCommand(get, set)
	return c
}

func visibilityString(v map[string]any, key string) string { s, _ := v[key].(string); return s }
func visibilityAction(v map[string]any, target string) bool {
	wanted := map[string]string{"private": "set_private", "unlisted": "set_unlisted", "public": "restore_public"}[target]
	actions, _ := v["actions"].([]any)
	for _, action := range actions {
		if action == wanted {
			return true
		}
	}
	return false
}

func (a *App) runVisibility(cmd *cobra.Command, args []string, target string) error {
	f, err := loadFolder(args)
	if err != nil {
		return err
	}
	if f.State.Target != "owned" || f.State.RoleID == "" {
		return output.Exitf(2, "visibility needs a linked owned card; use card push --create first")
	}
	c := a.Client()
	if f.State.API != "" && f.State.API != c.API {
		return output.Exitf(2, "this folder is linked to a different provider")
	}
	if err = a.RequireAuth(); err != nil {
		return err
	}
	path := "/me/cards/" + url.PathEscape(f.State.RoleID) + "/visibility"
	read := func() (visibilityResult, error) {
		var r visibilityResult
		e := c.SiteDo(cmd.Context(), http.MethodGet, path, nil, &r)
		if e == nil && (r.WorkVisibility == nil || visibilityString(r.WorkVisibility, "syncState") == "") {
			e = fmt.Errorf("missing visibility projection")
		}
		return r, e
	}
	pending := f.State.VisibilityOperation
	actor := ""
	if target != "" || pending != nil {
		var me map[string]any
		if err = c.OpenGet(cmd.Context(), "/me", nil, &me); err != nil {
			return err
		}
		// Persist a one-way identity key, never the token or account number.
		identity := me["publicId"]
		if identity == nil {
			identity = me["accountNumId"]
		}
		if identity == nil {
			return fmt.Errorf("provider omitted account identity")
		}
		actor = card.Digest(identity)
	}
	clear := func() error { f.State.VisibilityOperation = nil; return f.SaveState() }
	finish := func(r visibilityResult) error {
		if r.Operation == nil || r.Operation.OperationID != pending.OperationID {
			return a.visibilityUnknown(r, "operation receipt is missing or does not match; use visibility get")
		}
		if r.Operation.State != "complete" || visibilityString(r.WorkVisibility, "syncState") != "complete" {
			return a.visibilityUnknown(r, "visibility is still pending; use visibility get")
		}
		if err := clear(); err != nil {
			return err
		}
		return a.printVisibility(r)
	}
	transmit := func() error {
		body := map[string]any{"operationId": pending.OperationID, "targetAccess": pending.TargetAccess, "expectedPolicyRevision": pending.ExpectedPolicyRevision, "expectedApprovalRevision": pending.ExpectedApprovalRevision}
		var result visibilityResult
		if err = c.SiteDo(cmd.Context(), http.MethodPost, path, body, &result); err != nil {
			var remote *api.Error
			if errors.As(err, &remote) && remote.Status >= 400 && remote.Status < 500 {
				// A conflict receipt can describe a persisted operation. Only an explicit
				// rejection with no operation is safe to clear without operation readback.
				var envelope struct {
					Operation json.RawMessage `json:"operation"`
				}
				_ = json.Unmarshal(remote.Envelope, &envelope)
				if len(envelope.Operation) == 0 {
					latest, e := read()
					if e != nil {
						return e
					}
					if visibilityString(latest.WorkVisibility, "syncState") != "complete" {
						return a.visibilityUnknown(latest, "visibility result is unknown; use visibility get")
					}
					if e = clear(); e != nil {
						return e
					}
					return err
				}
			}
			return a.visibilityUnknown(result, "visibility result is unknown; use visibility get before retrying")
		}
		return finish(result)
	}

	if pending != nil {
		if pending.API != c.API || pending.Site != c.Site || pending.RoleID != f.State.RoleID || pending.Actor != actor {
			return output.Exitf(2, "unresolved visibility operation belongs to a different provider, site, account, or card")
		}
		if target != "" && target != pending.TargetAccess {
			return output.Exitf(2, "an unresolved visibility operation must be checked before choosing another target")
		}
		var result visibilityResult
		err = c.SiteDo(cmd.Context(), http.MethodGet, path+"/operations/"+url.PathEscape(pending.OperationID), nil, &result)
		if err == nil {
			if target != "" && result.Operation != nil && result.Operation.OperationID == pending.OperationID && result.Operation.State == "pending" {
				return transmit()
			}
			return finish(result)
		}
		var remote *api.Error
		if errors.As(err, &remote) && remote.Status == 409 {
			var conflict visibilityResult
			_ = json.Unmarshal(remote.Envelope, &conflict)
			if conflict.Operation == nil || conflict.Operation.OperationID != pending.OperationID || conflict.Operation.State != "conflict" {
				return a.visibilityUnknown(result, "visibility result is unknown; use visibility get")
			}
			// The operation endpoint's conflict is durable and terminal. Refresh before
			// permitting a new intent; do not treat a failed refresh as safe to retry.
			latest, e := read()
			if e != nil {
				return e
			}
			if visibilityString(latest.WorkVisibility, "syncState") != "complete" {
				return a.visibilityUnknown(latest, "visibility is still changing; use visibility get")
			}
			if e = clear(); e != nil {
				return e
			}
			return err
		}
		if !errors.As(err, &remote) || remote.Status != 404 || remote.Code != "visibility_operation_not_found" {
			return a.visibilityUnknown(result, "visibility result is unknown; use visibility get")
		}
		latest, e := read()
		if e != nil {
			return e
		}
		if target == "" {
			return a.visibilityUnknown(latest, "operation not found; repeat the same visibility set to resume")
		}
		v := latest.WorkVisibility
		if visibilityString(v, "syncState") != "complete" || visibilityString(v, "policyRevision") != pending.ExpectedPolicyRevision || visibilityString(v, "approvalRevision") != pending.ExpectedApprovalRevision || !visibilityAction(v, pending.TargetAccess) {
			return a.visibilityUnknown(latest, "saved intent no longer matches current visibility; inspect before retrying")
		}
	} else {
		current, e := read()
		if e != nil {
			return e
		}
		if target == "" {
			return a.printVisibility(current)
		}
		if visibilityString(current.WorkVisibility, "syncState") != "complete" {
			return a.visibilityUnknown(current, "visibility is still changing; use visibility get")
		}
		// public must reach the authoritative endpoint so review_required remains a
		// structured server rejection rather than a client-invented approval rule.
		if target != "public" && !visibilityAction(current.WorkVisibility, target) {
			if retentionHint(current.WorkVisibility) != "" {
				current.Error = "card_reward_public_locked"
				current.Hint = retentionHint(current.WorkVisibility)
				if a.Out.JSON {
					if e := a.Out.JSONValue(current); e != nil {
						return e
					}
					return output.ExitQuiet(1, "%s", current.Hint)
				}
				return output.Exitf(1, "%s", current.Hint)
			}
			return output.Exitf(2, "this visibility action is not currently available")
		}
		id, e := newUUID()
		if e != nil {
			return e
		}
		pending = &card.VisibilityOperation{API: c.API, Site: c.Site, Actor: actor, RoleID: f.State.RoleID, OperationID: id, TargetAccess: target, ExpectedPolicyRevision: visibilityString(current.WorkVisibility, "policyRevision"), ExpectedApprovalRevision: visibilityString(current.WorkVisibility, "approvalRevision")}
		if pending.ExpectedPolicyRevision == "" || pending.ExpectedApprovalRevision == "" {
			return fmt.Errorf("provider omitted visibility revisions")
		}
		f.State.VisibilityOperation = pending
		if e = f.SaveState(); e != nil {
			return e
		}
	}
	return transmit()
}

func (a *App) printVisibility(r visibilityResult) error {
	if a.Out.JSON {
		return a.Out.JSONValue(r)
	}
	a.Out.Line("Visibility: %s", visibilityString(r.WorkVisibility, "access"))
	if hint := retentionHint(r.WorkVisibility); hint != "" {
		a.Out.Line("%s", hint)
	}
	return nil
}
func (a *App) visibilityUnknown(r visibilityResult, hint string) error {
	r.Error = "visibility_unconfirmed"
	r.Hint = hint
	if a.Out.JSON {
		if err := a.Out.JSONValue(r); err != nil {
			return err
		}
		return output.ExitQuiet(1, "%s", hint)
	}
	return output.Exitf(1, "%s", hint)
}

func retentionHint(v map[string]any) string {
	r, _ := v["rewardRetention"].(map[string]any)
	until, _ := r["publicUntil"].(string)
	if until == "" {
		return ""
	}
	if parsed, e := time.Parse(time.RFC3339Nano, until); e == nil {
		until = parsed.In(time.Local).Format("2006-01-02 15:04:05 MST")
	}
	return fmt.Sprintf("This card received a Creator Season reward. You cannot change it to “Only me” or “Anyone with the link” before %s.", until)
}
