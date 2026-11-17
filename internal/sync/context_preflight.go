package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

type ContextPreflight struct {
	ID                string   `json:"id"`
	FixedPrefixTokens int      `json:"fixedPrefixTokens"`
	Materials         []string `json:"materials"`
}
type ContextQuote struct {
	ID    string `json:"id"`
	Model struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"model"`
	ExpectedCharge      *int64 `json:"expectedCharge"`
	MaxCharge           int64  `json:"maxCharge"`
	RequestMaxCharge    int64  `json:"requestMaxCharge"`
	CompactionMaxCharge int64  `json:"compactionMaxCharge"`
	GeneratedAt         string `json:"generatedAt"`
	ExpiresAt           string `json:"expiresAt"`
}

func remoteContextPreflight(ctx context.Context, c *api.Client, f *card.Folder, p card.Payload, roleID string, res *PushResult) error {
	if !f.ContextsPresent && len(f.Manifest.Main) == 0 && f.Lorebooks == nil {
		return nil
	}
	var out struct {
		Contexts      []ContextPreflight `json:"contexts"`
		ContextQuotes []ContextQuote     `json:"contextQuotes"`
	}
	body := map[string]any{"fields": p.Card}
	if roleID != "" {
		body["roleId"] = roleID
	}
	if p.Worldbooks != nil {
		body["worldbooks"] = p.Worldbooks
	}
	if err := c.OpenPost(ctx, "/role/context-preflight", body, &out); err != nil {
		return fmt.Errorf("Context preflight was not accepted by the provider. Check that this provider supports contexts and has enabled them before pushing again: %w", err)
	}
	if len(out.Contexts) != len(f.Contexts) || len(out.ContextQuotes) != len(f.Contexts) {
		return fmt.Errorf("The provider returned incomplete context estimates; push stopped before any writes. Check the provider version before retrying.")
	}
	if len(f.Contexts) > 0 && (f.Rules == nil || f.Rules.PageMode != "sandbox") {
		res.Warnings = append(res.Warnings, "Context SDK calls require a sandbox page. Verify the saved page mode before trying this configuration.")
	}
	res.Contexts = out.Contexts
	res.ContextQuotes = out.ContextQuotes
	for _, raw := range f.Contexts {
		var declaration struct {
			ID         string `json:"id"`
			PlayerLore string `json:"playerLore"`
		}
		if err := json.Unmarshal(raw, &declaration); err != nil {
			return err
		}
		if declaration.PlayerLore == "allow" {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Context %s allows player Lorebook entries; keep high-value private material out of this context.", declaration.ID))
		}
	}
	return nil
}
func preflightNamedPayloads(f *card.Folder, p *card.Payload, owned bool, limits pushLimits, res *PushResult) error {
	if p.Worldbooks == nil {
		return nil
	}
	keys := make([]string, 0, len(p.Worldbooks))
	for key := range p.Worldbooks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	count := 0
	if p.Worldbook != nil {
		entries, _ := p.Worldbook["entries"].([]map[string]any)
		count = len(entries)
	}
	preparedBooks := map[string]any{}
	for _, key := range keys {
		source := f.Lorebooks[key]
		body, ok := p.Worldbooks[key].(map[string]any)
		if source == nil || !ok {
			return fmt.Errorf("Invalid Lorebook %s; reload the configuration folder before pushing.", key)
		}
		nested := *f
		nested.Lorebook = source
		nested.Lorebooks = nil
		nested.Contexts = nil
		nested.ContextsPresent = false
		localPayload := card.Payload{Worldbook: body}
		if _, err := preflight(&nested, localPayload, f.Manifest.Language, true); err != nil {
			return fmt.Errorf("lorebooks/%s.json: %w", key, err)
		}
		prepared, _, groups := prepareEntries(&nested, body, limits.entryMax)
		localPayload.Worldbook = prepared
		if err := preflightPrepared(localPayload, true, limits.entryMax, limits.TrialMaxEntries, false); err != nil {
			return fmt.Errorf("lorebooks/%s.json: %w", key, err)
		}
		entries, _ := prepared["entries"].([]map[string]any)
		count += len(entries)
		preparedBooks[key] = prepared
		if res != nil {
			res.Grouped = append(res.Grouped, groups...)
		}
	}
	if !owned && count > limits.TrialMaxEntries {
		return fmt.Errorf("Trial cards allow %d Lorebook entries in total; this configuration needs %d after grouping. Use --create for a larger owned configuration.", limits.TrialMaxEntries, count)
	}
	if !owned {
		p.Worldbooks = preparedBooks
	}
	return nil
}
