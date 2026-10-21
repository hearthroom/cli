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
	Index int    `json:"index"`
	Name  string `json:"name"`
	Parts int    `json:"parts"`
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
		if local.Disabled || strings.TrimSpace(local.Content) == "" {
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
			groups = append(groups, GroupedEntry{Index: i, Name: local.Name, Parts: len(parts)})
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

func pushLorebook(ctx context.Context, c *api.Client, f *card.Folder, roleID string, wb map[string]any, opts PushOptions) error {
	bookID := f.State.LorebookID
	if bookID == "" && f.Lorebook != nil {
		bookID = f.Lorebook.ID
	}
	if bookID == "" {
		var bindings bindingsOutput
		if err := c.OpenGet(ctx, "/worldbook/bindings", url.Values{"roleId": {roleID}}, &bindings); err != nil {
			return fmt.Errorf("read Lorebook bindings: %w", err)
		}
		if len(bindings.Bindings) > 0 {
			bookID = bindings.Bindings[0].WorldbookID
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
		if err := c.OpenPost(ctx, "/worldbook", map[string]any{"name": name, "language": f.Manifest.Language}, &out); err != nil {
			return fmt.Errorf("create Lorebook: %w", err)
		}
		if out.WorldbookID == "" {
			return errors.New("create Lorebook: no id in response")
		}
		bookID = out.WorldbookID
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
	if f.State.LorebookUpload != nil && f.State.LorebookUpload.Digest != digest {
		if f.State.LorebookUpload.InFlight {
			return errors.New("the previous Lorebook batch has an unknown result; restore the previous files and push again to reconcile it")
		}
		f.State.LorebookUpload = nil
	}
	if f.State.LorebookEntryMap == nil {
		f.State.LorebookEntryMap = map[string]string{}
	}
	if f.State.LorebookUpload == nil {
		upload := &card.LorebookUpload{Digest: digest, BatchSize: opts.limits.MaxOpsPerCall}
		if upload.BatchSize <= 0 {
			upload.BatchSize = 200
		}
		byID := map[string]card.RemoteEntry{}
		byName := map[string][]card.RemoteEntry{}
		for _, e := range existing.Entries {
			byID[e.EntryID] = e
			byName[e.Name] = append(byName[e.Name], e)
		}
		entries, _ := wb["entries"].([]map[string]any)
		nameCount := map[string]int{}
		for _, e := range entries {
			n, _ := e["name"].(string)
			nameCount[n]++
		}
		seen := map[string]bool{}
		for i, body := range entries {
			key := opts.entryKeys[i]
			target := f.State.LorebookEntryMap[key]
			source, _ := strconv.Atoi(strings.Split(key, ":")[0])
			if target == "" && strings.HasSuffix(key, ":0") && source < len(f.Lorebook.Entries) {
				target = f.Lorebook.Entries[source].ID
			}
			if _, ok := byID[target]; !ok {
				target = ""
			}
			n, _ := body["name"].(string)
			if target == "" && n != "" && nameCount[n] == 1 && len(byName[n]) == 1 && !seen[byName[n][0].EntryID] {
				target = byName[n][0].EntryID
			}
			op := map[string]any{}
			for k, v := range body {
				op[k] = v
			}
			op["isEnabled"] = true
			if target != "" && !seen[target] {
				op["op"], op["entryId"] = "update", target
				seen[target] = true
			} else {
				op["op"] = "create"
			}
			upload.Ops = append(upload.Ops, op)
			upload.Keys = append(upload.Keys, key)
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
		for _, e := range existing.Entries {
			if known[e.EntryID] && !seen[e.EntryID] {
				upload.Ops = append(upload.Ops, map[string]any{"op": "delete", "entryId": e.EntryID})
				upload.Keys = append(upload.Keys, "")
			}
		}
		f.State.LorebookUpload = upload
		if err := f.SaveState(); err != nil {
			return err
		}
	}
	upload := f.State.LorebookUpload
	if upload.InFlight {
		if err := reconcileBatch(f, existing.Entries); err != nil {
			return err
		}
	}
	for upload.Next < len(upload.Ops) || (len(upload.Ops) == 0 && upload.CompletedBatches == 0) {
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
		}
		doc := map[string]any{"metadata": map[string]any{"name": name}, "binding": map[string]any{"roleId": roleID}, "entries": batch}
		if err := c.OpenPost(ctx, "/worldbook/"+bookID+"/document", doc, &response); err != nil {
			return fmt.Errorf("write Lorebook batch %d (progress saved; rerun the same command): %w", upload.CompletedBatches+1, err)
		}
		if err := completeBatch(f, response.CreatedEntryIDs); err != nil {
			return err
		}
		// Keep the baseline current for an unknown outcome in a later batch.
		if err := c.OpenGet(ctx, "/worldbook/entry/list", url.Values{"worldbookId": {bookID}}, &existing); err != nil {
			return fmt.Errorf("read Lorebook progress: %w", err)
		}
	}
	f.State.LorebookUpload = nil
	// Keep the original unsplit content on disk. IDs for fragments live in state.
	f.Lorebook.ID = bookID
	for i := range f.Lorebook.Entries {
		if id := f.State.LorebookEntryMap[strconv.Itoa(i)+":0"]; id != "" {
			f.Lorebook.Entries[i].ID = id
		}
	}
	return f.Save()
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
	if len(created) > 0 {
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
			if !ok || !matchesFields(m, other) {
				return false
			}
			continue
		}
		// The server omits groupOrder=0 in its canonical match options.
		if k == "groupOrder" && a == nil && v == float64(0) {
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
