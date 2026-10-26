package cli

import (
	"github.com/hearthroom/cli/internal/play"
	"strings"
)

func (a *App) printPlayStatus(status play.OperationStatus, args []string, roleID string) {
	settlement := "[COPY] not settled"
	if status.Settled {
		settlement = "[COPY] settled"
	}
	actions := strings.Join(status.AllowedActions, ", ")
	if actions == "" {
		actions = "[COPY] none"
	}
	a.Out.Note("[COPY] Generation: %s; charge: %d credits; settlement: %s; allowed actions: %s", status.State, status.ChargeCredits, settlement, actions)
	if status.CanContinue() {
		target := "--role " + playShellQuote(roleID)
		if len(args) > 0 {
			target = playShellQuote(args[0])
		}
		a.Out.Note("[COPY] Continue saved preparation: hearthroom play %s --continue --allow-spend. Add --model <model> to switch models.", target)
	}
}
func playShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
