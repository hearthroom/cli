package sync

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

func hasLocalCardImages(f *card.Folder) bool {
	for _, path := range []string{f.Manifest.Media.Portrait, f.Manifest.Media.Background, f.Manifest.Media.BackgroundLandscape, f.Manifest.Media.Share} {
		if strings.HasPrefix(path, "assets/") {
			return true
		}
	}
	return false
}

func ensureCardImageTarget(ctx context.Context, c *api.Client, f *card.Folder, target string, owned bool, opts PushOptions, res *PushResult) (string, error) {
	pending := f.State.CardImageProvision
	if opts.DryRun || (pending == nil && (target != "" || opts.SkipMedia || !hasLocalCardImages(f))) {
		return target, nil
	}
	var actor struct {
		ID int64 `json:"accountNumId"`
	}
	if err := c.OpenGet(ctx, "/me", nil, &actor); err != nil {
		return "", err
	}
	if actor.ID <= 0 {
		return "", errors.New("provider did not identify the signed-in account; card image creation stopped")
	}
	if pending != nil && (pending.API != c.API || pending.Actor != actor.ID) {
		return "", errors.New("unfinished card creation belongs to another provider or account; sign in to that account to resume")
	}
	wanted := "owned"
	if !owned {
		wanted = "trial"
	}
	if pending != nil && pending.Target != "" && pending.Target != wanted {
		return "", errors.New("finish the pending push with its original target before changing between trial and owned cards")
	}
	if target != "" {
		return target, nil
	}
	if pending == nil {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return "", err
		}
		id[6] = (id[6] & 15) | 64
		id[8] = (id[8] & 63) | 128
		operation := fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
		pending = &card.CardImageProvision{API: c.API, Actor: actor.ID, OperationID: operation, Name: f.Manifest.Name, Language: f.Manifest.Language, Target: wanted}
		f.State.CardImageProvision = pending
	}
	if !owned && f.State.TrialKey == "" {
		key, err := trialKeyFor(f.Dir)
		if err != nil {
			return "", err
		}
		f.State.TrialKey = key
	}
	if err := f.SaveState(); err != nil {
		return "", err
	}
	if !owned {
		var out trialResponse
		body := map[string]any{"name": pending.Name, "language": pending.Language, "evict": opts.Evict}
		if err := c.OpenPost(ctx, "/trial-cards/"+url.PathEscape(f.State.TrialKey)+"/ensure", body, &out); err != nil {
			return "", fmt.Errorf("create private trial target for images: %w", err)
		}
		if out.RoleID == "" {
			return "", errors.New("trial image target: no id in response")
		}
		f.State.RoleID, f.State.Target, f.State.API = out.RoleID, "trial", c.API
		if err := f.SaveState(); err != nil {
			return "", err
		}
		res.Created = out.Created
		res.RoleVisibility = "private"
		return out.RoleID, nil
	}
	var out roleOutput
	if err := c.OpenPost(ctx, "/role/provision", map[string]any{"operationId": pending.OperationID, "roleName": pending.Name, "language": pending.Language, "origin": "hearthroom"}, &out); err != nil {
		return "", fmt.Errorf("create private card for images: %w", err)
	}
	if out.RoleID == "" {
		return "", errors.New("create card for images: no id in response")
	}
	f.State.RoleID, f.State.Target, f.State.API = out.RoleID, "owned", c.API
	f.State.LocalHashes = map[string]string{}
	f.State.OwnedPushPending = true
	f.State.PrivatePending = false
	f.State.LorebookID = ""
	f.State.IgnoreLocalLorebook = true
	f.State.LorebookEntryIDs = nil
	f.State.LorebookEntryMap = nil
	f.State.LorebookSourceIDs = nil
	f.State.LorebookUpload = nil
	f.State.AuthorAssetVersion = 0
	if f.Lorebook != nil {
		f.Lorebook.ID = ""
		for i := range f.Lorebook.Entries {
			f.Lorebook.Entries[i].ID = ""
		}
	}
	if err := f.SaveState(); err != nil {
		return "", err
	}
	res.Created = true
	res.RoleVisibility = "private"
	return out.RoleID, nil
}
