package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

// Report is GET /open/v1/role/validate.
type Report struct {
	Status            string          `json:"status"`
	Score             float64         `json:"score"`
	Blockers          []string        `json:"blockers"`
	Warnings          []string        `json:"warnings"`
	SuggestedFixes    []string        `json:"suggestedFixes"`
	QualityDimensions json.RawMessage `json:"qualityDimensions,omitempty"`
	TokenBudget       *TokenBudget    `json:"tokenBudget,omitempty"`
}

// TokenBudget carries the per-field limits and counts.
type TokenBudget struct {
	EstimatedTokens         int64            `json:"estimatedTokens"`
	TotalChars              int64            `json:"totalChars"`
	RoleDescChars           int64            `json:"roleDescChars"`
	RoleDetailDescChars     int64            `json:"roleDetailDescChars"`
	RoleWelcomeChars        int64            `json:"roleWelcomeChars"`
	RoleOutputContractChars int64            `json:"roleOutputContractChars"`
	CustomInstructionsChars int64            `json:"customInstructionsChars"`
	Limits                  map[string]int64 `json:"limits"`
	Guidance                []string         `json:"guidance"`
	Profile                 string           `json:"profile"`
}

// Validate fetches the server report for a card.
func Validate(ctx context.Context, c *api.Client, roleID string) (*Report, error) {
	var r Report
	if err := c.OpenGet(ctx, "/role/validate", url.Values{"roleId": {roleID}}, &r); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}
	if r.Blockers == nil {
		r.Blockers = []string{}
	}
	if r.Warnings == nil {
		r.Warnings = []string{}
	}
	if r.SuggestedFixes == nil {
		r.SuggestedFixes = []string{}
	}
	return &r, nil
}

// LocalCheck runs the checks that need no server: files present and
// parseable, referenced assets on disk. It returns problems, not an error.
func LocalCheck(f *card.Folder) []string {
	var problems []string
	if f.Manifest.Name == "" {
		problems = append(problems, card.ManifestFile+": name is empty")
	}
	if card.Body(f.Welcome) == "" {
		problems = append(problems, card.WelcomeFile+": opening message is empty")
	}
	if card.Body(f.Definition) == "" {
		problems = append(problems, card.DefinitionFile+": definition is empty")
	}
	if _, missing := f.AssetRefs(); len(missing) > 0 {
		for _, m := range missing {
			problems = append(problems, "referenced asset not found: "+m)
		}
	}
	for _, ref := range []string{f.Manifest.Media.Portrait, f.Manifest.Media.Background, f.Manifest.Media.BackgroundLandscape} {
		if ref != "" && !isURL(ref) {
			if _, err := os.Stat(filepath.Join(f.Dir, filepath.FromSlash(ref))); err != nil {
				problems = append(problems, "media reference not found: "+ref)
			}
		}
	}
	problems = append(problems, card.CheckResponseDefaults(f.Manifest.ResponseDefaults)...)
	if f.Rules != nil {
		switch f.Rules.MountLayer {
		case "", "under", "over", "cover":
		default:
			problems = append(problems, card.RulesFile+": mountLayer must be under, over or cover")
		}
		switch f.Rules.PageMode {
		case "", "classic", "immersive", "sandbox":
		default:
			problems = append(problems, card.RulesFile+": pageMode must be classic, immersive or sandbox")
		}
	}
	return problems
}

func isURL(s string) bool {
	return len(s) > 7 && (s[:7] == "http://" || (len(s) > 8 && s[:8] == "https://"))
}
