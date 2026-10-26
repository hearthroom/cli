package check

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/hearthroom/cli/internal/card"
)

// This portable contract is mirrored at hearthroom/web/src/lib/lorebook-regex.json.
// Update both copies and their original fixture corpus together.
//
//go:embed lorebook-regex.json
var lorebookRegexContract []byte

var lorebookRegexRules = func() []*regexp.Regexp {
	var rules struct {
		Wrapper    string
		Constructs []string
	}
	if err := json.Unmarshal(lorebookRegexContract, &rules); err != nil {
		panic(err)
	}
	out := []*regexp.Regexp{regexp.MustCompile(rules.Wrapper)}
	for _, pattern := range rules.Constructs {
		out = append(out, regexp.MustCompile(pattern))
	}
	return out
}()

func looksLikeBareRegex(key string) bool {
	key = strings.TrimSpace(key)
	if lorebookRegexRules[0].MatchString(key) {
		return false
	}
	for _, rule := range lorebookRegexRules[1:] {
		if rule.MatchString(key) {
			return true
		}
	}
	return false
}

func checkLorebookRegex(book card.Lorebook, r *report) {
	for i, entry := range book.Entries {
		for _, field := range []struct {
			name string
			keys []string
		}{{"keywords", entry.Keywords}, {"secondaryKeywords", entry.SecondaryKeywords}} {
			for j, key := range field.keys {
				if looksLikeBareRegex(key) {
					r.warnf(fmt.Sprintf("%s entries[%d].%s[%d]", card.LorebookFile, i, field.name, j), "This is matched as plain text; regular expressions need /…/ around them, for example /chapter[4-6]/. You can ignore this notice if plain text is intended.")
				}
			}
		}
	}
}
