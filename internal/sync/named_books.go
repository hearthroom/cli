package sync

import (
	"context"
	"fmt"
	"net/url"
	"sort"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

func pushNamedLorebooks(ctx context.Context, c *api.Client, f *card.Folder, roleID string, books map[string]any, opts PushOptions, res *PushResult) error {
	if books == nil {
		return nil
	}
	var bindings bindingsOutput
	if err := c.OpenGet(ctx, "/worldbook/bindings", url.Values{"roleId": {roleID}}, &bindings); err != nil {
		return fmt.Errorf("read named Lorebook bindings: %w", err)
	}
	if f.State.Lorebooks == nil {
		f.State.Lorebooks = map[string]card.LorebookSync{}
	}

	removals := map[string]string{}
	for key, state := range f.State.Lorebooks {
		if _, retained := books[key]; retained {
			continue
		}
		for _, binding := range bindings.Bindings {
			if binding.ContextKey != nil && *binding.ContextKey == key && binding.WorldbookID == state.ID {
				removals[key] = state.ID
			}
		}
	}
	if len(removals) > 0 && !opts.Prune {
		return fmt.Errorf("Tracked Lorebook sources would be unbound; rerun with --prune to remove only those managed by this folder.")
	}
	keys := make([]string, 0, len(books))
	for key := range books {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		source := f.Lorebooks[key]
		if source == nil {
			return fmt.Errorf("Lorebook %s has no local source file; reload the configuration folder before pushing.", key)
		}
		nested := *f
		local := *source
		local.Entries = append([]card.LorebookEntry(nil), source.Entries...)
		nested.Lorebook = &local
		state := f.State.Lorebooks[key]
		nested.State.LoadNamedBook(state)
		nested.SaveStateHook = func() error { f.State.Lorebooks[key] = nested.State.NamedBookState(state.Digest); return f.SaveState() }
		// Only a matching stable key may reconnect a named source. A different
		// binding, including the newest legacy binding, is never a fallback.
		bookOpts := opts
		bookOpts.contextKey = key
		bookOpts.mainRecall = source.MainRecall
		bookOpts.remoteBookID = ""
		bookOpts.remoteBookBound = false
		bookOpts.remoteBookVersion = 0
		for _, b := range bindings.Bindings {
			if b.ContextKey != nil && *b.ContextKey == key {
				if state.ID != "" && state.ID != b.WorldbookID {
					return fmt.Errorf("Lorebook %s now points to a different remote book; pull the configuration before pushing.", key)
				}
				nested.State.LorebookID = b.WorldbookID
				bookOpts.remoteBookID = b.WorldbookID
				bookOpts.remoteBookBound = true
				bookOpts.remoteBookName = b.Name
			}
		}
		local.ID = nested.State.LorebookID
		if local.ID == "" {
			nested.State.IgnoreLocalLorebook = true
		}
		bookOpts.sourceKeys = lorebookSourceKeys(source)
		for i := range local.Entries {
			if nested.State.IgnoreLocalLorebook {
				local.Entries[i].ID = ""
			}
			for _, alias := range bookOpts.sourceKeys[i] {
				if source.Entries[i].ID != "" && alias != "id:"+source.Entries[i].ID {
					continue
				}
				if remote := nested.State.LorebookSourceIDs[alias]; remote != "" {
					local.Entries[i].ID = remote
					break
				}
			}
		}
		payload, ok := books[key].(map[string]any)
		if !ok {
			return fmt.Errorf("Invalid payload for Lorebook %s; reload the configuration folder.", key)
		}

		digest := card.Digest(payload)
		if !opts.Force && state.Digest == digest && nested.State.LorebookID != "" && nested.State.LorebookUpload == nil {
			continue
		}
		if nested.State.LorebookID != "" {
			var detail worldbookDetailOutput
			if err := c.OpenGet(ctx, "/worldbook/detail", url.Values{"worldbookId": {nested.State.LorebookID}}, &detail); err != nil {
				return fmt.Errorf("read Lorebook %s before sync: %w", key, err)
			}
			bookOpts.remoteBookVersion = detail.Version
			if detail.Name != "" {
				bookOpts.remoteBookName = detail.Name
			}
			if detail.EntryContentMax > 0 {
				bookOpts.limits.entryMax = detail.EntryContentMax
			}
		}
		maxChars := bookOpts.limits.entryMax
		if maxChars <= 0 {
			maxChars = opts.limits.EntryContentMaxByLanguage[f.Manifest.Language]
		}
		if maxChars <= 0 {
			maxChars = 4000
		}
		prepared, entryKeys, _ := prepareEntries(&nested, payload, maxChars)
		bookOpts.entryKeys = entryKeys

		if err := pushLorebook(ctx, c, &nested, roleID, prepared, bookOpts, res); err != nil {
			return fmt.Errorf("sync Lorebook %s: %w", key, err)
		}
		f.State.Lorebooks[key] = nested.State.NamedBookState(digest)
		if err := f.SaveState(); err != nil {
			return err
		}
	}

	for key, id := range removals {
		if err := c.OpenPost(ctx, "/worldbook/"+url.PathEscape(id)+"/unbind", map[string]any{"roleId": roleID}, nil); err != nil {
			return fmt.Errorf("unbind tracked Lorebook %s: %w", key, err)
		}
		delete(f.State.Lorebooks, key)
		if err := f.SaveState(); err != nil {
			return err
		}
	}
	return nil
}
