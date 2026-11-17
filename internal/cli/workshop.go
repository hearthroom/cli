package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/hearthroom/cli/internal/output"
	"github.com/spf13/cobra"
)

func init() {
	extraCommands = append(extraCommands, func(a *App) []*cobra.Command { return []*cobra.Command{a.workshopCommand()} })
}

func (a *App) workshopCommand() *cobra.Command {
	c := &cobra.Command{
		Use: "workshop", Short: "Create, edit and publish author DLC or player Lorebook extensions",
		Long: `Manage extensions through the connected provider's Workshop API.
An original author can add openings, response formatting rules and Lorebook entries
as DLC. Other creators can add Lorebook entries only. The provider derives the kind
from ownership and checks the original author's opt-in (closed by default).

Writes require role.write; reads require role.read. These commands read your own
extension drafts, never the original agent configuration's private instructions,
rules or Lorebook. Publication has no content review or revenue share.

Create, update, publish and unpublish require --operation-id. Use a new token
for each intent and reuse it only to retry the exact same request.
Updates and publication actions require --expected-revision to
protect concurrent edits. Publication returns a provider receipt; community-site
projection may become available later. Every command accepts --json.`,
	}
	c.AddCommand(a.workshopList(), a.workshopCreate(), a.workshopGet(), a.workshopUpdate(), a.workshopAction("publish"), a.workshopAction("unpublish"), a.workshopPolicy(false), a.workshopPolicy(true))
	return c
}

func (a *App) workshopResult(raw json.RawMessage) error {
	if a.Out.JSON {
		return a.Out.JSONValue(raw)
	}
	// The same provider DTO is readable in human mode and directly reusable as
	// an update file. Do not reconstruct or fetch the original card's contents.
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	a.Out.Line("%s", data)
	return nil
}

func (a *App) workshopList() *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List your own Workshop drafts and publications", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			var raw json.RawMessage
			if err := a.Client().OpenGet(cmd.Context(), "/workshop/mine", nil, &raw); err != nil {
				return err
			}
			return a.workshopResult(raw)
		},
	}
}

func requiredWorkshopIntent(operationID string) error {
	if strings.TrimSpace(operationID) == "" || len(operationID) > 100 {
		return output.Exitf(2, "--operation-id must contain 1 to 100 UTF-8 bytes; reuse it only for a retry of the same request")
	}
	return nil
}

func (a *App) workshopCreate() *cobra.Command {
	var operationID string
	c := &cobra.Command{Use: "create <roleId>", Short: "Create an empty extension draft for an agent configuration", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requiredWorkshopIntent(operationID); err != nil {
				return err
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			var raw json.RawMessage
			if err := a.Client().OpenPost(cmd.Context(), "/workshop/create", map[string]any{"roleId": args[0], "idempotencyKey": operationID}, &raw); err != nil {
				return err
			}
			return a.workshopResult(raw)
		},
	}
	c.Flags().StringVar(&operationID, "operation-id", "", "stable intent token for retrying this create (required)")
	return c
}

func (a *App) workshopGet() *cobra.Command {
	return &cobra.Command{Use: "get <workshopId>", Short: "Read your extension draft as JSON, including its revision", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			var raw json.RawMessage
			if err := a.Client().OpenGet(cmd.Context(), "/workshop/draft", url.Values{"workshopId": {args[0]}}, &raw); err != nil {
				return err
			}
			return a.workshopResult(raw)
		},
	}
}

var workshopEditableFields = map[string]bool{
	"name": true, "summary": true, "readme": true, "coverUrl": true,
	"rating": true, "ratingAnswers": true, "worldbook": true, "rules": true,
	"opening": true, "updateNote": true,
	"contexts": true,
}

// A get response can be edited and sent back. Only editable extension fields
// are forwarded; derived identity and source-card metadata are never authority.
func workshopSaveBody(data []byte, workshopID string, revision int64, operationID string) (map[string]json.RawMessage, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil || body == nil {
		return nil, output.Exitf(2, "--file must contain one Workshop draft JSON object")
	}
	for key, value := range body {
		if workshopEditableFields[key] {
			continue
		}
		switch key {
		case "workshopId":
			var id string
			if json.Unmarshal(value, &id) != nil || id != workshopID {
				return nil, output.Exitf(2, "draft workshopId does not match the command target")
			}
		case "revision":
			var n int64
			if json.Unmarshal(value, &n) != nil || n != revision {
				return nil, output.Exitf(2, "draft revision differs from --expected-revision; read the current draft before editing")
			}
		case "kind", "card", "published", "changedSincePublish":
		default:
			return nil, output.Exitf(2, "unsupported Workshop draft field %q; use extension content fields only", key)
		}
		delete(body, key)
	}
	for key, value := range map[string]any{"workshopId": workshopID, "expectedRevision": revision, "idempotencyKey": operationID} {
		raw, _ := json.Marshal(value)
		body[key] = raw
	}
	return body, nil
}

func (a *App) workshopUpdate() *cobra.Command {
	var file, operationID, contextRoot string
	var revision int64
	c := &cobra.Command{Use: "update <workshopId> --file <draft.json>", Short: "Replace your draft's editable content with a JSON file", Args: cobra.ExactArgs(1),
		Long: `Replace all editable content in an extension draft (not a partial patch).
Start with "workshop get <workshopId> --json", edit that JSON, and pass the saved
revision with --expected-revision. Omitted fields are cleared by the provider.
Player extensions must omit rules and opening (empty structures from get are safe).
Use --file - to read from stdin. Local media paths are not uploaded or rewritten.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requiredWorkshopIntent(operationID); err != nil {
				return err
			}
			if !cmd.Flags().Changed("expected-revision") || revision < 1 {
				return output.Exitf(2, "--expected-revision must be a positive draft revision from workshop get")
			}
			if strings.TrimSpace(file) == "" {
				return output.Exitf(2, "--file is required (use '-' for stdin)")
			}
			data, err := readInputBytes(cmd, file)
			if err != nil {
				return output.Exit(2, err)
			}
			body, err := workshopSaveBody(data, args[0], revision, operationID)
			if err != nil {
				return err
			}
			if err := expandWorkshopContexts(body, file, contextRoot); err != nil {
				return err
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			preflight, err := a.workshopContextPreflight(cmd.Context(), body)
			if err != nil {
				return err
			}
			var raw json.RawMessage
			if err := a.Client().OpenPost(cmd.Context(), "/workshop/draft/save", body, &raw); err != nil {
				return err
			}
			if len(preflight) > 0 {
				var result map[string]json.RawMessage
				if json.Unmarshal(raw, &result) != nil || result == nil {
					return fmt.Errorf("The provider returned an invalid Workshop save result; read the draft before retrying.")
				}
				result["contextPreflight"] = preflight
				raw, _ = json.Marshal(result)
			}
			return a.workshopResult(raw)
		},
	}
	c.Flags().StringVar(&file, "file", "", "complete editable draft JSON, or '-' for stdin (required)")
	c.Flags().StringVar(&contextRoot, "context-root", "", "DLC folder containing contexts/ and private preset files; overrides contexts in --file")
	c.Flags().Int64Var(&revision, "expected-revision", 0, "require this draft revision before writing (required)")
	c.Flags().StringVar(&operationID, "operation-id", "", "stable intent token for retrying this update (required)")
	return c
}

func (a *App) workshopAction(action string) *cobra.Command {
	var operationID string
	var revision int64
	short := "Publish your draft directly, without content review"
	if action == "unpublish" {
		short = "Unpublish your extension while retaining its draft"
	}
	c := &cobra.Command{Use: action + " <workshopId>", Short: short, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requiredWorkshopIntent(operationID); err != nil {
				return err
			}
			if !cmd.Flags().Changed("expected-revision") || revision < 1 {
				return output.Exitf(2, "--expected-revision must be a positive draft revision from workshop get")
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			var raw json.RawMessage
			body := map[string]any{"workshopId": args[0], "expectedRevision": revision, "operationId": operationID}
			if err := a.Client().OpenPost(cmd.Context(), "/workshop/"+action, body, &raw); err != nil {
				return err
			}
			return a.workshopResult(raw)
		},
	}
	c.Flags().Int64Var(&revision, "expected-revision", 0, "require this draft revision (required)")
	c.Flags().StringVar(&operationID, "operation-id", "", "stable intent token for retrying this action (required)")
	return c
}

func (a *App) workshopPolicy(write bool) *cobra.Command {
	var allowNew bool
	var revision int64
	use, short := "policy <roleId>", "Read an agent configuration's Workshop opt-in policy"
	if write {
		use, short = "policy-set <roleId>", "Change Workshop opt-in for your original agent configuration"
	}
	c := &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if write && (!cmd.Flags().Changed("allow-new") || !cmd.Flags().Changed("expected-revision") || revision < 0) {
				return output.Exitf(2, "--allow-new=true|false and a nonnegative --expected-revision are required")
			}
			if err := a.RequireAuth(); err != nil {
				return err
			}
			var raw json.RawMessage
			var err error
			if write {
				err = a.Client().OpenPost(cmd.Context(), "/workshop/policy/save", map[string]any{"roleId": args[0], "allowNew": allowNew, "expectedRevision": revision}, &raw)
			} else {
				err = a.Client().OpenGet(cmd.Context(), "/workshop/policy", url.Values{"roleId": {args[0]}}, &raw)
			}
			if err != nil {
				return fmt.Errorf("workshop policy: %w", err)
			}
			return a.workshopResult(raw)
		},
	}
	if write {
		c.Flags().BoolVar(&allowNew, "allow-new", false, "allow new extensions; original-author access required")
		c.Flags().Int64Var(&revision, "expected-revision", 0, "require this policy revision (required; zero is valid)")
	}
	return c
}
