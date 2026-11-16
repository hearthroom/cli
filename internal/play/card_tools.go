package play

import (
	"context"
	"fmt"
	"github.com/hearthroom/cli/internal/api"
	"regexp"
)

var cardToolName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

// Capability fields identify the new Provider contract, even when the selected
// lane is unknown and will use its documented text fallback. An older Provider
// must never silently charge for a tool option it ignores.
func ensureCardToolContract(ctx context.Context, c *api.Client, opts TurnOptions) error {
	if !cardToolName.MatchString(opts.Tool) {
		return fmt.Errorf("invalid tool name; use a declared tool name from card.json")
	}
	if opts.ResumeFromOperationID != "" || (opts.AgentMode != nil && *opts.AgentMode) {
		return fmt.Errorf("card tools are not available in Agent mode; run a new turn with --agent off")
	}
	var groups []struct {
		Families []struct {
			Variants []struct {
				Capabilities map[string]string `json:"capabilities"`
			} `json:"variants"`
		} `json:"families"`
	}
	if err := c.OpenGet(ctx, "/models", nil, &groups); err != nil {
		return fmt.Errorf("cannot confirm card tool support; turn not sent: %w", err)
	}
	for _, g := range groups {
		for _, f := range g.Families {
			for _, v := range f.Variants {
				if _, ok := v.Capabilities["forcedTools"]; ok {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("this server does not support card tools yet; turn not sent")
}
