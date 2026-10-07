package card

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ResponseDefaults is card.json `responseDefaults`: the card author's defaults
// for the player's response preferences (agency, style, perspective, length,
// pace, plus customStyle, lengthTarget and one note per axis). They replace
// only the platform defaults; an axis the player changes uses the player's
// choice. Values are strings on the wire; lengthTarget may be written as a
// number in card.json.
type ResponseDefaults map[string]string

// UnmarshalJSON accepts string values and plain numbers (for lengthTarget).
func (r *ResponseDefaults) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		*r = nil
		return nil
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return fmt.Errorf("responseDefaults must be an object: %w", err)
	}
	out := ResponseDefaults{}
	for k, v := range all {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			out[k] = s
			continue
		}
		var n json.Number
		if err := json.Unmarshal(v, &n); err != nil {
			return fmt.Errorf("responseDefaults.%s must be a string", k)
		}
		out[k] = n.String()
	}
	*r = out
	return nil
}

var responseOptions = map[string][]string{
	"agency":      {"protect", "assist", "lines", "coauthor"},
	"style":       {"default", "guided", "card", "custom"},
	"perspective": {"card", "first_character", "second_user", "third_limited", "third_omniscient"},
	"length":      {"auto", "recommended", "target"},
	"pace":        {"natural", "linger", "advance"},
}

var responseNotes = []string{"agencyNote", "styleNote", "perspectiveNote", "lengthNote", "paceNote"}

// LengthTargetLadder is the set of lengthTarget values the server accepts
// (characters for Chinese, Japanese and Korean replies, words for English).
var LengthTargetLadder = []int{200, 300, 400, 500, 600, 800, 1000, 1200, 1500, 2000, 2500, 3000, 4000, 5000, 7000, 10000}

const (
	maxResponseNoteRunes  = 200
	maxCustomStyleRunes   = 1000
	responseDefaultsField = "responseDefaults"
)

// CheckResponseDefaults reports what the server would reject, in the
// server's own terms, so `hearthroom validate` catches it before a push.
func CheckResponseDefaults(r ResponseDefaults) []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, ManifestFile+": "+fmt.Sprintf(format, args...))
	}
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := r[k]
		switch {
		case responseOptions[k] != nil:
			if !containsString(responseOptions[k], v) {
				add("%s.%s: %q is not one of %s", responseDefaultsField, k, v, strings.Join(responseOptions[k], ", "))
			}
		case k == "lengthTarget":
			n, err := strconv.Atoi(v)
			if err != nil || !containsInt(LengthTargetLadder, n) {
				add("%s.lengthTarget: %q is not one of %s", responseDefaultsField, v, ladderText())
			}
		case k == "customStyle":
			if strings.TrimSpace(v) == "" || utf8.RuneCountInString(v) > maxCustomStyleRunes {
				add("%s.customStyle must not be blank and at most %d characters", responseDefaultsField, maxCustomStyleRunes)
			}
		case containsString(responseNotes, k):
			if strings.TrimSpace(v) == "" {
				add("%s.%s must not be blank", responseDefaultsField, k)
			} else if utf8.RuneCountInString(v) > maxResponseNoteRunes {
				add("%s.%s must be at most %d characters", responseDefaultsField, k, maxResponseNoteRunes)
			}
		default:
			add("%s: unknown key %s (allowed: agency, style, perspective, length, pace, customStyle, lengthTarget, %s)", responseDefaultsField, k, strings.Join(responseNotes, ", "))
		}
	}
	_, hasTarget := r["lengthTarget"]
	if r["length"] == "target" && !hasTarget {
		add("%s: length is target, so lengthTarget is required", responseDefaultsField)
	}
	if hasTarget && r["length"] != "target" {
		add("%s.lengthTarget only applies when length is target", responseDefaultsField)
	}
	_, hasCustom := r["customStyle"]
	if r["style"] == "custom" && !hasCustom {
		add("%s: style is custom, so customStyle is required", responseDefaultsField)
	}
	if hasCustom && r["style"] != "custom" {
		add("%s.customStyle only applies when style is custom", responseDefaultsField)
	}
	return problems
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func ladderText() string {
	parts := make([]string, len(LengthTargetLadder))
	for i, n := range LengthTargetLadder {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}
