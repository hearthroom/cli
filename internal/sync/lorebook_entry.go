package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

// EntryUsageError marks invalid caller input or folder state (exit code 2).
type EntryUsageError struct {
	Msg string
}

func (e *EntryUsageError) Error() string { return e.Msg }

func entryUsagef(format string, args ...any) error {
	return &EntryUsageError{Msg: fmt.Sprintf(format, args...)}
}

// EntryPatch carries the fields to set in a single-entry create or update.
// Nil fields are omitted on update so the server keeps stored values.
type EntryPatch struct {
	Name              *string
	Content           *string
	Keywords          *[]string
	SecondaryKeywords *[]string
	Category          *string
	TriggerRegion     *string
	Constant          *bool
	Disabled          *bool
	SemanticRecall    *bool
	MatchOptions      *json.RawMessage
	// ClearMatchOptions sends an empty matchOptions object when a replacement
	// file omits options that were previously set on the remote entry.
	ClearMatchOptions bool
}

func (p EntryPatch) empty() bool {
	return p.Name == nil &&
		p.Content == nil &&
		p.Keywords == nil &&
		p.SecondaryKeywords == nil &&
		p.Category == nil &&
		p.TriggerRegion == nil &&
		p.Constant == nil &&
		p.Disabled == nil &&
		p.SemanticRecall == nil &&
		p.MatchOptions == nil &&
		!p.ClearMatchOptions
}

// EntryWriteOptions controls optimistic concurrency for single-entry writes.
type EntryWriteOptions struct {
	ExpectedVersion int32
	Force           bool
}

// EntryListResult is returned by ListLorebookEntries.
type EntryListResult struct {
	RoleID     string               `json:"roleId"`
	LorebookID string               `json:"lorebookId,omitempty"`
	Name       string               `json:"name,omitempty"`
	Version    int32                `json:"version"`
	Entries    []card.LorebookEntry `json:"entries"`
}

// EntryGetResult is returned by GetLorebookEntry.
type EntryGetResult struct {
	RoleID     string             `json:"roleId"`
	LorebookID string             `json:"lorebookId"`
	Name       string             `json:"name,omitempty"`
	Version    int32              `json:"version"`
	Entry      card.LorebookEntry `json:"entry"`
}

// EntryWriteResult is returned by AddLorebookEntry, UpdateLorebookEntry, and DeleteLorebookEntry.
type EntryWriteResult struct {
	Op             string              `json:"op"`
	RoleID         string              `json:"roleId"`
	LorebookID     string              `json:"lorebookId"`
	Version        int32               `json:"version"`
	Entry          *card.LorebookEntry `json:"entry,omitempty"`
	DeletedEntryID string              `json:"deletedEntryId,omitempty"`
	DeletedName    string              `json:"deletedName,omitempty"`
	Warnings       []string            `json:"warnings,omitempty"`
}

type resolvedBook struct {
	roleID  string
	bookID  string
	name    string
	detail  worldbookDetailOutput
	entries []card.RemoteEntry
	hasBook bool
}

func validateOwnedEntryFolder(c *api.Client, f *card.Folder) (string, error) {
	if f.State.Target != "owned" || strings.TrimSpace(f.State.RoleID) == "" {
		return "", entryUsagef("lorebook entry commands need a folder linked to an owned card; run 'hearthroom card push %s --create' or 'hearthroom card pull <roleId> %s' first", f.Dir, f.Dir)
	}
	if f.State.API != "" && f.State.API != c.API {
		return "", entryUsagef("this folder is linked to a different provider")
	}
	return f.State.RoleID, nil
}

func resolveBoundLorebook(ctx context.Context, c *api.Client, f *card.Folder) (*resolvedBook, error) {
	roleID, err := validateOwnedEntryFolder(c, f)
	if err != nil {
		return nil, err
	}
	var bindings bindingsOutput
	if err := c.OpenGet(ctx, "/worldbook/bindings", url.Values{"roleId": {roleID}}, &bindings); err != nil {
		return nil, fmt.Errorf("read Lorebook bindings: %w", err)
	}
	boundID, boundName, hasBound := latestBinding(bindings)
	if f.State.LorebookID != "" && boundID != f.State.LorebookID {
		if !hasBound {
			return nil, fmt.Errorf("card %s no longer has Lorebook %s bound on the server; run 'hearthroom card pull %s %s --force' to sync remote changes", roleID, f.State.LorebookID, roleID, f.Dir)
		}
		return nil, fmt.Errorf("card %s is bound to Lorebook %s on the server, not %s recorded in local state; run 'hearthroom card pull %s %s --force' to sync the bound Lorebook", roleID, boundID, f.State.LorebookID, roleID, f.Dir)
	}
	if !hasBound {
		return &resolvedBook{roleID: roleID, hasBook: false}, nil
	}
	var detail worldbookDetailOutput
	if err := c.OpenGet(ctx, "/worldbook/detail", url.Values{"worldbookId": {boundID}}, &detail); err != nil {
		return nil, fmt.Errorf("read Lorebook detail: %w", err)
	}
	if detail.Name != "" {
		boundName = detail.Name
	}
	var list entryListOutput
	if err := c.OpenGet(ctx, "/worldbook/entry/list", url.Values{"worldbookId": {boundID}}, &list); err != nil {
		return nil, fmt.Errorf("list Lorebook entries: %w", err)
	}
	return &resolvedBook{
		roleID:  roleID,
		bookID:  boundID,
		name:    boundName,
		detail:  detail,
		entries: list.Entries,
		hasBook: true,
	}, nil
}

func resolveRemoteEntry(entries []card.RemoteEntry, ref, dir string) (card.RemoteEntry, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return card.RemoteEntry{}, entryUsagef("entry id or name is required")
	}
	for _, e := range entries {
		if e.EntryID == ref {
			return e, nil
		}
	}
	var matches []card.RemoteEntry
	for _, e := range entries {
		if e.Name == ref {
			matches = append(matches, e)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return card.RemoteEntry{}, fmt.Errorf("Lorebook entry %q not found; run 'hearthroom lorebook entry list %s' to see entry ids and names", ref, dir)
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.EntryID
		}
		return card.RemoteEntry{}, entryUsagef("multiple Lorebook entries are named %q (%s); pass the entry id instead", ref, strings.Join(ids, ", "))
	}
}

// ListLorebookEntries reads the live entries of the card's bound Lorebook.
func ListLorebookEntries(ctx context.Context, c *api.Client, f *card.Folder) (*EntryListResult, error) {
	rb, err := resolveBoundLorebook(ctx, c, f)
	if err != nil {
		return nil, err
	}
	if !rb.hasBook {
		return &EntryListResult{
			RoleID:  rb.roleID,
			Version: 0,
			Entries: []card.LorebookEntry{},
		}, nil
	}
	out := make([]card.LorebookEntry, 0, len(rb.entries))
	for _, e := range rb.entries {
		out = append(out, card.RemoteToLorebookEntry(e))
	}
	return &EntryListResult{
		RoleID:     rb.roleID,
		LorebookID: rb.bookID,
		Name:       rb.name,
		Version:    rb.detail.Version,
		Entries:    out,
	}, nil
}

// GetLorebookEntry reads one live Lorebook entry by entryId or unique name.
func GetLorebookEntry(ctx context.Context, c *api.Client, f *card.Folder, entryRef string) (*EntryGetResult, error) {
	rb, err := resolveBoundLorebook(ctx, c, f)
	if err != nil {
		return nil, err
	}
	if !rb.hasBook {
		return nil, fmt.Errorf("card %s has no bound Lorebook; run 'hearthroom lorebook entry add %s ...' or 'hearthroom card push %s' to create one", rb.roleID, f.Dir, f.Dir)
	}
	target, err := resolveRemoteEntry(rb.entries, entryRef, f.Dir)
	if err != nil {
		return nil, err
	}
	return &EntryGetResult{
		RoleID:     rb.roleID,
		LorebookID: rb.bookID,
		Name:       rb.name,
		Version:    rb.detail.Version,
		Entry:      card.RemoteToLorebookEntry(target),
	}, nil
}

func checkWritePreconditions(f *card.Folder, opts EntryWriteOptions) error {
	if opts.Force && opts.ExpectedVersion > 0 {
		return entryUsagef("--force and --expected-version cannot be combined")
	}
	if opts.ExpectedVersion < 0 {
		return entryUsagef("--expected-version must be a positive version number")
	}
	if f.State.LorebookUpload != nil && f.State.LorebookUpload.InFlight {
		return fmt.Errorf("an interrupted Lorebook batch push is saved in state; rerun 'hearthroom card push %s' or 'hearthroom card pull %s %s --force' before editing individual entries", f.Dir, f.State.RoleID, f.Dir)
	}
	return nil
}

func resolveEntryLimit(ctx context.Context, c *api.Client, f *card.Folder, detailMax int) (int, error) {
	if detailMax > 0 {
		return detailMax, nil
	}
	limits, err := readPushLimits(ctx, c)
	if err != nil {
		return 0, err
	}
	lang := f.Manifest.Language
	if n := limits.EntryContentMaxByLanguage[lang]; n > 0 {
		return n, nil
	}
	return 4000, nil
}

func buildAndValidateEntryOp(f *card.Folder, opKind string, entryID string, patch EntryPatch, entryMax int) (map[string]any, string, error) {
	if opKind == "create" {
		if patch.Name == nil || strings.TrimSpace(*patch.Name) == "" {
			return nil, "", entryUsagef("entry name is required; pass --name or --file")
		}
		if patch.Content == nil || strings.TrimSpace(*patch.Content) == "" {
			return nil, "", entryUsagef("entry content is required; pass --content, --content-file, or --file")
		}
	} else if patch.empty() {
		return nil, "", entryUsagef("nothing to update; pass --file or at least one entry field flag")
	}

	op := map[string]any{"op": opKind}
	if entryID != "" {
		op["entryId"] = entryID
	}

	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" {
			return nil, "", entryUsagef("entry name cannot be empty")
		}
		if n := utf8.RuneCountInString(name); n > 120 {
			return nil, "", entryUsagef("entry name contains %d characters; the limit is 120 characters", n)
		}
		op["name"] = name
	}

	localContent := ""
	if patch.Content != nil {
		raw := *patch.Content
		if strings.TrimSpace(raw) == "" {
			return nil, "", entryUsagef("entry content cannot be empty")
		}
		localContent = raw
		urls := StoredURLs(f)
		for _, ref := range card.ScanAssetRefs(raw) {
			if _, ok := urls[ref]; !ok {
				return nil, "", entryUsagef("entry content references %s which has not been uploaded yet; run 'hearthroom card push %s' to upload media first", ref, f.Dir)
			}
		}
		rewritten := card.Rewrite(raw, urls)
		if n := utf8.RuneCountInString(rewritten); entryMax > 0 && n > entryMax {
			return nil, "", entryUsagef("entry contains %d characters; the per-entry limit is %d characters", n, entryMax)
		}
		op["content"] = rewritten
	}

	if patch.Keywords != nil {
		kw := *patch.Keywords
		if kw == nil {
			kw = []string{}
		}
		op["keywords"] = kw
	} else if opKind == "create" {
		op["keywords"] = []string{}
	}

	if patch.SecondaryKeywords != nil {
		skw := *patch.SecondaryKeywords
		if skw == nil {
			skw = []string{}
		}
		op["secondaryKeywords"] = skw
	}

	if patch.Category != nil {
		cat := strings.TrimSpace(*patch.Category)
		switch cat {
		case "rule", "character", "location", "item", "event", "custom":
			op["category"] = cat
		case "":
			if opKind == "update" {
				return nil, "", entryUsagef("category must be rule, character, location, item, event, or custom")
			}
		default:
			return nil, "", entryUsagef("category %q is unsupported; use rule, character, location, item, event, or custom", cat)
		}
	}

	if patch.TriggerRegion != nil {
		tr := strings.TrimSpace(*patch.TriggerRegion)
		switch tr {
		case "both", "user_only", "ai_only":
			op["triggerRegion"] = tr
		case "":
			if opKind == "update" {
				return nil, "", entryUsagef("trigger-region must be both, user_only, or ai_only")
			}
		default:
			return nil, "", entryUsagef("trigger-region %q is unsupported; use both, user_only, or ai_only", tr)
		}
	}

	if patch.Disabled != nil {
		op["isEnabled"] = !*patch.Disabled
	} else if opKind == "create" {
		op["isEnabled"] = true
	}

	if patch.Constant != nil {
		op["isConstant"] = *patch.Constant
	}

	if patch.SemanticRecall != nil {
		op["semanticRecall"] = *patch.SemanticRecall
	}

	if patch.MatchOptions != nil && len(*patch.MatchOptions) > 0 && string(*patch.MatchOptions) != "null" {
		var parsed struct {
			Selective       *bool                      `json:"selective"`
			ScanDepth       int                        `json:"scanDepth"`
			GroupID         string                     `json:"groupId"`
			GroupOrder      int                        `json:"groupOrder"`
			Order           *int                       `json:"order"`
			Extensions      map[string]json.RawMessage `json:"extensions"`
			CaseSensitive   bool                       `json:"caseSensitive"`
			MatchWholeWords bool                       `json:"matchWholeWords"`
			SelectiveLogic  int                        `json:"selectiveLogic"`
		}
		var generic any
		if json.Unmarshal(*patch.MatchOptions, &parsed) != nil || json.Unmarshal(*patch.MatchOptions, &generic) != nil {
			return nil, "", entryUsagef("match-options must be a valid JSON object")
		}
		if _, isObj := generic.(map[string]any); !isObj {
			return nil, "", entryUsagef("match-options must be a JSON object")
		}
		if parsed.ScanDepth < 0 || parsed.ScanDepth > 100 || len(parsed.GroupID) > 128 || parsed.GroupOrder < 0 || parsed.GroupOrder > 10000 || parsed.SelectiveLogic < 0 || parsed.SelectiveLogic > 3 {
			return nil, "", entryUsagef("match-options is out of range (scanDepth 0-100, groupId up to 128 bytes, groupOrder 0-10000, selectiveLogic 0-3)")
		}
		ext, _ := json.Marshal(parsed.Extensions)
		if len(ext) > 32768 {
			return nil, "", entryUsagef("match-options.extensions exceeds 32768 bytes")
		}
		op["matchOptions"] = generic
	} else if patch.ClearMatchOptions {
		op["matchOptions"] = map[string]any{}
	}

	return op, localContent, nil
}

func effectiveExpectedVersion(f *card.Folder, remoteVersion int32, opts EntryWriteOptions) (int32, []string) {
	if opts.Force {
		var warnings []string
		if f.State.LorebookVersion > 0 && remoteVersion > 0 && remoteVersion != f.State.LorebookVersion {
			warnings = append(warnings, fmt.Sprintf("--force writes without version conflict check (remote version %d, local base version %d)", remoteVersion, f.State.LorebookVersion))
		}
		return 0, warnings
	}
	if opts.ExpectedVersion > 0 {
		return opts.ExpectedVersion, nil
	}
	if f.State.LorebookVersion > 0 {
		return f.State.LorebookVersion, nil
	}
	return remoteVersion, nil
}

func formatEntryConflictError(err error, opKind, roleID, dir, entryRef string, expectedVer, fallbackRemote int32) error {
	remoteVer := fallbackRemote
	var ae *api.Error
	if errors.As(err, &ae) {
		if detail, ok := ae.Detail.(map[string]any); ok {
			if cv, ok := detail["currentVersion"].(float64); ok && int32(cv) > 0 {
				remoteVer = int32(cv)
			}
		}
	}
	wrap := func(msg string) error {
		if err != nil {
			return conflictError{msg: msg, err: err}
		}
		return errors.New(msg)
	}
	actionVerb := "edit"
	if opKind == "delete" {
		actionVerb = "delete"
	}
	if entryRef != "" {
		if remoteVer > 0 {
			return wrap(fmt.Sprintf("remote Lorebook is at version %d (expected %d); run 'hearthroom lorebook entry get %s %q' to inspect the live entry and retry with '--expected-version %d' to %s only this entry, or run 'hearthroom card pull %s %s --force' to sync the whole Lorebook", remoteVer, expectedVer, dir, entryRef, remoteVer, actionVerb, roleID, dir))
		}
		return fmt.Errorf("remote Lorebook changed since version %d (%w); run 'hearthroom lorebook entry get %s %q' to inspect the live entry or 'hearthroom card pull %s %s --force' to sync the whole Lorebook", expectedVer, err, dir, entryRef, roleID, dir)
	}
	if remoteVer > 0 {
		return wrap(fmt.Sprintf("remote Lorebook is at version %d (expected %d); run 'hearthroom lorebook entry list %s' to inspect live entries and retry with '--expected-version %d' to add only this entry, or run 'hearthroom card pull %s %s --force' to sync the whole Lorebook", remoteVer, expectedVer, dir, remoteVer, roleID, dir))
	}
	return fmt.Errorf("remote Lorebook changed since version %d (%w); run 'hearthroom lorebook entry list %s' or 'hearthroom card pull %s %s --force' to sync the whole Lorebook", expectedVer, err, dir, roleID, dir)
}

// AddLorebookEntry appends one entry to the owned card's bound Lorebook (creating and binding one if absent),
// then updates local lorebook.json and .hearthroom/state.json.
func AddLorebookEntry(ctx context.Context, c *api.Client, f *card.Folder, patch EntryPatch, opts EntryWriteOptions) (*EntryWriteResult, error) {
	if err := checkWritePreconditions(f, opts); err != nil {
		return nil, err
	}
	rb, err := resolveBoundLorebook(ctx, c, f)
	if err != nil {
		return nil, err
	}
	entryMax, err := resolveEntryLimit(ctx, c, f, rb.detail.EntryContentMax)
	if err != nil {
		return nil, err
	}
	op, localContent, err := buildAndValidateEntryOp(f, "create", "", patch, entryMax)
	if err != nil {
		return nil, err
	}

	bookID := rb.bookID
	bookName := rb.name
	remoteVersion := rb.detail.Version
	createdBook := false
	if !rb.hasBook {
		if f.Lorebook != nil && strings.TrimSpace(f.Lorebook.Name) != "" {
			bookName = strings.TrimSpace(f.Lorebook.Name)
		} else {
			bookName = strings.TrimSpace(f.Manifest.Name)
		}
		var out struct {
			WorldbookID string `json:"worldbookId"`
			Version     int32  `json:"version"`
		}
		if err := c.OpenPost(ctx, "/worldbook", map[string]any{"name": bookName, "language": f.Manifest.Language}, &out); err != nil {
			return nil, fmt.Errorf("create Lorebook: %w", err)
		}
		if out.WorldbookID == "" {
			return nil, errors.New("create Lorebook: no id in response")
		}
		bookID = out.WorldbookID
		remoteVersion = out.Version
		createdBook = true
	}

	var expectedVer int32
	var warnings []string
	if createdBook {
		if !opts.Force && opts.ExpectedVersion > 0 {
			expectedVer = opts.ExpectedVersion
		} else if !opts.Force && remoteVersion > 0 {
			expectedVer = remoteVersion
		}
	} else {
		expectedVer, warnings = effectiveExpectedVersion(f, remoteVersion, opts)
	}

	doc := map[string]any{"entries": []map[string]any{op}}
	if createdBook {
		doc["binding"] = map[string]any{"roleId": rb.roleID}
	}
	if expectedVer > 0 {
		doc["expectedVersion"] = expectedVer
	}

	var docRes struct {
		CreatedEntryIDs []string `json:"createdEntryIds"`
		Version         int32    `json:"version"`
	}
	if err := c.OpenPost(ctx, "/worldbook/"+bookID+"/document", doc, &docRes); err != nil {
		if createdBook {
			_ = c.OpenDelete(ctx, "/worldbook/"+bookID, nil)
		}
		if api.IsStatus(err, 409) {
			return nil, formatEntryConflictError(err, "add", rb.roleID, f.Dir, "", expectedVer, remoteVersion)
		}
		return nil, fmt.Errorf("add Lorebook entry: %w", err)
	}
	if len(docRes.CreatedEntryIDs) != 1 {
		return nil, fmt.Errorf("add Lorebook entry: expected 1 created entry id, got %d", len(docRes.CreatedEntryIDs))
	}
	createdID := docRes.CreatedEntryIDs[0]

	var afterList entryListOutput
	if err := c.OpenGet(ctx, "/worldbook/entry/list", url.Values{"worldbookId": {bookID}}, &afterList); err != nil {
		return nil, fmt.Errorf("read created Lorebook entry: %w", err)
	}
	remoteSaved, err := resolveRemoteEntry(afterList.Entries, createdID, f.Dir)
	if err != nil {
		return nil, err
	}
	localEntry := card.RemoteToLorebookEntry(remoteSaved)
	if localContent != "" && card.Rewrite(localContent, StoredURLs(f)) == localEntry.Content {
		localEntry.Content = localContent
	}

	if err := syncLocalEntryState(c, f, bookID, bookName, rb.detail, createdBook, docRes.Version, "add", createdID, "", &localEntry, afterList.Entries, entryMax); err != nil {
		return nil, err
	}
	if hasWorldbookMarkdownSources(f.Dir) {
		warnings = append(warnings, "worldbook/*.md source files were not modified (only lorebook.json was updated); update worldbook/*.md before running 'hearthroom lorebook build'")
	}
	return &EntryWriteResult{
		Op:         "add",
		RoleID:     rb.roleID,
		LorebookID: bookID,
		Version:    docRes.Version,
		Entry:      &localEntry,
		Warnings:   warnings,
	}, nil
}

// UpdateLorebookEntry patches a single entry in the owned card's bound Lorebook,
// leaving all other entries and book-level metadata untouched.
func UpdateLorebookEntry(ctx context.Context, c *api.Client, f *card.Folder, entryRef string, patch EntryPatch, opts EntryWriteOptions) (*EntryWriteResult, error) {
	if err := checkWritePreconditions(f, opts); err != nil {
		return nil, err
	}
	rb, err := resolveBoundLorebook(ctx, c, f)
	if err != nil {
		return nil, err
	}
	if !rb.hasBook {
		return nil, fmt.Errorf("card %s has no bound Lorebook; run 'hearthroom lorebook entry add %s ...' or 'hearthroom card push %s' to create one", rb.roleID, f.Dir, f.Dir)
	}
	target, err := resolveRemoteEntry(rb.entries, entryRef, f.Dir)
	if err != nil {
		return nil, err
	}
	if len(fragmentIDs(f.State.LorebookEntryMap, map[string]bool{target.EntryID: true})) > 0 {
		return nil, entryUsagef("Lorebook entry %q (%s) is split across multiple upload parts; edit it locally and run 'hearthroom card push %s'", target.Name, target.EntryID, f.Dir)
	}
	if patch.ClearMatchOptions && len(card.RemoteToLorebookEntry(target).MatchOptions) == 0 {
		patch.ClearMatchOptions = false
	}

	entryMax, err := resolveEntryLimit(ctx, c, f, rb.detail.EntryContentMax)
	if err != nil {
		return nil, err
	}
	op, localContent, err := buildAndValidateEntryOp(f, "update", target.EntryID, patch, entryMax)
	if err != nil {
		return nil, err
	}

	expectedVer, warnings := effectiveExpectedVersion(f, rb.detail.Version, opts)
	var afterEntries []card.RemoteEntry
	resultVersion := rb.detail.Version
	if len(op) <= 2 {
		if !opts.Force && expectedVer > 0 && rb.detail.Version > 0 && expectedVer != rb.detail.Version {
			return nil, formatEntryConflictError(nil, "update", rb.roleID, f.Dir, entryRef, expectedVer, rb.detail.Version)
		}
		afterEntries = rb.entries
	} else {
		doc := map[string]any{"entries": []map[string]any{op}}
		if expectedVer > 0 {
			doc["expectedVersion"] = expectedVer
		}

		var docRes struct {
			Version int32 `json:"version"`
		}
		if err := c.OpenPost(ctx, "/worldbook/"+rb.bookID+"/document", doc, &docRes); err != nil {
			if api.IsStatus(err, 409) {
				return nil, formatEntryConflictError(err, "update", rb.roleID, f.Dir, entryRef, expectedVer, rb.detail.Version)
			}
			return nil, fmt.Errorf("update Lorebook entry: %w", err)
		}
		var afterList entryListOutput
		if err := c.OpenGet(ctx, "/worldbook/entry/list", url.Values{"worldbookId": {rb.bookID}}, &afterList); err != nil {
			return nil, fmt.Errorf("read updated Lorebook entry: %w", err)
		}
		afterEntries = afterList.Entries
		resultVersion = docRes.Version
	}

	remoteSaved, err := resolveRemoteEntry(afterEntries, target.EntryID, f.Dir)
	if err != nil {
		return nil, err
	}
	localEntry := card.RemoteToLorebookEntry(remoteSaved)
	if localContent != "" && card.Rewrite(localContent, StoredURLs(f)) == localEntry.Content {
		localEntry.Content = localContent
	} else if localContent == "" && f.Lorebook != nil {
		if idx := findLocalEntryIndexWithIDs(f, resolvedLocalEntryIDs(f), target.EntryID, target.Name); idx >= 0 {
			prevContent := f.Lorebook.Entries[idx].Content
			if prevContent != "" && card.Rewrite(prevContent, StoredURLs(f)) == localEntry.Content {
				localEntry.Content = prevContent
			}
		}
	}

	if err := syncLocalEntryState(c, f, rb.bookID, rb.name, rb.detail, false, resultVersion, "update", target.EntryID, target.Name, &localEntry, afterEntries, entryMax); err != nil {
		return nil, err
	}
	if hasWorldbookMarkdownSources(f.Dir) {
		warnings = append(warnings, "worldbook/*.md source files were not modified (only lorebook.json was updated); update worldbook/*.md before running 'hearthroom lorebook build'")
	}
	return &EntryWriteResult{
		Op:         "update",
		RoleID:     rb.roleID,
		LorebookID: rb.bookID,
		Version:    resultVersion,
		Entry:      &localEntry,
		Warnings:   warnings,
	}, nil
}

// DeleteLorebookEntry removes one entry from the owned card's bound Lorebook
// and removes its local copy and tracking metadata from lorebook.json and state.json.
func DeleteLorebookEntry(ctx context.Context, c *api.Client, f *card.Folder, entryRef string, opts EntryWriteOptions) (*EntryWriteResult, error) {
	if err := checkWritePreconditions(f, opts); err != nil {
		return nil, err
	}
	rb, err := resolveBoundLorebook(ctx, c, f)
	if err != nil {
		return nil, err
	}
	if !rb.hasBook {
		return nil, fmt.Errorf("card %s has no bound Lorebook", rb.roleID)
	}
	target, err := resolveRemoteEntry(rb.entries, entryRef, f.Dir)
	if err != nil {
		return nil, err
	}
	if len(fragmentIDs(f.State.LorebookEntryMap, map[string]bool{target.EntryID: true})) > 0 {
		return nil, entryUsagef("Lorebook entry %q (%s) is split across multiple upload parts; remove it locally and run 'hearthroom card push %s --prune'", target.Name, target.EntryID, f.Dir)
	}

	entryMax, err := resolveEntryLimit(ctx, c, f, rb.detail.EntryContentMax)
	if err != nil {
		return nil, err
	}
	expectedVer, warnings := effectiveExpectedVersion(f, rb.detail.Version, opts)
	doc := map[string]any{
		"entries": []map[string]any{{"op": "delete", "entryId": target.EntryID}},
	}
	if expectedVer > 0 {
		doc["expectedVersion"] = expectedVer
	}

	var docRes struct {
		Version int32 `json:"version"`
	}
	if err := c.OpenPost(ctx, "/worldbook/"+rb.bookID+"/document", doc, &docRes); err != nil {
		if api.IsStatus(err, 409) {
			return nil, formatEntryConflictError(err, "delete", rb.roleID, f.Dir, entryRef, expectedVer, rb.detail.Version)
		}
		return nil, fmt.Errorf("delete Lorebook entry: %w", err)
	}

	var afterList entryListOutput
	if err := c.OpenGet(ctx, "/worldbook/entry/list", url.Values{"worldbookId": {rb.bookID}}, &afterList); err != nil {
		return nil, fmt.Errorf("read Lorebook entries after delete: %w", err)
	}

	if err := syncLocalEntryState(c, f, rb.bookID, rb.name, rb.detail, false, docRes.Version, "delete", target.EntryID, target.Name, nil, afterList.Entries, entryMax); err != nil {
		return nil, err
	}
	if hasWorldbookMarkdownSources(f.Dir) {
		warnings = append(warnings, "worldbook/*.md source files were not modified (only lorebook.json was updated); remove the source file before running 'hearthroom lorebook build'")
	}
	return &EntryWriteResult{
		Op:             "delete",
		RoleID:         rb.roleID,
		LorebookID:     rb.bookID,
		Version:        docRes.Version,
		DeletedEntryID: target.EntryID,
		DeletedName:    target.Name,
		Warnings:       warnings,
	}, nil
}

func singleEntryWireBody(e card.LorebookEntry, urls map[string]string) map[string]any {
	kw := e.Keywords
	if kw == nil {
		kw = []string{}
	}
	body := map[string]any{
		"name":      e.Name,
		"content":   card.Rewrite(e.Content, urls),
		"keywords":  kw,
		"isEnabled": !e.Disabled,
	}
	if e.SemanticRecall != nil {
		body["semanticRecall"] = *e.SemanticRecall
	}
	if len(e.SecondaryKeywords) > 0 {
		body["secondaryKeywords"] = e.SecondaryKeywords
	}
	if e.Category != "" {
		body["category"] = e.Category
	}
	if e.Constant {
		body["isConstant"] = true
	}
	if e.TriggerRegion != "" {
		body["triggerRegion"] = e.TriggerRegion
	}
	if len(e.MatchOptions) > 0 && string(e.MatchOptions) != "null" {
		var mo any
		if json.Unmarshal(e.MatchOptions, &mo) == nil {
			body["matchOptions"] = mo
		}
	}
	return body
}

// resolvedLocalEntryIDs returns the effective remote primary entry ID for each
// local entry in f.Lorebook.Entries, honoring state-tracked IDs for locally
// authored cards (where e.ID == "") and remapped --create targets.
func resolvedLocalEntryIDs(f *card.Folder) []string {
	if f.Lorebook == nil {
		return nil
	}
	out := make([]string, len(f.Lorebook.Entries))
	if f.State.IgnoreLocalLorebook {
		return out
	}
	sourceKeys := lorebookSourceKeys(f.Lorebook)
	for i, e := range f.Lorebook.Entries {
		id := ""
		if i < len(sourceKeys) {
			for _, key := range sourceKeys[i] {
				if e.ID != "" && key != "id:"+e.ID {
					continue
				}
				if mapped := f.State.LorebookSourceIDs[key]; mapped != "" {
					id = mapped
					break
				}
			}
		}
		if id == "" && e.ID == "" {
			id = f.State.LorebookEntryMap[strconv.Itoa(i)+":0"]
		}
		if id == "" {
			id = e.ID
		}
		out[i] = id
	}
	return out
}

func findLocalEntryIndexWithIDs(f *card.Folder, resolvedIDs []string, targetID, beforeName string) int {
	if f.Lorebook == nil {
		return -1
	}
	for i, id := range resolvedIDs {
		if id != "" && id == targetID {
			return i
		}
	}
	for i, e := range f.Lorebook.Entries {
		if e.ID == targetID {
			return i
		}
	}
	if beforeName != "" {
		matched := -1
		count := 0
		for i, e := range f.Lorebook.Entries {
			resolved := ""
			if i < len(resolvedIDs) {
				resolved = resolvedIDs[i]
			}
			if resolved == "" && e.Name == beforeName {
				matched = i
				count++
			}
		}
		if count == 1 {
			return matched
		}
	}
	return -1
}

func syncLocalEntryState(
	c *api.Client,
	f *card.Folder,
	bookID, bookName string,
	detail worldbookDetailOutput,
	createdBook bool,
	newVersion int32,
	opKind, targetID, beforeName string,
	savedEntry *card.LorebookEntry,
	afterRemoteEntries []card.RemoteEntry,
	entryMax int,
) error {
	urls := StoredURLs(f)
	wasClean := false
	if f.Lorebook == nil && len(f.State.LorebookEntryIDs) == 0 {
		wasClean = true
	} else if beforeBuilt, err := f.Build(urls); err == nil && beforeBuilt.Worldbook != nil {
		d := beforeBuilt.Digests()[card.SectionWorldbook]
		wasClean = d != "" && f.State.LocalHashes[card.SectionWorldbook] == d
	}

	// Preserve existing secondary fragment IDs keyed by their primary entry ID before mutating positions.
	fragmentsByPrimary := map[string]map[string]string{}
	for key, id := range f.State.LorebookEntryMap {
		prefix, suffix, ok := strings.Cut(key, ":")
		if !ok || suffix == "0" {
			continue
		}
		primary := f.State.LorebookEntryMap[prefix+":0"]
		if primary == "" || primary == targetID {
			continue
		}
		if fragmentsByPrimary[primary] == nil {
			fragmentsByPrimary[primary] = map[string]string{}
		}
		fragmentsByPrimary[primary][suffix] = id
	}

	beforeIDs := resolvedLocalEntryIDs(f)
	var afterIDs []string
	hadNoLocalLorebook := f.Lorebook == nil
	switch opKind {
	case "add", "update":
		if f.Lorebook == nil {
			entries := make([]card.LorebookEntry, 0, len(afterRemoteEntries))
			afterIDs = make([]string, 0, len(afterRemoteEntries))
			for _, re := range afterRemoteEntries {
				if savedEntry != nil && re.EntryID == targetID {
					entries = append(entries, *savedEntry)
				} else {
					entries = append(entries, card.RemoteToLorebookEntry(re))
				}
				afterIDs = append(afterIDs, re.EntryID)
			}
			f.Lorebook = &card.Lorebook{
				ID:          bookID,
				Name:        bookName,
				Description: detail.Description,
				Format:      detail.Format,
				Entries:     entries,
			}
		} else {
			if f.Lorebook.ID == "" {
				f.Lorebook.ID = bookID
			}
			if strings.TrimSpace(f.Lorebook.Name) == "" {
				f.Lorebook.Name = bookName
			}
			afterIDs = append([]string(nil), beforeIDs...)
			if opKind == "add" {
				f.Lorebook.Entries = append(f.Lorebook.Entries, *savedEntry)
				afterIDs = append(afterIDs, targetID)
			} else if idx := findLocalEntryIndexWithIDs(f, beforeIDs, targetID, beforeName); idx >= 0 {
				f.Lorebook.Entries[idx] = *savedEntry
				afterIDs[idx] = targetID
			} else {
				f.Lorebook.Entries = append(f.Lorebook.Entries, *savedEntry)
				afterIDs = append(afterIDs, targetID)
			}
		}
	case "delete":
		if f.Lorebook != nil {
			afterIDs = append([]string(nil), beforeIDs...)
			if idx := findLocalEntryIndexWithIDs(f, beforeIDs, targetID, beforeName); idx >= 0 {
				f.Lorebook.Entries = append(f.Lorebook.Entries[:idx], f.Lorebook.Entries[idx+1:]...)
				afterIDs = append(afterIDs[:idx], afterIDs[idx+1:]...)
			}
		}
	}

	f.State.API = c.API
	f.State.LorebookID = bookID
	if bookName != "" {
		f.State.LorebookName = bookName
	}
	// Advance the whole-Lorebook base version only when no intervening remote
	// versions were bypassed via --expected-version or --force; otherwise retain
	// the prior local base version so unpulled web edits on sibling entries
	// still trigger 409 on subsequent sibling or bulk writes.
	canAdvanceVersion := createdBook || hadNoLocalLorebook || (f.State.LorebookVersion > 0 && f.State.LorebookVersion == detail.Version)
	if newVersion > 0 && canAdvanceVersion {
		f.State.LorebookVersion = newVersion
	}
	f.State.LorebookUpload = nil
	f.State.IgnoreLocalLorebook = false

	known := map[string]bool{}
	for _, id := range f.State.LorebookEntryIDs {
		if id != "" {
			known[id] = true
		}
	}
	if f.State.LorebookEntryHashes == nil {
		f.State.LorebookEntryHashes = map[string]string{}
	}
	if hadNoLocalLorebook && f.Lorebook != nil {
		for _, e := range f.Lorebook.Entries {
			if e.ID != "" {
				known[e.ID] = true
				f.State.LorebookEntryHashes[e.ID] = entryOpDigest(singleEntryWireBody(e, urls))
			}
		}
	} else if opKind == "delete" {
		delete(known, targetID)
		delete(f.State.LorebookEntryHashes, targetID)
	} else if savedEntry != nil {
		known[targetID] = true
		f.State.LorebookEntryHashes[targetID] = entryOpDigest(singleEntryWireBody(*savedEntry, urls))
	}
	f.State.LorebookEntryIDs = make([]string, 0, len(known))
	for id := range known {
		f.State.LorebookEntryIDs = append(f.State.LorebookEntryIDs, id)
	}
	sort.Strings(f.State.LorebookEntryIDs)

	if f.Lorebook != nil {
		newMap := map[string]string{}
		for i, id := range afterIDs {
			if id == "" {
				continue
			}
			prefix := strconv.Itoa(i)
			newMap[prefix+":0"] = id
			for suffix, fragID := range fragmentsByPrimary[id] {
				newMap[prefix+":"+suffix] = fragID
			}
		}
		f.State.LorebookEntryMap = newMap

		sourceIDs := map[string]string{}
		for i, aliases := range lorebookSourceKeys(f.Lorebook) {
			if i < len(afterIDs) && afterIDs[i] != "" {
				for _, k := range aliases {
					sourceIDs[k] = afterIDs[i]
				}
			}
		}
		f.State.LorebookSourceIDs = sourceIDs
	}

	if afterBuilt, err := f.Build(urls); err == nil && afterBuilt.Worldbook != nil {
		if wasClean || localLorebookMatchesSyncedState(f, afterBuilt.Worldbook, afterRemoteEntries, entryMax) {
			if f.State.LocalHashes == nil {
				f.State.LocalHashes = map[string]string{}
			}
			f.State.LocalHashes[card.SectionWorldbook] = afterBuilt.Digests()[card.SectionWorldbook]
		}
	}

	return f.SaveLorebook()
}

func localLorebookMatchesSyncedState(f *card.Folder, wb map[string]any, remoteEntries []card.RemoteEntry, entryMax int) bool {
	if f.Lorebook == nil {
		return false
	}
	explicitName := strings.TrimSpace(f.Lorebook.Name)
	if explicitName != "" && f.State.LorebookName != "" && explicitName != f.State.LorebookName {
		return false
	}
	prepared, keys, _ := prepareEntries(f, wb, entryMax)
	bodies, _ := prepared["entries"].([]map[string]any)
	if len(bodies) != len(keys) {
		return false
	}
	presentIDs := map[string]bool{}
	for i, body := range bodies {
		id := f.State.LorebookEntryMap[keys[i]]
		if id == "" {
			return false
		}
		expectedHash := f.State.LorebookEntryHashes[id]
		if expectedHash == "" || expectedHash != entryOpDigest(body) {
			return false
		}
		presentIDs[id] = true
	}
	remoteSet := map[string]bool{}
	for _, re := range remoteEntries {
		remoteSet[re.EntryID] = true
	}
	for _, trackedID := range f.State.LorebookEntryIDs {
		if remoteSet[trackedID] && !presentIDs[trackedID] {
			return false
		}
	}
	return true
}
