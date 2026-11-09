package importer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/hearthroom/cli/internal/card"
)

var initVarTag = regexp.MustCompile(`(?i)\[initvar\]`)

const variableMacro = "{{get_message_variable::"

// SplitStateEntries takes SillyTavern MagVarUpdate (MVU) variables out of a
// Lorebook. Entries whose comment or name contains [InitVar] hold the initial
// variables as JSON (often hand-written JSON5); they are merged in order into
// the card variables (convention mvu) and leave the Lorebook.
//
//   - Entries that do not parse (usually YAML) stay as disabled entries so the
//     author still has the text to rewrite.
//   - Entries reading variables through {{get_message_variable::…}} are
//     disabled: the provider sends the current values and rules itself, and the
//     macro would reach the model unexpanded.
//   - <UpdateVariable> blocks in greetings are left as they are.
//   - MVU features the provider does not run ($meta.template, EJS, _.move) are
//     reported when they are cheap to spot.
//
// Mirrors splitStateEntries in hearthroom/web/src/lib/tavern.ts.
func SplitStateEntries(entries []BookEntry, greetings []string) (kept []BookEntry, vars *card.Variables, notes []string) {
	var merged *card.JSONNode
	used, unparsed, macro := 0, 0, 0
	metaTemplate := false
	for _, e := range entries {
		if initVarTag.MatchString(e.Comment + "\n" + e.Name) {
			if n, err := ParseRelaxedJSON(e.Content); err == nil && n.Kind == card.JSONObject {
				metaTemplate = metaTemplate || hasMetaTemplate(n)
				if merged == nil {
					merged = n
				} else {
					merged = card.MergeJSON(merged, n)
				}
				used++
			} else {
				unparsed++
				e.Enabled = false
				kept = append(kept, e)
			}
			continue
		}
		if strings.Contains(e.Content, variableMacro) {
			macro++
			e.Enabled = false
			kept = append(kept, e)
			continue
		}
		kept = append(kept, e)
	}
	if used > 0 {
		notes = append(notes, fmt.Sprintf("%d initial-variable entries ([InitVar]) became the card variables in %s instead of Lorebook entries", used, card.VariablesFile))
	}
	if unparsed > 0 {
		notes = append(notes, fmt.Sprintf("%d initial-variable entries are not JSON (possibly YAML); they were kept as disabled Lorebook entries, rewrite them in %s", unparsed, card.VariablesFile))
	}
	if macro > 0 {
		notes = append(notes, fmt.Sprintf("%d Lorebook entries read variables through {{get_message_variable::…}} macros; the card variables are sent with their rules instead, so these entries were disabled", macro))
	}
	if metaTemplate {
		notes = append(notes, "$meta.template (a template for new items) in the initial variables is not supported; new items will not follow it")
	}
	texts := make([]string, 0, len(entries)+len(greetings))
	for _, e := range entries {
		texts = append(texts, e.Content)
	}
	texts = append(texts, greetings...)
	ejs, move := false, false
	for _, t := range texts {
		ejs = ejs || strings.Contains(t, "<%")
		move = move || strings.Contains(t, "_.move")
	}
	if ejs {
		notes = append(notes, "the card uses EJS templates (<% %>); they are not run and the text was kept as is")
	}
	if move {
		notes = append(notes, "_.move (moving a value) in the variable rules is not supported")
	}
	if merged != nil {
		vars = &card.Variables{Convention: "mvu", Data: json.RawMessage(merged.Compact())}
	}
	return kept, vars, notes
}

func hasMetaTemplate(n *card.JSONNode) bool {
	switch n.Kind {
	case card.JSONArray:
		for _, v := range n.Arr {
			if hasMetaTemplate(v) {
				return true
			}
		}
	case card.JSONObject:
		if meta, ok := n.Get("$meta"); ok && meta.Kind == card.JSONObject {
			if _, ok := meta.Get("template"); ok {
				return true
			}
		}
		for _, v := range n.Vals {
			if hasMetaTemplate(v) {
				return true
			}
		}
	}
	return false
}
