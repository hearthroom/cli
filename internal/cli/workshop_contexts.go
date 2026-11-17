package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/hearthroom/cli/internal/card"
)

func expandWorkshopContexts(body map[string]json.RawMessage, file, root string) error {
	if root != "" {
		declarations, present, err := card.LoadContextDeclarations(root)
		if err != nil {
			return err
		}
		if !present {
			return fmt.Errorf("No context declarations were found in this folder's contexts/. Add declarations, or use contexts: [] in the draft JSON to clear them.")
		}
		body["contexts"], _ = json.Marshal(declarations)
		return nil
	}
	raw, present := body["contexts"]
	if !present {
		return nil
	}
	base := ""
	if file != "-" {
		base = filepath.Dir(file)
	}
	declarations, err := card.ExpandContextDeclarations(base, raw)
	if err != nil {
		return err
	}
	body["contexts"], _ = json.Marshal(declarations)
	return nil
}

func (a *App) workshopContextPreflight(ctx context.Context, body map[string]json.RawMessage) (json.RawMessage, error) {
	contexts, present := body["contexts"]
	if !present {
		return nil, nil
	}
	var declarations []json.RawMessage
	if json.Unmarshal(contexts, &declarations) != nil {
		return nil, fmt.Errorf("contexts must contain a declaration array; use [] to clear it.")
	}
	var raw json.RawMessage
	if err := a.Client().OpenPost(ctx, "/workshop/context-preflight", body, &raw); err != nil {
		return nil, fmt.Errorf("Workshop context preflight was not accepted. Check that this provider supports DLC contexts and has enabled them before updating again: %w", err)
	}
	var result struct {
		Kind     string `json:"kind"`
		Enabled  bool   `json:"cardContexts"`
		Contexts []struct {
			ID string `json:"id"`
		} `json:"contexts"`
		Quotes []struct {
			ID string `json:"id"`
		} `json:"contextQuotes"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Kind != "dlc" || !result.Enabled || len(result.Contexts) != len(declarations) || len(result.Quotes) != len(declarations) {
		return nil, fmt.Errorf("The provider returned incomplete DLC context estimates; the draft was not written. Check the provider version before retrying.")
	}
	seen, quoted := map[string]bool{}, map[string]bool{}
	for _, report := range result.Contexts {
		if seen[report.ID] {
			return nil, fmt.Errorf("The provider returned duplicate context reports; the draft was not written.")
		}
		seen[report.ID] = true
	}
	for _, quote := range result.Quotes {
		if quoted[quote.ID] {
			return nil, fmt.Errorf("The provider returned duplicate context quotes; the draft was not written.")
		}
		quoted[quote.ID] = true
	}
	for _, declaration := range declarations {
		var id struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(declaration, &id)
		if !seen[id.ID] || !quoted[id.ID] {
			return nil, fmt.Errorf("The provider omitted a declared context estimate; the draft was not written.")
		}
	}
	return raw, nil
}
