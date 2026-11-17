package card

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const ContextsDir = "contexts"
const LorebooksDir = "lorebooks"

var contextIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// confinedFile rejects symlink escapes as well as lexical traversal. Private
// presets are never fetched from URLs or resolved outside this card folder.
func confinedFile(root, ref string) (string, error) {
	if ref == "" || filepath.IsAbs(ref) || strings.Contains(ref, "\\") {
		return "", errors.New("Use a preset file path inside this configuration folder.")
	}
	clean := filepath.Clean(filepath.FromSlash(ref))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("Preset paths must stay inside this configuration folder.")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	candidate, err := filepath.EvalSymlinks(filepath.Join(root, clean))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(realRoot, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("The preset path leaves this configuration folder; copy the file into contexts/.")
	}
	st, err := os.Stat(candidate)
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", errors.New("The preset must be a regular JSON file inside contexts/.")
	}
	return candidate, nil
}
func readPrivateJSON(root, ref string) ([]byte, error) {
	path, err := confinedFile(root, ref)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Size() > 2<<20 {
		return nil, fmt.Errorf("%s exceeds 2 MiB; reduce it before pushing.", ref)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return nil, fmt.Errorf("%s is not valid UTF-8 JSON; correct the file before pushing.", ref)
	}
	return raw, nil
}
func (f *Folder) loadContextsAndBooks() error {
	for _, folder := range []string{ContextsDir, LorebooksDir} {
		entries, err := os.ReadDir(filepath.Join(f.Dir, folder))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".preset.json") {
				continue
			}
			id := strings.TrimSuffix(entry.Name(), ".json")
			if !contextIDPattern.MatchString(id) {
				return fmt.Errorf("%s: use a stable 1–64 character ASCII filename (letters, digits, _ or -).", folder)
			}
			ref := folder + "/" + entry.Name()
			raw, err := readPrivateJSON(f.Dir, ref)
			if err != nil {
				return err
			}
			if folder == LorebooksDir {
				var book Lorebook
				if json.Unmarshal(raw, &book) != nil || book.ID != "" && book.ID != id {
					return fmt.Errorf("%s: id must match its filename.", ref)
				}
				book.ID = id
				if f.Lorebooks == nil {
					f.Lorebooks = map[string]*Lorebook{}
				}
				f.Lorebooks[id] = &book
				continue
			}
			var c map[string]json.RawMessage
			if json.Unmarshal(raw, &c) != nil || c == nil {
				return fmt.Errorf("%s must contain a context declaration object.", ref)
			}
			var declared, name, presetRef string
			if json.Unmarshal(c["id"], &declared) != nil || declared != id || id == "main" {
				return fmt.Errorf("%s: id must match its filename and cannot be main.", ref)
			}
			if json.Unmarshal(c["name"], &name) != nil || strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 100 || strings.ContainsAny(name, "\x00\r\n") {
				return fmt.Errorf("%s: name must contain 1–100 characters on one line.", ref)
			}
			if json.Unmarshal(c["preset"], &presetRef) != nil {
				return fmt.Errorf("%s: preset must name a private preset document file inside contexts/.", ref)
			}
			preset, err := readPrivateJSON(f.Dir, presetRef)
			if err != nil {
				return fmt.Errorf("%s: %w", ref, err)
			}
			var document map[string]json.RawMessage
			if json.Unmarshal(preset, &document) != nil || document == nil {
				return fmt.Errorf("%s: the preset must be a JSON document object.", ref)
			}
			c["preset"] = preset
			delete(c, "compilerVersion")
			delete(c, "compiledPreset")
			normalized, _ := json.Marshal(c)
			f.Contexts = append(f.Contexts, normalized)
			f.ContextsPresent = true
		}
	}
	if len(f.Manifest.Contexts) > 0 {
		if string(bytes.TrimSpace(f.Manifest.Contexts)) != "[]" {
			return errors.New("card.json: use contexts: [] only to remove declarations; write declarations in contexts/.")
		}
		if len(f.Contexts) == 0 {
			f.ContextsPresent = true
		}
	}
	if len(f.Manifest.Lorebooks) > 0 {
		if string(bytes.TrimSpace(f.Manifest.Lorebooks)) != "[]" {
			return errors.New("card.json: use lorebooks: [] only to remove tracked sources; write books in lorebooks/.")
		}
		if f.Lorebooks == nil {
			f.Lorebooks = map[string]*Lorebook{}
		}
	}
	if len(f.Contexts) > 32 {
		return errors.New("A configuration can declare up to 32 contexts; combine or remove declarations before pushing.")
	}
	if len(f.Lorebooks) > 64 {
		return errors.New("A configuration can bind up to 64 Lorebooks; reduce the file count before pushing.")
	}
	return nil
}
func writePrivateJSON(path string, v any) error {
	if err := privateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	// Existing symlinks are never followed while saving private material.
	if st, err := os.Lstat(path); err == nil && st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("Refusing to overwrite a symlink: %s", filepath.Base(path))
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0600)
}
func (f *Folder) saveContextsAndBooks() error {
	if f.Lorebooks != nil && len(f.Lorebooks) == 0 {
		f.Manifest.Lorebooks = json.RawMessage(`[]`)
	} else {
		f.Manifest.Lorebooks = nil
	}
	if f.ContextsPresent && len(f.Contexts) == 0 {
		f.Manifest.Contexts = json.RawMessage(`[]`)
	} else {
		f.Manifest.Contexts = nil
	}

	if f.ContextsPresent {
		if err := privateDirectory(filepath.Join(f.Dir, ContextsDir)); err != nil {
			return err
		}
		for _, raw := range f.Contexts {
			var c map[string]json.RawMessage
			if json.Unmarshal(raw, &c) != nil {
				return errors.New("Cannot save an invalid context declaration.")
			}
			var id string
			_ = json.Unmarshal(c["id"], &id)
			if !contextIDPattern.MatchString(id) || id == "main" {
				return errors.New("Cannot save an invalid context id.")
			}
			preset := c["preset"]
			ref := ContextsDir + "/" + id + ".preset.json"
			if err := writePrivateJSON(filepath.Join(f.Dir, filepath.FromSlash(ref)), preset); err != nil {
				return err
			}
			c["preset"], _ = json.Marshal(ref)
			delete(c, "compilerVersion")
			delete(c, "compiledPreset")
			if err := writePrivateJSON(filepath.Join(f.Dir, ContextsDir, id+".json"), c); err != nil {
				return err
			}
		}
	}
	keys := make([]string, 0, len(f.Lorebooks))
	for key := range f.Lorebooks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !contextIDPattern.MatchString(key) {
			return errors.New("Cannot save an invalid Lorebook id.")
		}
		if err := writePrivateJSON(filepath.Join(f.Dir, LorebooksDir, key+".json"), f.Lorebooks[key]); err != nil {
			return err
		}
	}
	return nil
}
func (f *Folder) contextCardFields(fields map[string]any) error {
	if f.ContextsPresent {
		contexts := f.Contexts
		if contexts == nil {
			contexts = []json.RawMessage{}
		}
		fields["contexts"] = contexts
	}
	if len(f.Manifest.Main) > 0 {
		var main struct {
			Assembly string `json:"assembly"`
		}
		if json.Unmarshal(f.Manifest.Main, &main) != nil || (main.Assembly != "platform" && main.Assembly != "client") {
			return errors.New("card.json: main.assembly must be platform or client.")
		}
		fields["main"] = f.Manifest.Main
	}
	return nil
}

func privateDirectory(path string) error {
	if st, err := os.Lstat(path); err == nil && (!st.IsDir() || st.Mode()&os.ModeSymlink != 0) {
		return errors.New("Private context and Lorebook folders cannot be symbolic links; pull into a new folder.")
	}
	return os.MkdirAll(path, 0700)
}
