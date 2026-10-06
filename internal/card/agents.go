package card

import (
	"os"
	"path/filepath"
)

// AgentsFile is read automatically by coding agents (Claude Code, Codex and
// others) when they start working inside the folder. It is never sent to the
// provider.
const AgentsFile = "AGENTS.md"

// SkillsRepo is the open-source toolkit that teaches an agent how to write a
// good card, as opposed to merely moving one.
const SkillsRepo = "https://github.com/hearthroom/skills"

// AgentsGuide is the text written to AGENTS.md for a card named name.
func AgentsGuide(name string) string {
	return "# " + name + "\n\n" +
		"This folder is a Hearthroom character card. Long text lives in Markdown\n" +
		"(`" + DefinitionFile + "`, `" + WelcomeFile + "`, `openings/alt-NN.md`), short fields in `" + ManifestFile + "`,\n" +
		"the Lorebook in `" + LorebookFile + "`, display rules in `" + RulesFile + "`, media under `" + AssetsDir + "/`\n" +
		"(`media.portrait`, `media.background` for the portrait 9:16 background, and `media.backgroundLandscape`\n" +
		"for the landscape 16:9 background wide screens prefer; keep key elements in the central 75%).\n" +
		"Format: https://cli.hearthroom.club/guides/card-folder/\n\n" +
		"## Before writing\n\n" +
		"Install the writing skills and start from their router: " + SkillsRepo + "\n" +
		"(`claude plugin marketplace add hearthroom/skills` then `claude plugin install hearthroom`,\n" +
		"or read skills/using-hearthroom/SKILL.md from that repository). They cover premise,\n" +
		"character, Lorebook, openings, voice, state, presentation, diagnosis and iteration;\n" +
		"every platform fact they rely on is in references/platform-facts.md there.\n\n" +
		"## Media\n\n" +
		"`" + AssetsDir + "/` is uploaded into one media-library folder, `media.folder` in `" + ManifestFile + "`\n" +
		"(the card name by default), and `" + AssetsDir + "/<path>` is served at `<libraryPrefix>/<folder>/<path>`.\n" +
		"Keep one card folder for the card's whole life and iterate in it. Name files for\n" +
		"what they show and group them by job (`" + AssetsDir + "/art/expr/happy.webp`), never by hash,\n" +
		"date or version. A directory reference (`" + AssetsDir + "/art/expr/$1.webp` in a rule) uploads\n" +
		"the whole directory, so file names can match the values the card emits.\n\n" +
		"## The loop\n\n" +
		"```sh\n" +
		"hearthroom card check .                    # local: rules, markers, sandbox API use; free, no sign-in\n" +
		"hearthroom card push . --validate --json   # private trial card + the provider's report\n" +
		"hearthroom card render . --json            # the opening after display rules, per-rule outcome\n" +
		"hearthroom card preview . --open           # the real sandbox shell on a local port, offline\n" +
		"hearthroom play . --new-session -m \"...\" --allow-spend --json   # one real turn; spends the author's credits\n" +
		"hearthroom card check . --replay history.txt   # protocol health from real replies\n" +
		"hearthroom lorebook build .                # worldbook/*.md -> lorebook.json\n" +
		"```\n\n" +
		"`" + ReadmeFile + "` is the card's working notes (never sent): `uiRole: assist | core`, the status\n" +
		"overhead threshold, decisions, rejected directions and the evidence of each version.\n" +
		"Read it first; keep it current.\n\n" +
		"Pushing, validating and rendering are free. Only `play -m` spends credits and\n" +
		"needs `--allow-spend`; ask the author before the first turn. Do not run\n" +
		"`card push --create` or submit the card for review unless asked.\n" +
		"Manual: https://cli.hearthroom.club/llms-full.txt\n"
}

// WriteAgentsGuide writes AGENTS.md unless the author already has one.
func WriteAgentsGuide(dir, name string) (bool, error) {
	path := filepath.Join(dir, AgentsFile)
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	return true, os.WriteFile(path, []byte(AgentsGuide(name)), 0o644)
}
