package card

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
)

// The rules for rules.json's sharedState, kept equal to the server's (provider role/shared_state.go):
// checked locally so card check reports a bad declaration before any upload.
var sharedKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type sharedPoll struct {
	Options []string `json:"options"`
	Share   bool     `json:"share,omitempty"`
}

type sharedCounter struct {
	Goal      int64  `json:"goal,omitempty"`
	MaxStep   int64  `json:"maxStep"`
	PerMember int64  `json:"perMember"`
	Period    string `json:"period,omitempty"`
}

// CheckSharedState returns the problems in a sharedState declaration (none for an empty one).
func CheckSharedState(raw json.RawMessage) []string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	var decl struct {
		Polls    map[string]sharedPoll    `json:"polls"`
		Counters map[string]sharedCounter `json:"counters"`
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decl); err != nil {
		return []string{RulesFile + ": sharedState must be {\"polls\": {...}, \"counters\": {...}}"}
	}
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, RulesFile+": sharedState "+fmt.Sprintf(format, args...))
	}
	if len(decl.Polls) > 16 || len(decl.Counters) > 16 {
		add("allows at most 16 polls and 16 counters")
	}
	for key, p := range decl.Polls {
		if !sharedKeyPattern.MatchString(key) {
			add("poll key %q must be 1-64 letters, digits, '.', '_' or '-', starting with a letter or digit", key)
		}
		if len(p.Options) < 2 || len(p.Options) > 12 {
			add("poll %q needs 2-12 options", key)
		}
		seen := map[string]bool{}
		for _, o := range p.Options {
			if !sharedKeyPattern.MatchString(o) || seen[o] {
				add("poll %q option %q must be unique and use the same characters as keys", key, o)
			}
			seen[o] = true
		}
	}
	for key, c := range decl.Counters {
		if !sharedKeyPattern.MatchString(key) {
			add("counter key %q must be 1-64 letters, digits, '.', '_' or '-', starting with a letter or digit", key)
		}
		if _, clash := decl.Polls[key]; clash {
			add("counter %q has the same key as a poll", key)
		}
		if c.MaxStep < 1 || c.MaxStep > 1000 {
			add("counter %q maxStep must be 1-1000", key)
		}
		if c.PerMember < 1 || c.PerMember > 1_000_000_000 {
			add("counter %q perMember must be 1-1000000000", key)
		}
		if c.Goal < 0 || c.Goal > 1_000_000_000_000 {
			add("counter %q goal must be 0-1000000000000", key)
		}
		switch c.Period {
		case "", "none", "day", "week":
		default:
			add("counter %q period must be none, day or week", key)
		}
	}
	return problems
}
