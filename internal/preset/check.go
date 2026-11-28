// Package preset checks the file structure and Hearthroom extensions offline.
// The Provider makes the final compatibility and persistence decisions.
package preset

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"unicode/utf8"
)

const schemaVersion = "hearthroom.st-preset.v1"
const sourceLimit = 1 << 20
const documentLimit = 2 << 20

type Report struct {
	Format    string   `json:"format"`
	ProfileID int64    `json:"profileId"`
	ToolReply bool     `json:"toolReply"`
	Warnings  []string `json:"warnings"`
}

var responseSlots = []string{
	"ruleSlot", "writingStyleSlot", "perspectiveSlot", "lengthSlot", "turnInputInstructionSlot",
}
var materialRoles = []string{
	"personaDescription", "charDescription", "dialogueExamples", "worldInfo", "memory", "userHistory",
}
var frameworkEntries = []string{
	"chatHistory", "worldInfoBefore", "worldInfoAfter", "personaDescription",
	"charDescription", "charPersonality", "scenario", "dialogueExamples",
}

type object = map[string]any

// Read returns the checked bytes to send, avoiding a second read of a file
// that could have changed after validation.
func Read(path string, profile *int64) ([]byte, Report, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, Report{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, Report{}, err
	}
	if !info.Mode().IsRegular() {
		return nil, Report{}, errors.New("choose a regular JSON file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, documentLimit+1))
	if err != nil {
		return nil, Report{}, err
	}
	report, err := Check(raw, profile)
	return raw, report, err
}

func Check(raw []byte, profile *int64) (Report, error) {
	report := Report{Format: "st", Warnings: []string{}}
	if len(raw) > documentLimit || !utf8.Valid(raw) {
		return report, errors.New("preset files must be UTF-8 JSON within the file size limit")
	}
	root, err := decodeObject(raw)
	if err != nil {
		return report, err
	}
	st := root
	var selection object
	if version, wrapped := root["schemaVersion"]; wrapped {
		report.Format = "hearthroom"
		if version != schemaVersion {
			return report, errors.New("expected schemaVersion " + schemaVersion)
		}
		if profile != nil {
			return report, errors.New("edit selection.profileId in a Hearthroom document")
		}
		st, _ = root["st"].(object)
		selection, _ = root["selection"].(object)
		selected, ok := integer(selection["profileId"])
		if st == nil || !ok {
			return report, errors.New("a Hearthroom document needs an st object and an integer selection.profileId")
		}
		profile = &selected
		if value, exists := selection["toolReply"]; exists {
			report.ToolReply, ok = value.(bool)
			if !ok {
				return report, errors.New("selection.toolReply must be true or false")
			}
		}
		// Preserve the source bytes for the server's size limit.
		var envelope struct{ ST json.RawMessage }
		_ = json.Unmarshal(raw, &envelope)
		if len(envelope.ST) > sourceLimit {
			return report, errors.New("the st object exceeds 1 MiB")
		}
	} else if len(raw) > sourceLimit {
		return report, errors.New("the SillyTavern file exceeds 1 MiB")
	}
	root, pack, err := source(st)
	if err != nil {
		return report, err
	}
	if err := checkSuki(root); err != nil {
		return report, err
	}
	for _, field := range []string{"frequency_penalty", "presence_penalty"} {
		value, exists := root[field]
		if !exists {
			continue
		}
		number, ok := value.(json.Number)
		n, err := number.Float64()
		if !ok || err != nil || n < -2 || n > 2 {
			if excludedPenalty(selection, field) {
				report.Warnings = append(report.Warnings, fmt.Sprintf("%s is excluded. Fix its value before creating a native preset.", field))
				continue
			}
			return report, fmt.Errorf("%s must be a number from -2 to 2", field)
		}
	}
	if selection == nil {
		report.ToolReply, _ = root["suki_tool_reply"].(bool)
	}
	prompts, err := promptIndex(root["prompts"])
	if err != nil {
		return report, err
	}
	orders, err := profileOrders(root["prompt_order"], pack, prompts)
	if err != nil {
		return report, err
	}
	id, err := selectProfile(orders, profile)
	if err != nil {
		return report, err
	}
	report.ProfileID = id
	excluded, err := checkSelection(selection, root, prompts)
	if err != nil {
		return report, err
	}
	err = checkSlots(root, orders[id], prompts, excluded, &report)
	return report, err
}

func excludedPenalty(selection object, field string) bool {
	items, _ := selection["excludedFields"].([]any)
	for _, item := range items {
		if item == field {
			return true
		}
	}
	return false
}

func source(st object) (object, bool, error) {
	value, pack := st["data"]
	if !pack {
		return st, false, nil
	}
	root, ok := value.(object)
	version, _ := integer(st["version"])
	if !ok || version != 1 || (st["type"] != "full" && st["type"] != "character") {
		return nil, true, errors.New("expected a version 1 full or character prompt pack")
	}
	return root, true, nil
}

func checkSuki(root object) error {
	for _, key := range []string{"suki_response_slots", "suki_tool_reply", "suki_takeover_last_user_message"} {
		if value, exists := root[key]; exists {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("%s must be true or false", key)
			}
		}
	}
	value, exists := root["suki_material_roles"]
	if !exists {
		return nil
	}
	roles, ok := value.(object)
	if !ok {
		return errors.New("suki_material_roles must be an object")
	}
	for key, value := range roles {
		if !slices.Contains(materialRoles, key) || (value != "system" && value != "user" && value != "assistant") {
			return errors.New("suki_material_roles requires a supported material and a system, user, or assistant role")
		}
	}
	return nil
}

func promptIndex(value any) (map[string]object, error) {
	list, ok := value.([]any)
	if !ok {
		return nil, errors.New("prompts must be an array")
	}
	out := make(map[string]object, len(list))
	for i, item := range list {
		row, _ := item.(object)
		id, _ := row["identifier"].(string)
		if id == "" || out[id] != nil {
			return nil, fmt.Errorf("prompts[%d] needs a unique identifier", i)
		}
		for _, key := range []string{"content", "role"} {
			if v := row[key]; v != nil {
				if _, ok := v.(string); !ok {
					return nil, fmt.Errorf("prompts[%d].%s must be a string", i, key)
				}
			}
		}
		for _, key := range []string{"injection_position", "injection_depth"} {
			if v := row[key]; v != nil {
				n, ok := integer(v)
				if !ok || n < 0 {
					return nil, fmt.Errorf("prompts[%d].%s must be a nonnegative integer", i, key)
				}
			}
		}
		out[id] = row
	}
	return out, nil
}

func profileOrders(value any, pack bool, prompts map[string]object) (map[int64][]any, error) {
	list, ok := value.([]any)
	if !ok {
		return nil, errors.New("prompt_order must be an array")
	}
	orders := map[int64][]any{}
	if pack {
		orders[0] = list
	} else {
		for i, item := range list {
			row, _ := item.(object)
			id, idOK := integer(row["character_id"])
			order, orderOK := row["order"].([]any)
			if !idOK || !orderOK || orders[id] != nil {
				return nil, fmt.Errorf("prompt_order[%d] needs a unique integer character_id and an order array", i)
			}
			orders[id] = order
		}
	}
	for _, order := range orders {
		seen := map[string]bool{}
		for _, item := range order {
			row, _ := item.(object)
			id, _ := row["identifier"].(string)
			_, enabled := row["enabled"].(bool)
			if !enabled || prompts[id] == nil || seen[id] {
				return nil, errors.New("each order entry needs a unique, declared prompt identifier and an enabled boolean")
			}
			seen[id] = true
		}
	}
	return orders, nil
}

func selectProfile(profiles map[int64][]any, wanted *int64) (int64, error) {
	ids := make([]int64, 0, len(profiles))
	for id := range profiles {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	if wanted != nil && slices.Contains(ids, *wanted) {
		return *wanted, nil
	}
	if len(ids) == 1 && wanted == nil {
		return ids[0], nil
	}
	if len(ids) == 0 {
		return 0, errors.New("the file has no prompt order profiles")
	}
	return 0, fmt.Errorf("choose a profile with --profile-id or selection.profileId. Available profiles: %v", ids)
}

func checkSelection(selection, root object, prompts map[string]object) (map[string]bool, error) {
	excluded := map[string]bool{}
	for _, key := range []string{"excludedEntryIds", "acceptedAdaptationIds", "excludedFields"} {
		if selection[key] == nil {
			continue
		}
		values, ok := selection[key].([]any)
		if !ok {
			return nil, fmt.Errorf("selection.%s must be an array", key)
		}
		seen := map[string]bool{}
		for _, value := range values {
			id, _ := value.(string)
			field := false
			switch key {
			case "excludedEntryIds":
				field = prompts[id] != nil
				excluded[id] = true
			case "acceptedAdaptationIds":
				entry, found := strings.CutSuffix(id, ":move_to_suffix")
				field = found && prompts[entry] != nil && !slices.Contains(frameworkEntries, entry)
			case "excludedFields":
				_, field = root[id]
				field = field && id != "prompts" && id != "prompt_order"
			}
			if id == "" || seen[id] || !field {
				return nil, fmt.Errorf("selection.%s must refer to distinct entries or fields in this preset", key)
			}
			seen[id] = true
		}
	}
	return excluded, nil
}

func checkSlots(root object, order []any, prompts map[string]object, excluded map[string]bool, report *Report) error {
	counts := make(map[string]int)
	afterHistory, lastUser := false, false
	for _, item := range order {
		row := item.(object)
		id := row["identifier"].(string)
		if id == "chatHistory" {
			afterHistory = true
		}
		if !row["enabled"].(bool) || excluded[id] || slices.Contains(frameworkEntries, id) {
			continue
		}
		prompt := prompts[id]
		content, _ := prompt["content"].(string)
		position, _ := integer(prompt["injection_position"])
		for _, macro := range visibleMacros(content) {
			switch {
			case slices.Contains(responseSlots, macro) || macro == "toolReplySlot":
				if !afterHistory || prompt["role"] != "system" || position != 0 {
					report.Warnings = append(report.Warnings, macro+" requires an ordered system entry after chatHistory with injection_position 0")
					continue
				}
				counts[macro]++
			case macro == "lastUserMessage":
				lastUser = true
				if !afterHistory {
					report.Warnings = append(report.Warnings, "Harbor may move an entry that uses lastUserMessage after chatHistory")
				}
			}
		}
	}
	total, distinct := 0, 0
	for _, slot := range responseSlots {
		if n := counts[slot]; n != 0 {
			total += n
			distinct++
		}
	}
	if total != 0 && (total != len(responseSlots) || distinct != len(responseSlots)) {
		return errors.New("include each of the five response slots exactly once in the enabled entries")
	}
	if root["suki_response_slots"] == true && total == 0 {
		report.Warnings = append(report.Warnings, "suki_response_slots is true, but no usable response slots were found")
	}
	if root["suki_takeover_last_user_message"] == true && !lastUser {
		report.Warnings = append(report.Warnings, "suki_takeover_last_user_message needs an enabled lastUserMessage slot")
	}
	return nil
}

// visibleMacros skips inline comment bodies, including their nested macros.
func visibleMacros(content string) []string {
	var macros []string
	commentDepth := 0
	for content != "" {
		start := strings.Index(content, "{{")
		end := strings.Index(content, "}}")
		if end >= 0 && (start < 0 || end < start) {
			if commentDepth > 0 {
				commentDepth--
			}
			content = content[end+2:]
			continue
		}
		if start < 0 {
			break
		}
		content = content[start+2:]
		if commentDepth > 0 || strings.HasPrefix(content, "//") {
			commentDepth++
			continue
		}
		if end = strings.Index(content, "}}"); end >= 0 {
			macros = append(macros, content[:end])
			content = content[end+2:]
		}
	}
	return macros
}

func integer(value any) (int64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	n, err := number.Int64()
	return n, err == nil
}

func decodeObject(raw []byte) (object, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := strictValue(decoder, 0)
	if err != nil {
		return nil, errors.New("invalid JSON, duplicate key, or excessive nesting")
	}
	doc, ok := value.(object)
	if _, err := decoder.Token(); !ok || err != io.EOF {
		return nil, errors.New("expected one JSON object")
	}
	return doc, nil
}

func strictValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, errors.New("JSON is too deeply nested")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		values := object{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			name := key.(string)
			if _, seen := values[name]; seen {
				return nil, errors.New("duplicate JSON key")
			}
			value, err := strictValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			values[name] = value
		}
		_, err := decoder.Token()
		return values, err
	case json.Delim('['):
		values := []any{}
		for decoder.More() {
			value, err := strictValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		_, err := decoder.Token()
		return values, err
	default:
		return token, nil
	}
}
