// Package sync moves card folders to and from the provider.
package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/media"
)

// PushOptions control a push.
type PushOptions struct {
	// To targets an owned private card instead of a trial card.
	To string
	// Create makes a new private card and targets it (implies owned).
	Create bool
	// Evict frees a trial slot when all are used.
	Evict bool
	// DryRun computes what would be sent without sending.
	DryRun bool
	// Force sends every section even if the hash is unchanged.
	Force bool
	// SkipMedia leaves assets alone and keeps previously recorded URLs.
	SkipMedia bool
}

// PushResult reports what happened.
type PushResult struct {
	Target     string            `json:"target"`
	RoleID     string            `json:"roleId"`
	TrialKey   string            `json:"trialKey,omitempty"`
	Created    bool              `json:"created"`
	ExpiresAt  string            `json:"expiresAt,omitempty"`
	Changed    []string          `json:"changed"`
	Unchanged  []string          `json:"unchanged"`
	Skipped    []string          `json:"skipped,omitempty"`
	Assets     []media.Outcome   `json:"assets,omitempty"`
	MediaDir   string            `json:"mediaFolder,omitempty"`
	Slots      map[string]any    `json:"slots,omitempty"`
	Sections   map[string]string `json:"sections,omitempty"`
	DryRun     bool              `json:"dryRun,omitempty"`
	AssetError bool              `json:"assetErrors,omitempty"`
}

var trialKeyRe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// trialKeyFor derives a stable-looking key from the folder name plus a random
// suffix so two folders with the same name do not collide.
func trialKeyFor(dir string) (string, error) {
	base := trialKeyRe.ReplaceAllString(filepath.Base(dir), "-")
	base = strings.Trim(base, "-_")
	if base == "" {
		base = "card"
	}
	if len(base) > 40 {
		base = base[:40]
	}
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base + "-" + hex.EncodeToString(buf[:]), nil
}

// Push syncs the folder. It saves the folder state on success and on partial
// failure (so already-uploaded assets are not re-uploaded).
func Push(ctx context.Context, c *api.Client, f *card.Folder, opts PushOptions) (*PushResult, error) {
	res := &PushResult{DryRun: opts.DryRun}
	if opts.Create && opts.To != "" {
		return nil, errors.New("--create and --to cannot be combined")
	}
	owned := opts.Create || opts.To != "" || (f.State.Target == "owned" && f.State.RoleID != "")
	if opts.To != "" && f.State.RoleID != "" && f.State.RoleID != opts.To && f.State.Target == "owned" && !opts.Force {
		return nil, fmt.Errorf("this folder is linked to card %s; pass --to %s again with --force to relink", f.State.RoleID, opts.To)
	}

	// Media first: the payload needs the served URLs.
	urls := map[string]string{}
	for p, a := range f.State.Assets {
		if a.URL != "" {
			urls[p] = a.URL
		}
	}
	if !opts.SkipMedia {
		present, _ := f.AssetRefs()
		if len(present) > 0 || len(f.State.Assets) > 0 {
			caps, err := media.Probe(ctx, c)
			if err != nil {
				return nil, err
			}
			synced, outcomes, err := media.Sync(ctx, c, f, caps, opts.DryRun)
			if err != nil {
				return nil, err
			}
			for k, v := range synced {
				urls[k] = v
			}
			res.Assets = outcomes
			res.MediaDir, _, _ = media.Folder(f)
			for _, o := range outcomes {
				if o.Err != "" {
					res.AssetError = true
				}
			}
			if !opts.DryRun {
				_ = f.SaveState()
			}
		}
	}

	payload, err := f.Build(urls)
	if err != nil {
		return nil, err
	}
	digests := payload.Digests()

	if owned {
		return pushOwned(ctx, c, f, payload, digests, opts, res)
	}
	return pushTrial(ctx, c, f, payload, digests, opts, res)
}

type trialResponse struct {
	ClientKey string            `json:"clientKey"`
	RoleID    string            `json:"roleId"`
	Created   bool              `json:"created"`
	ExpiresAt string            `json:"expiresAt"`
	Slots     map[string]any    `json:"slots"`
	Sections  map[string]string `json:"sections"`
	Changed   []string          `json:"changed"`
	Worldbook *struct {
		WorldbookID string `json:"worldbookId"`
	} `json:"worldbook"`
}

func pushTrial(ctx context.Context, c *api.Client, f *card.Folder, p card.Payload, digests map[string]string, opts PushOptions, res *PushResult) (*PushResult, error) {
	res.Target = "trial"
	if f.State.TrialKey == "" {
		key, err := trialKeyFor(f.Dir)
		if err != nil {
			return nil, err
		}
		f.State.TrialKey = key
	}
	res.TrialKey = f.State.TrialKey
	body := map[string]any{"name": p.Name}
	if opts.Evict {
		body["evict"] = true
	}
	// The trial document is whole-card: an omitted welcome, worldbook or
	// authorAsset section is cleared on the server, so every present section
	// is always sent and the server reports which ones actually changed by
	// content hash. Only `card` may be omitted, and there is no reason to.
	present := []string{}
	for _, s := range card.Sections {
		if sec := p.Section(s); sec != nil {
			body[s] = sec
			present = append(present, s)
		}
	}
	res.Changed = []string{}
	res.Unchanged = []string{}
	if opts.DryRun {
		for _, s := range present {
			if opts.Force || f.State.LocalHashes[s] != digests[s] || f.State.RoleID == "" {
				res.Changed = append(res.Changed, s)
			} else {
				res.Unchanged = append(res.Unchanged, s)
			}
		}
		res.RoleID = f.State.RoleID
		return res, nil
	}
	var tr trialResponse
	if err := c.OpenPutLanguage(ctx, "/trial-cards/"+url.PathEscape(f.State.TrialKey), f.Manifest.Language, body, &tr); err != nil {
		if api.IsStatus(err, 409) {
			return nil, fmt.Errorf("%w\nAll trial slots are in use; pass --evict to free the least recently used one", err)
		}
		return nil, err
	}
	f.State.Target = "trial"
	f.State.RoleID = tr.RoleID
	f.State.API = c.API
	f.State.Sections = tr.Sections
	if f.State.LocalHashes == nil {
		f.State.LocalHashes = map[string]string{}
	}
	for s, d := range digests {
		f.State.LocalHashes[s] = d
	}
	if tr.Worldbook != nil && tr.Worldbook.WorldbookID != "" {
		f.State.LorebookID = tr.Worldbook.WorldbookID
	}
	// The trial contract ignores image fields; the trial role is still an
	// owned private card, so its images go through the document route.
	images := map[string]any{}
	for _, k := range []string{"roleAvatar", "roleBackground", "roleBackgroundLandscape"} {
		if v, ok := p.Card[k]; ok {
			images[k] = v
		}
	}
	mediaDigest := card.Digest(images)
	if len(images) > 0 && (opts.Force || f.State.LocalHashes["media"] != mediaDigest) {
		if err := c.OpenPost(ctx, "/role/"+tr.RoleID+"/document", map[string]any{"fields": images}, nil); err != nil {
			_ = f.SaveState()
			return nil, fmt.Errorf("set card media: %w", err)
		}
		f.State.LocalHashes["media"] = mediaDigest
		tr.Changed = append(tr.Changed, "media")
	}
	if err := f.SaveState(); err != nil {
		return nil, err
	}
	res.RoleID = tr.RoleID
	res.Created = tr.Created
	res.ExpiresAt = tr.ExpiresAt
	res.Slots = tr.Slots
	res.Sections = tr.Sections
	changed := map[string]bool{}
	for _, s := range tr.Changed {
		changed[s] = true
	}
	res.Changed = tr.Changed
	if res.Changed == nil {
		res.Changed = []string{}
	}
	for _, s := range present {
		if !changed[s] {
			res.Unchanged = append(res.Unchanged, s)
		}
	}
	return res, nil
}

type roleOutput struct {
	RoleID string `json:"roleId"`
}

func pushOwned(ctx context.Context, c *api.Client, f *card.Folder, p card.Payload, digests map[string]string, opts PushOptions, res *PushResult) (*PushResult, error) {
	res.Target = "owned"
	roleID := f.State.RoleID
	if opts.To != "" {
		roleID = opts.To
	}
	if opts.DryRun {
		for _, s := range card.Sections {
			if p.Section(s) != nil {
				res.Changed = append(res.Changed, s)
			}
		}
		res.RoleID = roleID
		return res, nil
	}
	if opts.Create || roleID == "" {
		body := map[string]any{"roleName": p.Name, "origin": "hearthroom"}
		if f.Manifest.Language != "" {
			body["language"] = f.Manifest.Language
		}
		if f.Manifest.Type != "" {
			body["cardType"] = f.Manifest.Type
		}
		var out roleOutput
		if err := c.OpenPost(ctx, "/role", body, &out); err != nil {
			return nil, fmt.Errorf("create card: %w", err)
		}
		roleID = out.RoleID
		res.Created = true
		f.State.LorebookID = ""
		f.State.AuthorAssetVersion = 0
	}
	f.State.Target = "owned"
	f.State.RoleID = roleID
	f.State.API = c.API
	res.RoleID = roleID
	if f.State.LocalHashes == nil {
		f.State.LocalHashes = map[string]string{}
	}
	changed := func(s string) bool {
		return opts.Force || f.State.LocalHashes[s] != digests[s]
	}

	// card fields
	if changed(card.SectionCard) {
		if err := c.OpenPost(ctx, "/role/"+roleID+"/document", map[string]any{"fields": p.Card}, nil); err != nil {
			return nil, fmt.Errorf("write card fields: %w", err)
		}
		f.State.LocalHashes[card.SectionCard] = digests[card.SectionCard]
		res.Changed = append(res.Changed, card.SectionCard)
	} else {
		res.Unchanged = append(res.Unchanged, card.SectionCard)
	}
	// welcome
	if p.Welcome != nil {
		if changed(card.SectionWelcome) {
			if p.Welcome["roleWelcome"] == "" {
				res.Skipped = append(res.Skipped, card.SectionWelcome+" (welcome.md is empty; owned cards require an opening)")
			} else {
				if err := c.OpenPatch(ctx, "/role/"+roleID+"/welcome", p.Welcome, nil); err != nil {
					return nil, fmt.Errorf("write opening: %w", err)
				}
				f.State.LocalHashes[card.SectionWelcome] = digests[card.SectionWelcome]
				res.Changed = append(res.Changed, card.SectionWelcome)
			}
		} else {
			res.Unchanged = append(res.Unchanged, card.SectionWelcome)
		}
	}
	// lorebook
	if p.Worldbook != nil {
		if changed(card.SectionWorldbook) {
			if err := pushLorebook(ctx, c, f, roleID, p.Worldbook); err != nil {
				return nil, err
			}
			f.State.LocalHashes[card.SectionWorldbook] = digests[card.SectionWorldbook]
			res.Changed = append(res.Changed, card.SectionWorldbook)
		} else {
			res.Unchanged = append(res.Unchanged, card.SectionWorldbook)
		}
	}
	// display rules
	if p.AuthorAsset != nil {
		if changed(card.SectionAuthorAsset) {
			body := map[string]any{}
			for k, v := range p.AuthorAsset {
				body[k] = v
			}
			body["version"] = f.State.AuthorAssetVersion
			var out struct {
				Version int64 `json:"version"`
			}
			err := c.OpenPut(ctx, "/role/"+roleID+"/author-asset", body, &out)
			if api.IsStatus(err, 409) {
				// First save on a card that already has rules, or a stale version: read and retry once.
				var cur card.RemoteAsset
				if rerr := c.OpenGet(ctx, "/role/author-asset", url.Values{"roleId": {roleID}}, &cur); rerr == nil {
					body["version"] = cur.Version
					err = c.OpenPut(ctx, "/role/"+roleID+"/author-asset", body, &out)
				}
			}
			if err != nil {
				return nil, fmt.Errorf("write display rules: %w", err)
			}
			if out.Version != 0 {
				f.State.AuthorAssetVersion = out.Version
			} else {
				f.State.AuthorAssetVersion++
			}
			f.State.LocalHashes[card.SectionAuthorAsset] = digests[card.SectionAuthorAsset]
			res.Changed = append(res.Changed, card.SectionAuthorAsset)
		} else {
			res.Unchanged = append(res.Unchanged, card.SectionAuthorAsset)
		}
	}
	if res.Changed == nil {
		res.Changed = []string{}
	}
	if res.Unchanged == nil {
		res.Unchanged = []string{}
	}
	return res, f.SaveState()
}

type bindingsOutput struct {
	Bindings []struct {
		WorldbookID string `json:"worldbookId"`
		Name        string `json:"name"`
	} `json:"bindings"`
}

type entryListOutput struct {
	Entries []card.RemoteEntry `json:"entries"`
}

// pushLorebook writes the folder's Lorebook to the card's bound book,
// creating and binding one when none exists. Entries are matched by id when
// the folder knows them, otherwise by name. Only entries the CLI itself
// created or pulled (recorded in state) are ever deleted; entries written by
// other tools are left alone.
func pushLorebook(ctx context.Context, c *api.Client, f *card.Folder, roleID string, wb map[string]any) error {
	bookID := f.State.LorebookID
	if bookID == "" && f.Lorebook != nil {
		bookID = f.Lorebook.ID
	}
	if bookID == "" {
		var b bindingsOutput
		if err := c.OpenGet(ctx, "/worldbook/bindings", url.Values{"roleId": {roleID}}, &b); err == nil && len(b.Bindings) > 0 {
			bookID = b.Bindings[0].WorldbookID
		}
	}
	name, _ := wb["name"].(string)
	if name == "" {
		name = f.Manifest.Name
	}
	if bookID == "" {
		var out struct {
			WorldbookID string `json:"worldbookId"`
		}
		if err := c.OpenPost(ctx, "/worldbook", map[string]any{"name": name}, &out); err != nil {
			return fmt.Errorf("create Lorebook: %w", err)
		}
		if out.WorldbookID == "" {
			return errors.New("create Lorebook: no id in response")
		}
		bookID = out.WorldbookID
	}
	// Existing entries for matching.
	var existing entryListOutput
	if err := c.OpenGet(ctx, "/worldbook/entry/list", url.Values{"worldbookId": {bookID}}, &existing); err != nil {
		return fmt.Errorf("list Lorebook entries: %w", err)
	}
	byID := map[string]card.RemoteEntry{}
	byName := map[string]card.RemoteEntry{}
	for _, e := range existing.Entries {
		byID[e.EntryID] = e
		if _, dup := byName[e.Name]; !dup {
			byName[e.Name] = e
		}
	}
	var ops []map[string]any
	seen := map[string]bool{}
	local := []card.LorebookEntry{}
	if f.Lorebook != nil {
		local = f.Lorebook.Entries
	}
	entries, _ := wb["entries"].([]map[string]any)
	// Map payload entries back to local entries by position among enabled ones.
	pos := 0
	for i := range local {
		le := &local[i]
		if strings.TrimSpace(le.Content) == "" || le.Disabled {
			continue
		}
		if pos >= len(entries) {
			break
		}
		body := entries[pos]
		pos++
		op := map[string]any{}
		for k, v := range body {
			op[k] = v
		}
		target := ""
		if le.ID != "" {
			if _, ok := byID[le.ID]; ok {
				target = le.ID
			}
		}
		if target == "" {
			if e, ok := byName[le.Name]; ok && le.Name != "" && !seen[e.EntryID] {
				target = e.EntryID
			}
		}
		if target != "" {
			op["op"] = "update"
			op["entryId"] = target
			op["isEnabled"] = true
			seen[target] = true
			le.ID = target
		} else {
			op["op"] = "create"
		}
		ops = append(ops, op)
	}
	known := map[string]bool{}
	for _, id := range f.State.LorebookEntryIDs {
		known[id] = true
	}
	for _, le := range local {
		if le.ID != "" {
			known[le.ID] = true
		}
	}
	for _, e := range existing.Entries {
		if !seen[e.EntryID] && known[e.EntryID] {
			ops = append(ops, map[string]any{"op": "delete", "entryId": e.EntryID})
		}
	}
	doc := map[string]any{
		"metadata": map[string]any{"name": name},
		"binding":  map[string]any{"roleId": roleID, "targetType": "character", "targetId": roleID},
	}
	if len(ops) > 0 {
		doc["entries"] = ops
	}
	var out struct {
		Entries []struct {
			Op      string `json:"op"`
			EntryID string `json:"entryId"`
			Name    string `json:"name"`
		} `json:"entries"`
	}
	if err := c.OpenPost(ctx, "/worldbook/"+bookID+"/document", doc, &out); err != nil {
		return fmt.Errorf("write Lorebook: %w", err)
	}
	// Learn ids of created entries so the next push updates them.
	var created entryListOutput
	if err := c.OpenGet(ctx, "/worldbook/entry/list", url.Values{"worldbookId": {bookID}}, &created); err == nil {
		byName = map[string]card.RemoteEntry{}
		for _, e := range created.Entries {
			byName[e.Name] = e
		}
		for i := range local {
			if local[i].ID == "" {
				if e, ok := byName[local[i].Name]; ok {
					local[i].ID = e.EntryID
				}
			}
		}
	}
	f.State.LorebookID = bookID
	f.State.LorebookEntryIDs = nil
	for _, le := range local {
		if le.ID != "" {
			f.State.LorebookEntryIDs = append(f.State.LorebookEntryIDs, le.ID)
		}
	}
	if f.Lorebook != nil {
		f.Lorebook.ID = bookID
		_ = f.Save()
	}
	return nil
}
