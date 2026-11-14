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
	"unicode"
	"unicode/utf8"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

type pushLimits struct {
	MaxOpsPerCall             int            `json:"maxOpsPerCall"`
	EntryContentMaxByLanguage map[string]int `json:"entryContentMaxByLanguage"`
	TrialMaxEntries           int            `json:"trialMaxEntries"`
	entryMax                  int
}

func readPushLimits(ctx context.Context, c *api.Client) (pushLimits, error) {
	var limits pushLimits
	err := c.OpenGet(ctx, "/worldbook/limits", nil, &limits)
	if err != nil && !api.IsStatus(err, 404) {
		return limits, fmt.Errorf("read Lorebook limits: %w", err)
	}
	// Providers predating limit discovery use the published v1 limits. Never
	// mask auth/network failures as old-server compatibility.
	if limits.MaxOpsPerCall <= 0 {
		limits.MaxOpsPerCall = 200
	}
	if limits.TrialMaxEntries <= 0 {
		limits.TrialMaxEntries = 1000
	}
	if len(limits.EntryContentMaxByLanguage) == 0 {
		limits.EntryContentMaxByLanguage = map[string]int{"zh-Hant": 4000, "zh-Hans": 4000, "ja": 6000, "ko": 6000, "en": 12000}
	}
	return limits, nil
}

type GroupedEntry struct {
	Characters int    `json:"characters"`
	Limit      int    `json:"limit"`
	Index      int    `json:"index"`
	Name       string `json:"name"`
	Parts      int    `json:"parts"`
}

// splitContent retains every rune, including paragraph separators and CRLF.
// Prefer the last paragraph boundary, then sentence boundary, then whitespace.
func splitContent(content string, limit int) []string {
	if limit <= 0 {
		return []string{content}
	}
	r := []rune(content)
	var out []string
	for len(r) > limit {
		cut := 0
		for i := 0; i < limit; i++ {
			if r[i] == '\n' {
				cut = i + 1
			}
		}
		if cut == 0 {
			for i := 0; i < limit; i++ {
				if strings.ContainsRune("。！？.!?", r[i]) {
					cut = i + 1
				}
			}
		}
		if cut == 0 {
			for i := 0; i < limit; i++ {
				if unicode.IsSpace(r[i]) {
					cut = i + 1
				}
			}
		}
		if cut == 0 {
			cut = limit
		}
		out = append(out, string(r[:cut]))
		r = r[cut:]
	}
	if len(r) > 0 {
		out = append(out, string(r))
	}
	return out
}

func prepareEntries(f *card.Folder, wb map[string]any, limit int) (map[string]any, []string, []GroupedEntry) {
	out := map[string]any{}
	for k, v := range wb {
		out[k] = v
	}
	entries, _ := wb["entries"].([]map[string]any)
	prepared := []map[string]any{}
	keys := []string{}
	groups := []GroupedEntry{}
	pos := 0
	for i, local := range f.Lorebook.Entries {
		if strings.TrimSpace(local.Content) == "" {
			continue
		}
		if pos >= len(entries) {
			break
		}
		entry := entries[pos]
		pos++
		mo, _ := entry["matchOptions"].(map[string]any)
		_, hasID := mo["groupId"]
		_, hasOrder := mo["groupOrder"]
		parts := []string{entry["content"].(string)}
		if !hasID && !hasOrder {
			parts = splitContent(parts[0], limit)
		}
		if len(parts) > 1 {
			groups = append(groups, GroupedEntry{Index: i, Name: local.Name, Parts: len(parts), Characters: utf8.RuneCountInString(entry["content"].(string)), Limit: limit})
		}
		groupID := "cli-" + card.Digest([]any{i, local.Name, entry["content"]})[:24]
		for j, part := range parts {
			body := map[string]any{}
			for k, v := range entry {
				body[k] = v
			}
			body["content"] = part
			if len(parts) > 1 {
				options := map[string]any{}
				for k, v := range mo {
					options[k] = v
				}
				options["groupId"], options["groupOrder"] = groupID, j
				body["matchOptions"] = options
			}
			prepared = append(prepared, body)
			keys = append(keys, strconv.Itoa(i)+":"+strconv.Itoa(j))
		}
	}
	out["entries"] = prepared
	return out, keys, groups
}

// Aliases survive reordering and edits without writing generated IDs into sources.
// Ambiguous names/content have no alias; explicit IDs or a fresh pull disambiguate them.
func lorebookSourceKeys(book *card.Lorebook) [][]string {
	names, hashes := map[string]int{}, map[string]int{}
	digests := make([]string, len(book.Entries))
	for i, e := range book.Entries {
		names[e.Name]++
		e.ID, e.Name = "", ""
		digests[i] = card.Digest(e)
		hashes[digests[i]]++
	}
	keys := make([][]string, len(book.Entries))
	for i, e := range book.Entries {
		if e.ID != "" {
			keys[i] = append(keys[i], "id:"+e.ID)
		}
		if hashes[digests[i]] == 1 {
			keys[i] = append(keys[i], "content:"+digests[i])
		}
		if e.Name != "" && names[e.Name] == 1 {
			keys[i] = append(keys[i], "name:"+e.Name)
		}
	}
	return keys
}

func entryOpDigest(op map[string]any) string {
	clean := make(map[string]any, len(op))
	for k, v := range op {
		if k == "op" || k == "entryId" {
			continue
		}
		clean[k] = v
	}
	return card.Digest(clean)
}

func planLorebook(f *card.Folder, wb map[string]any, existing []card.RemoteEntry, opts PushOptions) *card.LorebookUpload {
	upload := &card.LorebookUpload{
		Digest:      card.Digest(wb),
		BatchSize:   opts.limits.MaxOpsPerCall,
		RetainedMap: map[string]string{},
	}
	if upload.BatchSize <= 0 {
		upload.BatchSize = 200
	}
	if !opts.Force && f.State.LorebookVersion > 0 {
		upload.BaseVersion = f.State.LorebookVersion
	}
	byID := map[string]card.RemoteEntry{}
	byName := map[string][]card.RemoteEntry{}
	for _, e := range existing {
		byID[e.EntryID] = e
		byName[e.Name] = append(byName[e.Name], e)
	}
	entries, _ := wb["entries"].([]map[string]any)
	nameCount := map[string]int{}
	for _, e := range f.Lorebook.Entries {
		nameCount[e.Name]++
	}
	seen := map[string]bool{}
	retainedSources := map[string]bool{}
	for _, entry := range f.Lorebook.Entries {
		if entry.ID != "" {
			retainedSources[entry.ID] = true
		}
	}
	// Fragment keys use local positions, which change on removal/reorder.
	// Anchor the previous fragment family to the persisted first-entry ID.
	prefixByID := map[string]string{}
	for key, id := range f.State.LorebookEntryMap {
		if strings.HasSuffix(key, ":0") {
			prefixByID[id] = strings.TrimSuffix(key, ":0")
		}
	}
	for i, body := range entries {
		key := opts.entryKeys[i]
		parts := strings.Split(key, ":")
		source, _ := strconv.Atoi(parts[0])
		target := ""
		if source < len(f.Lorebook.Entries) {
			id := f.Lorebook.Entries[source].ID
			if parts[1] == "0" {
				target = id
			} else if prefix, ok := prefixByID[id]; ok {
				target = f.State.LorebookEntryMap[prefix+":"+parts[1]]
			}
			// A reconciled create reply may predate saving IDs to the
			// local file. Reuse its checkpoint ID only after checking the
			// content/options, so shifted positions cannot claim other rows.
			if id == "" {
				candidate := f.State.LorebookEntryMap[key]
				if existing, ok := byID[candidate]; ok {
					wanted := map[string]any{}
					for k, v := range body {
						wanted[k] = v
					}
					wanted["isEnabled"] = existing.IsEnabled
					if matchesEntry(wanted, existing) {
						target = candidate
					}
				}
			}
		}
		if _, ok := byID[target]; !ok {
			target = ""
		}
		n, _ := body["name"].(string)
		if target == "" && strings.HasSuffix(key, ":0") && n != "" && nameCount[n] == 1 && len(byName[n]) == 1 && !seen[byName[n][0].EntryID] {
			target = byName[n][0].EntryID
		}
		if target != "" && !seen[target] && !opts.Force && f.State.LorebookEntryHashes[target] != "" && f.State.LorebookEntryHashes[target] == entryOpDigest(body) {
			seen[target] = true
			if strings.HasSuffix(key, ":0") {
				retainedSources[target] = true
			}
			upload.RetainedMap[key] = target
			continue
		}
		op := map[string]any{}
		for k, v := range body {
			op[k] = v
		}
		if target != "" && !seen[target] {
			op["op"], op["entryId"] = "update", target
			seen[target] = true
			if strings.HasSuffix(key, ":0") {
				retainedSources[target] = true
			}
		} else {
			op["op"] = "create"
		}
		upload.Ops = append(upload.Ops, op)
		upload.Keys = append(upload.Keys, key)
		var aliases []string
		if strings.HasSuffix(key, ":0") && source < len(opts.sourceKeys) {
			aliases = opts.sourceKeys[source]
		}
		upload.SourceKeys = append(upload.SourceKeys, aliases)
	}
	known := map[string]bool{}
	for _, id := range f.State.LorebookEntryIDs {
		known[id] = true
	}
	for _, e := range f.Lorebook.Entries {
		if e.ID != "" {
			known[e.ID] = true
		}
	}
	fragments := fragmentIDs(f.State.LorebookEntryMap, retainedSources)
	for _, e := range existing {
		if known[e.EntryID] && !seen[e.EntryID] {
			upload.Ops = append(upload.Ops, map[string]any{"op": "delete", "entryId": e.EntryID})
			upload.Keys = append(upload.Keys, "")
			upload.SourceKeys = append(upload.SourceKeys, nil)
			if fragments[e.EntryID] {
				upload.FragmentDeletes = append(upload.FragmentDeletes, e.EntryID)
			}
		}
	}
	return upload
}

// Only secondary fragments of a still-present source are mechanical cleanup.
// Missing/ambiguous source identity stays protected as whole-entry deletion.
func fragmentIDs(entryMap map[string]string, sources map[string]bool) map[string]bool {
	fragments := map[string]bool{}
	for key, id := range entryMap {
		prefix, suffix, ok := strings.Cut(key, ":")
		part, err := strconv.Atoi(suffix)
		if ok && err == nil && part > 0 && sources[entryMap[prefix+":0"]] {
			fragments[id] = true
		}
	}
	return fragments
}

func pruneEntryNames(f *card.Folder, upload *card.LorebookUpload, existing []card.RemoteEntry) []string {
	sources := map[string]bool{}
	for _, entry := range f.Lorebook.Entries {
		if entry.ID != "" {
			sources[entry.ID] = true
		}
	}
	fragments := fragmentIDs(f.State.LorebookEntryMap, sources)
	// Persisted classification survives earlier batches rewriting positional keys.
	for _, id := range upload.FragmentDeletes {
		fragments[id] = true
	}
	byID := map[string]string{}
	for _, entry := range existing {
		byID[entry.EntryID] = entry.Name
	}
	names := []string{}
	for _, op := range upload.Ops[upload.Next:] {
		if op["op"] == "delete" {
			id, _ := op["entryId"].(string)
			if !fragments[id] {
				names = append(names, byID[id])
			}
		}
	}
	return names
}

func previewLorebook(ctx context.Context, c *api.Client, f *card.Folder, roleID string, wb map[string]any, opts PushOptions) ([]string, error) {
	// A fresh --create has no prior entries; never preview pruning the old card.
	if roleID == "" || (opts.Create && !f.State.OwnedPushPending) {
		return nil, nil
	}
	local := *f
	bookID := ""
	if roleID == f.State.RoleID && f.State.Target == "owned" {
		bookID = f.State.LorebookID
		if bookID == "" && !f.State.IgnoreLocalLorebook {
			bookID = f.Lorebook.ID
		}
	} else {
		// Re-linking must use the new target's identities, as real push does.
		local.State = card.State{}
		book := *f.Lorebook
		book.ID = ""
		book.Entries = append([]card.LorebookEntry(nil), book.Entries...)
		for i := range book.Entries {
			book.Entries[i].ID = ""
		}
		local.Lorebook = &book
	}
	if bookID == "" {
		var bindings bindingsOutput
		if err := c.OpenGet(ctx, "/worldbook/bindings", url.Values{"roleId": {roleID}}, &bindings); err != nil {
			return nil, fmt.Errorf("read Lorebook bindings: %w", err)
		}
		var ok bool
		bookID, _, ok = latestBinding(bindings)
		if !ok {
			return nil, nil
		}
	}
	var existing entryListOutput
	if err := c.OpenGet(ctx, "/worldbook/entry/list", url.Values{"worldbookId": {bookID}}, &existing); err != nil {
		return nil, fmt.Errorf("list Lorebook entries: %w", err)
	}
	plan := planLorebook(&local, wb, existing.Entries, opts)
	return pruneEntryNames(&local, plan, existing.Entries), nil
}

func pushLorebook(ctx context.Context, c *api.Client, f *card.Folder, roleID string, wb map[string]any, opts PushOptions, res *PushResult) error {
	bookID := f.State.LorebookID
	if bookID == "" && f.Lorebook != nil && !f.State.IgnoreLocalLorebook {
		bookID = f.Lorebook.ID
	}
	alreadyBound := opts.remoteBookBound && opts.remoteBookID == bookID && bookID != ""
	remoteName := opts.remoteBookName
	remoteVersion := opts.remoteBookVersion
	if bookID == "" {
		var bindings bindingsOutput
		if err := c.OpenGet(ctx, "/worldbook/bindings", url.Values{"roleId": {roleID}}, &bindings); err != nil {
			return fmt.Errorf("read Lorebook bindings: %w", err)
		}
		if bid, bname, ok := latestBinding(bindings); ok {
			bookID = bid
			remoteName = bname
			alreadyBound = true
			if opts.remoteBookID != bookID {
				var detail worldbookDetailOutput
				if err := c.OpenGet(ctx, "/worldbook/detail", url.Values{"worldbookId": {bookID}}, &detail); err == nil {
					if detail.Name != "" {
						remoteName = detail.Name
					}
					remoteVersion = detail.Version
				}
			}
		}
	}
	explicitName, _ := wb["name"].(string)
	explicitName = strings.TrimSpace(explicitName)
	createName := explicitName
	if createName == "" {
		createName = f.Manifest.Name
	}
	createdBook := false
	if bookID == "" {
		var out struct {
			WorldbookID string `json:"worldbookId"`
			Version     int32  `json:"version"`
		}
		if err := c.OpenPost(ctx, "/worldbook", map[string]any{"name": createName, "language": f.Manifest.Language}, &out); err != nil {
			return fmt.Errorf("create Lorebook: %w", err)
		}
		if out.WorldbookID == "" {
			return errors.New("create Lorebook: no id in response")
		}
		bookID = out.WorldbookID
		createdBook = true
		alreadyBound = false
		remoteName = createName
		remoteVersion = out.Version
		f.State.LorebookVersion = out.Version
		f.State.LorebookName = createName
	} else if opts.Force && res != nil {
		if f.State.LorebookVersion > 0 && remoteVersion > 0 && remoteVersion != f.State.LorebookVersion {
			res.Warnings = append(res.Warnings, fmt.Sprintf("--force overwrites online Lorebook changes on %s (remote version %d, local base version %d)", bookID, remoteVersion, f.State.LorebookVersion))
		} else {
			res.Warnings = append(res.Warnings, fmt.Sprintf("--force overwrites remote Lorebook %s without version conflict check", bookID))
		}
	}
	f.State.LorebookID = bookID
	if err := f.SaveState(); err != nil {
		return err
	}
	var existing entryListOutput
	if err := c.OpenGet(ctx, "/worldbook/entry/list", url.Values{"worldbookId": {bookID}}, &existing); err != nil {
		return fmt.Errorf("list Lorebook entries: %w", err)
	}
	digest := card.Digest(wb)
	if f.State.LorebookEntryMap == nil {
		f.State.LorebookEntryMap = map[string]string{}
	}
	if f.State.LorebookUpload != nil && f.State.LorebookUpload.Digest != digest {
		if f.State.LorebookUpload.InFlight {
			prevBatches := f.State.LorebookUpload.CompletedBatches
			if err := reconcileBatch(f, existing.Entries); err != nil {
				return err
			}
			if f.State.LorebookUpload.CompletedBatches > prevBatches && remoteVersion > 0 {
				f.State.LorebookVersion = remoteVersion
			}
		}
		f.State.LorebookUpload = nil
	}
	if f.State.LorebookUpload == nil {
		if !opts.Force && !createdBook && (f.State.LorebookVersion == 0 || f.State.LorebookVersion == remoteVersion) {
			if missingName, missingID, missing := missingTrackedEntry(f, existing.Entries); missing {
				return fmt.Errorf("Lorebook entry %q (%s) was deleted on the server since last sync; run 'hearthroom card pull %s %s --force' to merge remote changes, or rerun push with --force to re-create it", missingName, missingID, roleID, f.Dir)
			}
		}
		upload := planLorebook(f, wb, existing.Entries, opts)
		upload.CreatedUnbound = createdBook
		if createdBook && !opts.Force && remoteVersion > 0 {
			upload.BaseVersion = remoteVersion
		}
		if !opts.Prune && !createdBook {
			names := pruneEntryNames(f, upload, existing.Entries)
			for i, name := range names {
				names[i] = fmt.Sprintf("%q", name)
			}
			if len(names) > 0 {
				return fmt.Errorf("Lorebook entries would be deleted: %s; rerun with --prune to delete them", strings.Join(names, ", "))
			}
		}
		f.State.LorebookUpload = upload
		if err := f.SaveState(); err != nil {
			return err
		}
	}
	upload := f.State.LorebookUpload
	if upload.InFlight {
		prevBatches := upload.CompletedBatches
		if err := reconcileBatch(f, existing.Entries); err != nil {
			return err
		}
		if upload.CompletedBatches > prevBatches {
			upload.CreatedUnbound = false
			if remoteVersion > 0 {
				upload.BaseVersion = remoteVersion
				f.State.LorebookVersion = remoteVersion
			}
		}
	}
	// Check saved plans too: an older CLI may have checkpointed deletes.
	if !opts.Prune {
		names := pruneEntryNames(f, upload, existing.Entries)
		for i, name := range names {
			names[i] = fmt.Sprintf("%q", name)
		}
		if len(names) > 0 {
			return fmt.Errorf("Lorebook entries would be deleted: %s; rerun with --prune to delete them", strings.Join(names, ", "))
		}
	}
	knownName := strings.TrimSpace(remoteName)
	if knownName == "" {
		knownName = strings.TrimSpace(f.State.LorebookName)
	}
	metaNeeded := !createdBook && explicitName != "" && knownName != "" && explicitName != knownName
	if len(upload.Ops) == 0 && !metaNeeded && alreadyBound {
		if !opts.Force && upload.BaseVersion > 0 && remoteVersion > 0 && remoteVersion != upload.BaseVersion {
			f.State.LorebookUpload = nil
			_ = f.SaveState()
			return formatLorebookConflictError(nil, roleID, f.Dir, upload.BaseVersion, remoteVersion)
		}
		if remoteVersion > 0 {
			f.State.LorebookVersion = remoteVersion
		}
	}
	for upload.Next < len(upload.Ops) || (len(upload.Ops) == 0 && upload.CompletedBatches == 0 && (metaNeeded || !alreadyBound)) {
		end := min(upload.Next+upload.BatchSize, len(upload.Ops))
		batch := upload.Ops[upload.Next:end]
		// Save intent before sending: a crash or lost reply is reconciled by
		// independently listing the book, never by blindly replaying creates.
		upload.InFlight = true
		upload.BeforeIDs = nil
		for _, e := range existing.Entries {
			upload.BeforeIDs = append(upload.BeforeIDs, e.EntryID)
		}
		if err := f.SaveState(); err != nil {
			return err
		}
		var response struct {
			CreatedEntryIDs []string `json:"createdEntryIds"`
			Version         int32    `json:"version"`
		}
		doc := map[string]any{"entries": batch}
		if upload.CompletedBatches == 0 {
			if metaNeeded {
				doc["metadata"] = map[string]any{"name": explicitName}
			}
			if !alreadyBound {
				doc["binding"] = map[string]any{"roleId": roleID}
			}
		}
		if !opts.Force && upload.BaseVersion > 0 {
			doc["expectedVersion"] = upload.BaseVersion
		}
		if err := c.OpenPost(ctx, "/worldbook/"+bookID+"/document", doc, &response); err != nil {
			if api.IsStatus(err, 409) {
				if upload.CompletedBatches == 0 {
					f.State.LorebookUpload = nil
				} else {
					upload.InFlight = false
				}
				_ = f.SaveState()
				return formatLorebookConflictError(err, roleID, f.Dir, upload.BaseVersion, 0)
			}
			return fmt.Errorf("write Lorebook batch %d (progress saved; rerun the same command): %w", upload.CompletedBatches+1, err)
		}
		if response.Version > 0 {
			upload.BaseVersion = response.Version
			f.State.LorebookVersion = response.Version
		}
		upload.CreatedUnbound = false
		if err := completeBatch(f, response.CreatedEntryIDs); err != nil {
			return err
		}
		// Keep the baseline current for an unknown outcome in a later batch.
		if err := c.OpenGet(ctx, "/worldbook/entry/list", url.Values{"worldbookId": {bookID}}, &existing); err != nil {
			return fmt.Errorf("read Lorebook progress: %w", err)
		}
	}
	if explicitName != "" && (createdBook || metaNeeded) {
		f.State.LorebookName = explicitName
	} else if knownName != "" {
		f.State.LorebookName = knownName
	}
	retainedMap := upload.RetainedMap
	f.State.LorebookUpload = nil
	currentMap := map[string]string{}
	for _, key := range opts.entryKeys {
		if id, ok := retainedMap[key]; ok {
			currentMap[key] = id
		} else {
			currentMap[key] = f.State.LorebookEntryMap[key]
		}
	}
	f.State.LorebookEntryMap = currentMap
	sourceIDs := map[string]string{}
	for i, aliases := range opts.sourceKeys {
		if id := currentMap[strconv.Itoa(i)+":0"]; id != "" {
			for _, key := range aliases {
				sourceIDs[key] = id
			}
		}
	}
	f.State.LorebookSourceIDs = sourceIDs
	entries, _ := wb["entries"].([]map[string]any)
	entryHashes := map[string]string{}
	known := map[string]bool{}
	for _, id := range f.State.LorebookEntryIDs {
		known[id] = true
	}
	for i, body := range entries {
		if i >= len(opts.entryKeys) {
			break
		}
		if id := currentMap[opts.entryKeys[i]]; id != "" {
			entryHashes[id] = entryOpDigest(body)
			known[id] = true
		}
	}
	f.State.LorebookEntryHashes = entryHashes
	f.State.LorebookEntryIDs = nil
	for id := range known {
		f.State.LorebookEntryIDs = append(f.State.LorebookEntryIDs, id)
	}
	sort.Strings(f.State.LorebookEntryIDs)
	// Generated identities and fragment progress belong only to sync state.
	return f.SaveState()
}

func missingTrackedEntry(f *card.Folder, existing []card.RemoteEntry) (string, string, bool) {
	if f.Lorebook == nil || len(f.State.LorebookEntryIDs) == 0 {
		return "", "", false
	}
	tracked := make(map[string]bool, len(f.State.LorebookEntryIDs))
	for _, id := range f.State.LorebookEntryIDs {
		tracked[id] = true
	}
	remoteHas := make(map[string]bool, len(existing))
	for _, e := range existing {
		remoteHas[e.EntryID] = true
	}
	resolvedIDs := resolvedLocalEntryIDs(f)
	for i, e := range f.Lorebook.Entries {
		id := e.ID
		if i < len(resolvedIDs) && resolvedIDs[i] != "" {
			id = resolvedIDs[i]
		}
		if id != "" && tracked[id] && !remoteHas[id] {
			return e.Name, id, true
		}
	}
	return "", "", false
}

// conflictError replaces the provider error prose with actionable CLI guidance
// while preserving the underlying *api.Error for errors.As and --json detail.
type conflictError struct {
	msg string
	err error
}

func (e conflictError) Error() string { return e.msg }
func (e conflictError) Unwrap() error { return e.err }

func formatLorebookConflictError(err error, roleID, dir string, expectedVer, remoteVer int32) error {
	var ae *api.Error
	if remoteVer == 0 && errors.As(err, &ae) {
		if detail, ok := ae.Detail.(map[string]any); ok {
			if cv, ok := detail["currentVersion"].(float64); ok {
				remoteVer = int32(cv)
			}
		}
	}
	if remoteVer > 0 && expectedVer > 0 {
		msg := fmt.Sprintf("remote Lorebook was modified on the server (remote version %d, local base version %d); run 'hearthroom card pull %s %s --force' to fetch remote changes and merge, or rerun 'hearthroom card push %s --force' to overwrite online Lorebook edits", remoteVer, expectedVer, roleID, dir, dir)
		if err != nil {
			return conflictError{msg: msg, err: err}
		}
		return errors.New(msg)
	}
	if err != nil {
		return fmt.Errorf("remote Lorebook was modified on the server (%w); run 'hearthroom card pull %s %s --force' to fetch remote changes and merge, or rerun 'hearthroom card push %s --force' to overwrite online Lorebook edits", err, roleID, dir, dir)
	}
	return fmt.Errorf("remote Lorebook was modified on the server; run 'hearthroom card pull %s %s --force' to fetch remote changes and merge, or rerun 'hearthroom card push %s --force' to overwrite online Lorebook edits", roleID, dir, dir)
}

func completeBatch(f *card.Folder, created []string) error {
	u := f.State.LorebookUpload
	end := min(u.Next+u.BatchSize, len(u.Ops))
	expected := 0
	for _, op := range u.Ops[u.Next:end] {
		if op["op"] == "create" {
			expected++
		}
	}
	if expected != len(created) {
		return fmt.Errorf("Lorebook response contains %d new entry ids; expected %d; rerun to reconcile the saved batch", len(created), expected)
	}
	known := map[string]bool{}
	for _, id := range f.State.LorebookEntryIDs {
		known[id] = true
	}
	cursor := 0
	for i := u.Next; i < end; i++ {
		op := u.Ops[i]
		id, _ := op["entryId"].(string)
		if op["op"] == "create" {
			id = created[cursor]
			cursor++
		}
		if op["op"] == "delete" {
			delete(known, id)
		} else {
			f.State.LorebookEntryMap[u.Keys[i]] = id
			if i < len(u.SourceKeys) {
				if f.State.LorebookSourceIDs == nil {
					f.State.LorebookSourceIDs = map[string]string{}
				}
				for _, key := range u.SourceKeys[i] {
					f.State.LorebookSourceIDs[key] = id
				}
			}
			known[id] = true
		}
	}
	f.State.LorebookEntryIDs = nil
	for id := range known {
		f.State.LorebookEntryIDs = append(f.State.LorebookEntryIDs, id)
	}
	sort.Strings(f.State.LorebookEntryIDs)
	u.Next = end
	u.CompletedBatches++
	u.InFlight = false
	u.BeforeIDs = nil
	return f.SaveState()
}

// Reconcile an atomic document whose reply was lost. A partial/ambiguous
// readback stops rather than creating more entries or erasing external edits.
func reconcileBatch(f *card.Folder, remote []card.RemoteEntry) error {
	u := f.State.LorebookUpload
	end := min(u.Next+u.BatchSize, len(u.Ops))
	if end == u.Next {
		u.InFlight = false
		return f.SaveState()
	}
	before := map[string]bool{}
	for _, id := range u.BeforeIDs {
		before[id] = true
	}
	byID := map[string]card.RemoteEntry{}
	var added []card.RemoteEntry
	for _, e := range remote {
		byID[e.EntryID] = e
		if !before[e.EntryID] {
			added = append(added, e)
		}
	}
	created := []string{}
	used := map[string]bool{}
	allApplied := true
	for offset, op := range u.Ops[u.Next:end] {
		id, _ := op["entryId"].(string)
		switch op["op"] {
		case "create":
			matches := []string{}
			for _, e := range added {
				if !used[e.EntryID] && matchesEntry(op, e) {
					matches = append(matches, e.EntryID)
				}
			}
			remaining := 0
			for _, other := range u.Ops[u.Next+offset : end] {
				if other["op"] == "create" && card.Digest(other) == card.Digest(op) {
					remaining++
				}
			}
			if len(matches) > remaining {
				return errors.New("Lorebook batch readback is ambiguous; check the remote entries before continuing")
			}
			if len(matches) > 0 && len(matches) < remaining {
				return errors.New("Lorebook batch readback is incomplete; check the remote entries before continuing")
			}
			if len(matches) == remaining {
				created = append(created, matches[0])
				used[matches[0]] = true
			} else {
				allApplied = false
			}
		case "update":
			e, ok := byID[id]
			if !ok || !matchesEntry(op, e) {
				allApplied = false
			}
		case "delete":
			if _, ok := byID[id]; ok {
				allApplied = false
			}
		}
	}
	if allApplied {
		return completeBatch(f, created)
	}
	if len(created) > 0 || len(added) > 0 {
		return errors.New("Lorebook batch readback is incomplete; check the remote entries before continuing")
	}
	// No creates became visible. A failed transaction can safely be retried;
	// remove deletes already absent so repeated deletion cannot fail the batch.
	for i := u.Next; i < end; i++ {
		if u.Ops[i]["op"] == "delete" {
			id, _ := u.Ops[i]["entryId"].(string)
			if _, ok := byID[id]; !ok {
				return errors.New("Lorebook changed during the interrupted batch; check the remote entries before continuing")
			}
		}
	}
	u.InFlight = false
	return f.SaveState()
}

func matchesEntry(op map[string]any, e card.RemoteEntry) bool {
	b, _ := json.Marshal(e)
	var actual map[string]any
	json.Unmarshal(b, &actual)
	b, _ = json.Marshal(op)
	var wanted map[string]any
	json.Unmarshal(b, &wanted)
	delete(wanted, "op")
	delete(wanted, "entryId")
	return matchesFields(wanted, actual)
}

func matchesFields(wanted, actual map[string]any) bool {
	for k, v := range wanted {
		a := actual[k]
		if m, ok := v.(map[string]any); ok {
			other, ok := a.(map[string]any)
			if k == "matchOptions" {
				wantedJSON, _ := json.Marshal(canonicalMatchOptions(m))
				actualJSON, _ := json.Marshal(canonicalMatchOptions(other))
				if string(wantedJSON) != string(actualJSON) {
					return false
				}
				continue
			}
			if !ok || !matchesFields(m, other) {
				return false
			}
			continue
		}
		x, _ := json.Marshal(a)
		y, _ := json.Marshal(v)
		if string(x) != string(y) {
			return false
		}
	}
	return true
}

// MatchOptions is canonicalized by the Provider's typed JSON structure. Omitted
// zero values and unknown input keys cannot be used to infer a failed write.
func canonicalMatchOptions(in map[string]any) map[string]any {
	out := map[string]any{"caseSensitive": false, "matchWholeWords": false, "selectiveLogic": float64(0)}
	for _, k := range []string{"selective", "scanDepth", "groupId", "groupOrder", "order", "extensions", "caseSensitive", "matchWholeWords", "selectiveLogic"} {
		v, ok := in[k]
		if !ok || v == nil {
			continue
		}
		if (k == "scanDepth" || k == "groupOrder") && v == float64(0) {
			continue
		}
		if k == "groupId" && v == "" {
			continue
		}
		if k == "extensions" {
			if m, ok := v.(map[string]any); ok && len(m) == 0 {
				continue
			}
		}
		out[k] = v
	}
	return out
}
