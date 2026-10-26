// Package sync moves card folders to and from the provider.
package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	// To targets an owned card instead of a trial card.
	To string
	// Create makes a new unlisted card and targets it (implies owned).
	Create bool
	// Evict frees a trial slot when all are used.
	Evict bool
	// DryRun computes what would be sent without sending.
	DryRun bool
	// Force sends every section even if the hash is unchanged.
	Force bool
	// Prune permits deleting CLI-managed entries removed from the local file.
	Prune bool
	// SkipMedia leaves assets alone and keeps previously recorded URLs.
	SkipMedia bool
	// Concurrency is the number of parallel media uploads (0: the default).
	Concurrency int
	// Progress, when set, is told about every finished media upload.
	Progress func(media.Progress)
	// GroupProgress reports entries split before any writes.
	GroupProgress func(GroupedEntry)
	limits        pushLimits
	entryKeys     []string
}

// PushResult reports what happened.
type PushResult struct {
	PruneEntries   []string          `json:"pruneEntries,omitempty"`
	Grouped        []GroupedEntry    `json:"grouped,omitempty"`
	Target         string            `json:"target"`
	RoleID         string            `json:"roleId"`
	RoleVisibility string            `json:"roleVisibility,omitempty"`
	TrialKey       string            `json:"trialKey,omitempty"`
	Created        bool              `json:"created"`
	ExpiresAt      string            `json:"expiresAt,omitempty"`
	Changed        []string          `json:"changed"`
	Unchanged      []string          `json:"unchanged"`
	Skipped        []string          `json:"skipped,omitempty"`
	Assets         []media.Outcome   `json:"assets,omitempty"`
	MediaDir       string            `json:"mediaFolder,omitempty"`
	Slots          map[string]any    `json:"slots,omitempty"`
	Sections       map[string]string `json:"sections,omitempty"`
	DryRun         bool              `json:"dryRun,omitempty"`
	RulesTotal     *RuleTotalSize    `json:"rulesTotal,omitempty"`
	Rules          []RuleSize        `json:"rules,omitempty"`
	AssetError     bool              `json:"assetErrors,omitempty"`
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
	if opts.DryRun {
		clone := *f
		raw, err := json.Marshal(f.State)
		if err != nil {
			return nil, err
		}
		clone.State = card.State{}
		if err := json.Unmarshal(raw, &clone.State); err != nil {
			return nil, err
		}
		f = &clone
	}

	if opts.Create && opts.To != "" {
		return nil, errors.New("--create and --to cannot be combined")
	}
	if err := card.CheckLanguage(f.Manifest.Language); err != nil {
		return nil, fmt.Errorf("%s: %w", card.ManifestFile, err)
	}
	opts.limits.TrialMaxEntries = 1000
	owned := opts.Create || opts.To != "" || (f.State.Target == "owned" && f.State.RoleID != "")
	if opts.To != "" && f.State.RoleID != "" && f.State.RoleID != opts.To && f.State.Target == "owned" && !opts.Force {
		return nil, fmt.Errorf("this folder is linked to card %s; pass --to %s again with --force to relink", f.State.RoleID, opts.To)
	}

	if f.State.API != "" && f.State.API != c.API && f.State.RoleID != "" {
		return nil, errors.New("this folder is linked to a different provider; use a separate folder for another provider")
	}
	language := f.Manifest.Language
	target := opts.To
	if target == "" && (!opts.Create || f.State.OwnedPushPending) {
		target = f.State.RoleID
	}
	if target != "" {
		var remote card.RemoteDetail
		if err := c.OpenGet(ctx, "/role/detail", url.Values{"roleId": {target}}, &remote); err != nil && (owned || !api.IsStatus(err, 404)) {
			return nil, err
		}
		if remote.Language != "" {
			language = remote.Language
		}
	}
	// Check all URL-independent constraints even when a provider assigns media URLs only on upload.
	pre, err := f.Build(StoredURLs(f))
	if err != nil {
		return nil, err
	}
	if _, err := preflight(f, pre, language, false); err != nil {
		return nil, err
	}
	// Preflight happens before media or remote mutations.
	if f.Lorebook != nil {
		limits, err := readPushLimits(ctx, c)
		if err != nil {
			return nil, err
		}
		opts.limits = limits
		limit := limits.EntryContentMaxByLanguage[f.Manifest.Language]
		if limit <= 0 {
			return nil, errors.New("provider did not publish a content limit for this card language")
		}
		if owned && (!opts.Create || f.State.OwnedPushPending) {
			target := f.State.RoleID
			if opts.To != "" {
				target = opts.To
			}
			bookID := ""
			if target == f.State.RoleID {
				bookID = f.State.LorebookID
			}
			if target != "" {
				if n := limits.EntryContentMaxByLanguage[language]; n > 0 {
					limit = n
				}
				if bookID == "" {
					var bindings bindingsOutput
					if err := c.OpenGet(ctx, "/worldbook/bindings", url.Values{"roleId": {target}}, &bindings); err != nil {
						return nil, err
					}
					if len(bindings.Bindings) > 0 {
						bookID = bindings.Bindings[0].WorldbookID
					}
				}
			}
			if bookID != "" {
				var detail struct {
					EntryContentMax int `json:"entryContentMax"`
				}
				if err := c.OpenGet(ctx, "/worldbook/detail", url.Values{"worldbookId": {bookID}}, &detail); err != nil {
					return nil, err
				}
				if detail.EntryContentMax > 0 {
					limit = detail.EntryContentMax
				}
			}
		}
		opts.limits.entryMax = limit

	}

	// Media first: the payload needs the served URLs.
	urls := StoredURLs(f)
	if !opts.SkipMedia {
		present, _ := f.AssetRefs()
		if len(present) > 0 || len(f.State.Assets) > 0 {
			caps, err := media.Probe(ctx, c)
			if err != nil {
				return nil, err
			}
			planned, plannedOutcomes, err := media.Sync(ctx, c, f, caps, media.SyncOptions{DryRun: true})
			if err != nil {
				return nil, err
			}
			previewURLs := StoredURLs(f)
			for k, v := range planned {
				previewURLs[k] = v
			}
			exact := true
			for _, o := range plannedOutcomes {
				if o.Uploaded && o.URL == "" && textReferences(f, o.Path) {
					exact = false
					if opts.DryRun {
						return nil, fmt.Errorf("Exact rewritten sizes are unavailable because the provider assigns a URL for %s only after upload. Run push without --dry-run to upload media and check sizes before writing card content.", o.Path)
					}
				}
			}
			preview, err := f.Build(previewURLs)
			if err != nil {
				return nil, err
			}
			if _, err := preflight(f, preview, language, exact); err != nil {
				return nil, err
			}
			if preview.Worldbook != nil {
				preview.Worldbook, _, _ = prepareEntries(f, preview.Worldbook, opts.limits.entryMax)
			}
			if err := preflightPrepared(preview, owned, opts.limits.entryMax, opts.limits.TrialMaxEntries, opts.Evict); exact && err != nil {
				return nil, err
			}

			synced, outcomes, err := media.Sync(ctx, c, f, caps, media.SyncOptions{
				DryRun:      opts.DryRun,
				Concurrency: opts.Concurrency,
				Save:        f.SaveState,
				Progress:    opts.Progress,
			})
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
					if textReferences(f, o.Path) {
						return nil, fmt.Errorf("Exact rewritten sizes could not be checked because no media URL is available for %s. Resolve the media upload before pushing again. No card content was written.", o.Path)
					}
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
	res.Rules, err = preflight(f, payload, language, true)
	if err != nil {
		return nil, err
	}
	res.RulesTotal = ruleTotal(payload)
	digests := payload.Digests()
	if payload.Worldbook != nil {
		payload.Worldbook, opts.entryKeys, res.Grouped = prepareEntries(f, payload.Worldbook, opts.limits.entryMax)
	}

	if err := preflightPrepared(payload, owned, opts.limits.entryMax, opts.limits.TrialMaxEntries, opts.Evict); err != nil {
		return nil, err
	}
	for _, g := range res.Grouped {
		if opts.GroupProgress != nil {
			opts.GroupProgress(g)
		}
	}
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
		if !opts.DryRun {
			if err := f.SaveState(); err != nil {
				return nil, err
			}
		}
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
	for _, k := range []string{"roleAvatar", "roleBackground", "roleBackgroundLandscape", "roleShareImage"} {
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
	Visibility string `json:"roleVisibility"`
	RoleID     string `json:"roleId"`
}

func pushOwned(ctx context.Context, c *api.Client, f *card.Folder, p card.Payload, digests map[string]string, opts PushOptions, res *PushResult) (result *PushResult, pushErr error) {
	rulesSaved := false
	defer func() {
		if rulesSaved && pushErr != nil {
			pushErr = fmt.Errorf("Display rules were saved, but the push is incomplete: %w", pushErr)
		}
	}()
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
		newTarget := roleID != f.State.RoleID || f.State.Target != "owned" || (opts.Create && !f.State.OwnedPushPending)
		if p.Worldbook != nil && (newTarget || opts.Force || f.State.LocalHashes[card.SectionWorldbook] != digests[card.SectionWorldbook]) {
			var err error
			res.PruneEntries, err = previewLorebook(ctx, c, f, roleID, p.Worldbook, opts)
			if err != nil {
				return nil, err
			}
		}
		return res, nil
	}
	if (opts.Create && !f.State.OwnedPushPending) || roleID == "" {
		body := map[string]any{"roleName": p.Name, "origin": "hearthroom", "visibility": "unlisted"}
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
		if roleID == "" {
			return nil, errors.New("create card: no id in response")
		}
		res.Created = true
		res.RoleVisibility = out.Visibility
		// Persist identity before follow-up work, including compatibility visibility.
		f.State.RoleID, f.State.Target, f.State.API = roleID, "owned", c.API
		f.State.LocalHashes = map[string]string{}
		f.State.OwnedPushPending = true
		f.State.PrivatePending = false
		f.State.LorebookID = ""
		f.State.IgnoreLocalLorebook = true
		f.State.LorebookEntryIDs = nil
		f.State.LorebookEntryMap = nil
		f.State.LorebookUpload = nil
		if f.Lorebook != nil {
			f.Lorebook.ID = ""
			for i := range f.Lorebook.Entries {
				f.Lorebook.Entries[i].ID = ""
			}
		}
		f.State.AuthorAssetVersion = 0
		if err := f.SaveState(); err != nil {
			return nil, err
		}

	}
	if roleID != f.State.RoleID || f.State.Target != "owned" {
		// The section hashes describe what the previous target holds (a trial
		// card, or another owned card). Against a new target they would make
		// every section look unchanged and leave the card empty.
		f.State.LocalHashes = map[string]string{}
		f.State.LorebookID = ""
		f.State.IgnoreLocalLorebook = true
		f.State.LorebookEntryIDs = nil
		f.State.LorebookEntryMap = nil
		f.State.LorebookUpload = nil
		f.State.AuthorAssetVersion = 0
		f.State.PrivatePending = false
		if f.Lorebook != nil {
			f.Lorebook.ID = ""
			for i := range f.Lorebook.Entries {
				f.Lorebook.Entries[i].ID = ""
			}
		}
	}
	// Older clients recorded a compatibility request to make the created card
	// private. Resuming content must preserve the author's current visibility;
	// never replay that stale implicit request over a later explicit choice.
	f.State.PrivatePending = false

	f.State.Target = "owned"
	f.State.RoleID = roleID
	f.State.API = c.API
	res.RoleID = roleID
	f.State.OwnedPushPending = true
	if err := f.SaveState(); err != nil {
		return nil, err
	}
	if f.State.LocalHashes == nil {
		f.State.LocalHashes = map[string]string{}
	}
	changed := func(s string) bool {
		return opts.Force || f.State.LocalHashes[s] != digests[s]
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
				return nil, fmt.Errorf("Display rules were not confirmed as saved. This push stopped before writing configuration text, openings, or Lorebook entries. Check the saved rules before pushing again: %w", err)
			}
			if out.Version != 0 {
				f.State.AuthorAssetVersion = out.Version
			} else {
				f.State.AuthorAssetVersion++
			}
			f.State.LocalHashes[card.SectionAuthorAsset] = digests[card.SectionAuthorAsset]
			// Persist accepted rules before another section can fail. A retry must not resend them.
			if err := f.SaveState(); err != nil {
				return nil, err
			}
			rulesSaved = true
			res.Changed = append(res.Changed, card.SectionAuthorAsset)
		} else {
			res.Unchanged = append(res.Unchanged, card.SectionAuthorAsset)
		}
	}
	// card fields. A Chinese card can switch between zh-Hant and zh-Hans after
	// creation, so its language rides with the card fields and counts toward
	// their digest; other languages are fixed and never sent.
	cardFields, cardDigest := p.Card, OwnedDigest(f, digests, card.SectionCard)
	if lang, ok := switchableScript(f); ok {
		cardFields = make(map[string]any, len(p.Card)+1)
		for k, v := range p.Card {
			cardFields[k] = v
		}
		cardFields["language"] = lang
	}
	if opts.Force || f.State.LocalHashes[card.SectionCard] != cardDigest {
		if err := c.OpenPost(ctx, "/role/"+roleID+"/document", map[string]any{"fields": cardFields}, nil); err != nil {
			return nil, fmt.Errorf("write card fields: %w", err)
		}
		f.State.LocalHashes[card.SectionCard] = cardDigest
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
			if err := pushLorebook(ctx, c, f, roleID, p.Worldbook, opts); err != nil {
				return nil, err
			}
			f.State.LocalHashes[card.SectionWorldbook] = digests[card.SectionWorldbook]
			res.Changed = append(res.Changed, card.SectionWorldbook)
		} else {
			res.Unchanged = append(res.Unchanged, card.SectionWorldbook)
		}
	}

	if res.Changed == nil {
		res.Changed = []string{}
	}
	if res.Unchanged == nil {
		res.Unchanged = []string{}
	}
	f.State.OwnedPushPending = false
	f.State.IgnoreLocalLorebook = false
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
// switchableScript is the folder's language when it is one of the two Chinese
// scripts an owned card may switch between.
func switchableScript(f *card.Folder) (string, bool) {
	lang := f.Manifest.Language
	return lang, lang == "zh-Hant" || lang == "zh-Hans"
}

// StoredURLs is the asset URL map the last push built the card with: the
// served URL of every uploaded file plus each referenced directory's URL.
// Comparing local sections against the last push must build with this, not
// the file URLs alone.
func StoredURLs(f *card.Folder) map[string]string {
	urls := map[string]string{}
	for p, a := range f.State.Assets {
		if a.URL != "" {
			urls[p] = a.URL
		}
	}
	media.AddDirURLs(f, urls)
	return urls
}

// OwnedDigest is the stored hash a section is compared with. For an owned
// Chinese card the card section also covers its language, so a script switch
// alone counts as a change (push sends it; status reports it).
func OwnedDigest(f *card.Folder, digests map[string]string, section string) string {
	d := digests[section]
	if lang, ok := switchableScript(f); ok && section == card.SectionCard && f.State.Target == "owned" {
		d += "|language=" + lang
	}
	return d
}
