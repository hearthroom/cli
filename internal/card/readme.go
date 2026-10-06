package card

import (
	"os"
	"path/filepath"
)

// ReadmeFile holds the card's working notes. It is never sent to the provider; the
// checker reads two declarations from it (uiRole, statusOverheadThreshold).
const ReadmeFile = "README.md"

// ReadmeTemplate is the dossier written by `card init`: the resume point for whoever
// works on the card next, an agent included.
func ReadmeTemplate(name string) string {
	return "# " + name + " — working notes (never sent)\n\n" +
		"uiRole: assist\n" +
		"statusOverheadThreshold: 15%   # a core card declares its own number and says why\n\n" +
		"## Decisions\n\n" +
		"- <date> <decision> — because …\n\n" +
		"## Rejected directions\n\n" +
		"- <direction> — rejected because …; what it would have changed\n\n" +
		"## Evidence by version\n\n" +
		"| version | check | validate | render | protocol health (block rate / overhead) | L0 | L1 | L2 | L3 | weakest |\n" +
		"|---|---|---|---|---|---|---|---|---|---|\n\n" +
		"## Open shortcomings (one per version)\n\n" +
		"- …\n"
}

// WriteReadme writes README.md unless the author already has one.
func WriteReadme(dir, name string) (bool, error) {
	path := filepath.Join(dir, ReadmeFile)
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	return true, os.WriteFile(path, []byte(ReadmeTemplate(name)), 0o644)
}
