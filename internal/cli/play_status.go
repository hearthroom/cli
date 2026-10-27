package cli

import (
	"github.com/hearthroom/cli/internal/play"
	"strings"
)

func (a *App) printPlayStatus(status play.OperationStatus, args []string, roleID string) {
	settlement := "pending (amount not final)"
	if status.Settled {
		settlement = "settled"
	}
	actions := strings.Join(status.AllowedActions, ", ")
	if actions == "" {
		actions = "none"
	}
	a.Out.Note("Generation: %s; credits: %d; settlement: %s; allowed actions: %s", status.State, status.ChargeCredits, settlement, actions)
	if status.CanContinue() {
		target := "--role " + playShellQuote(roleID)
		if len(args) > 0 {
			target = playShellQuote(args[0])
		}
		a.Out.Note("Continue saved preparation:")
		a.Out.Note("hearthroom play %s --continue --allow-spend", target)
		a.Out.Note("Add --model <model> to switch models.")
	}
}
func playShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
