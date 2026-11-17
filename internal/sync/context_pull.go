package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

var stableBookID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func pullNamedBooks(ctx context.Context, c *api.Client, f *card.Folder, bindings bindingsOutput, res *PullResult) (map[string]bool, error) {
	keep := map[string]bool{}
	old, _ := card.Load(f.Dir)
	for _, binding := range bindings.Bindings {
		if binding.ContextKey == nil || *binding.ContextKey == "" {
			continue
		}
		key := *binding.ContextKey
		if !stableBookID.MatchString(key) || keep[key] {
			return nil, errors.New("The provider returned invalid or duplicate Lorebook identities; pull stopped before replacing local files.")
		}
		keep[key] = true
		var detail worldbookDetailOutput
		var entries entryListOutput
		err := c.OpenGet(ctx, "/worldbook/detail", url.Values{"worldbookId": {binding.WorldbookID}}, &detail)
		if err == nil {
			err = c.OpenGet(ctx, "/worldbook/entry/list", url.Values{"worldbookId": {binding.WorldbookID}}, &entries)
		}
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Lorebook %s could not be read; its local file was retained: %v", key, err))
			if old != nil && old.Lorebooks[key] != nil {
				if f.Lorebooks == nil {
					f.Lorebooks = map[string]*card.Lorebook{}
				}
				f.Lorebooks[key] = old.Lorebooks[key]
				if f.State.Lorebooks == nil {
					f.State.Lorebooks = map[string]card.LorebookSync{}
				}
				f.State.Lorebooks[key] = old.State.Lorebooks[key]
			}
			continue
		}
		name := binding.Name
		if detail.Name != "" {
			name = detail.Name
		}
		nested := card.FromRemote(f.Dir, card.RemoteDetail{RoleID: f.State.RoleID, RoleName: f.Manifest.Name, Language: f.Manifest.Language}, binding.WorldbookID, name, entries.Entries, nil)
		book := nested.Lorebook
		book.ID = key
		book.Description = detail.Description
		book.Format = detail.Format
		book.MainRecall = binding.MainRecall
		nested.State.LorebookVersion = detail.Version
		nested.State.LorebookName = name
		recordPulledLorebookHashes(nested, detail.EntryContentMax)
		if f.Lorebooks == nil {
			f.Lorebooks = map[string]*card.Lorebook{}
		}
		f.Lorebooks[key] = book
		if f.State.Lorebooks == nil {
			f.State.Lorebooks = map[string]card.LorebookSync{}
		}
		payload, err := (&card.Folder{Manifest: card.Manifest{Name: f.Manifest.Name}, Lorebooks: map[string]*card.Lorebook{key: book}}).Build(nil)
		if err != nil {
			return nil, err
		}
		f.State.Lorebooks[key] = nested.State.NamedBookState(card.Digest(payload.Worldbooks[key]))
		res.Files = append(res.Files, card.LorebooksDir+"/"+key+".json")
	}
	return keep, nil
}
func staleContextFiles(f *card.Folder) ([]string, error) {
	keep := map[string]bool{}
	for _, raw := range f.Contexts {
		var c struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &c) != nil || !stableBookID.MatchString(c.ID) {
			return nil, errors.New("The provider returned an invalid context id; pull into a new folder after checking the provider response.")
		}
		keep[c.ID+".json"] = true
		keep[c.ID+".preset.json"] = true
	}
	return stalePrivateFiles(f.Dir, card.ContextsDir, keep)
}
func stalePrivateFiles(root, folder string, keep map[string]bool) ([]string, error) {
	dir := filepath.Join(root, folder)
	st, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s must be a directory, not a symbolic link; pull into a new folder.", folder)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var stale []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") && !keep[entry.Name()] {
			stale = append(stale, folder+"/"+entry.Name())
		}
	}
	return stale, nil
}
