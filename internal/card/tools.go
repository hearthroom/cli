package card

import (
	"encoding/json"
	"github.com/hearthroom/cli/internal/cardtools"
)

func CheckTools(raw json.RawMessage) []string {
	if _, err := cardtools.Parse(raw); err != nil {
		return []string{"card.json: tools: invalid declaration; use the supported JSON Schema subset and limits"}
	}
	return nil
}
