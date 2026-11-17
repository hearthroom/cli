package card

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// LoadContextDeclarations reads only contexts/, not card.json, instructions or
// Lorebooks. Card and Workshop authoring share the same private-file boundary.
func LoadContextDeclarations(root string) ([]json.RawMessage, bool, error) {
	entries, err := os.ReadDir(filepath.Join(root, ContextsDir))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var out []json.RawMessage
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".preset.json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		ref := ContextsDir + "/" + entry.Name()
		raw, err := readPrivateJSON(root, ref)
		if err != nil {
			return nil, false, err
		}
		normalized, _, err := expandContextDeclaration(root, raw, ref, id, false)
		if err != nil {
			return nil, false, err
		}
		out = append(out, normalized)
	}
	if len(out) > 32 {
		return nil, false, fmt.Errorf("A configuration can declare up to 32 contexts; reduce the file count before pushing.")
	}
	return out, len(out) > 0, nil
}

// ExpandContextDeclarations accepts an author's inline array, or relative
// private preset paths confined to root. Compiler metadata is never authority.
func ExpandContextDeclarations(root string, raw json.RawMessage) ([]json.RawMessage, error) {
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || len(entries) > 32 {
		return nil, fmt.Errorf("contexts must be an array with at most 32 declarations; use [] to clear it.")
	}
	out := make([]json.RawMessage, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		normalized, id, err := expandContextDeclaration(root, entry, "contexts", "", true)
		if err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, fmt.Errorf("Context IDs must be unique within this extension.")
		}
		seen[id] = true
		out = append(out, normalized)
	}
	return out, nil
}

func expandContextDeclaration(root string, raw json.RawMessage, label, expectedID string, allowInline bool) (json.RawMessage, string, error) {
	var c map[string]json.RawMessage
	if len(raw) > 2<<20 || json.Unmarshal(raw, &c) != nil || c == nil {
		return nil, "", fmt.Errorf("%s must contain a context declaration object no larger than 2 MiB.", label)
	}
	var id, name string
	if json.Unmarshal(c["id"], &id) != nil || !contextIDPattern.MatchString(id) || id == "main" || (expectedID != "" && id != expectedID) {
		return nil, "", fmt.Errorf("%s: id must match its filename, use 1–64 ASCII letters, digits, _ or -, and cannot be main.", label)
	}
	if json.Unmarshal(c["name"], &name) != nil || strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 100 || strings.ContainsAny(name, "\x00\r\n") {
		return nil, "", fmt.Errorf("%s: name must contain 1–100 characters on one line.", label)
	}
	var ref string
	preset := c["preset"]
	if json.Unmarshal(preset, &ref) == nil {
		var err error
		preset, err = readPrivateJSON(root, ref)
		if err != nil {
			return nil, "", fmt.Errorf("%s: %w", label, err)
		}
	} else if !allowInline {
		return nil, "", fmt.Errorf("%s: preset must name a private preset document file inside contexts/.", label)
	}
	var document map[string]json.RawMessage
	if json.Unmarshal(preset, &document) != nil || document == nil {
		return nil, "", fmt.Errorf("%s: the preset must be a JSON document object.", label)
	}
	c["preset"] = preset
	delete(c, "compilerVersion")
	delete(c, "compiledPreset")
	normalized, _ := json.Marshal(c)
	return normalized, id, nil
}
