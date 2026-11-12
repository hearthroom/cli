package sync

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/media"
)

// Card slot URLs are separate from text-path rewriting. A file used in both
// places deliberately has a card copy and a named personal-library copy.
func syncCardImages(ctx context.Context, c *api.Client, f *card.Folder, roleID string, opts PushOptions) (map[string]string, []media.Outcome, error) {
	out := map[string]string{}
	var outcomes []media.Outcome
	portrait := f.Manifest.Media.Background
	if portrait == "" {
		portrait = f.Manifest.Media.Portrait
	}
	slots := []struct{ field, path string }{{"roleBackground", portrait}, {"roleBackgroundLandscape", f.Manifest.Media.BackgroundLandscape}, {"roleShareImage", f.Manifest.Media.Share}}
	if f.State.CardImagesRoleID != roleID || f.State.API != c.API {
		f.State.CardImages = map[string]card.Asset{}
	}
	if f.State.CardImages == nil {
		f.State.CardImages = map[string]card.Asset{}
	}
	byDigest := map[string]card.Asset{}
	for _, slot := range slots {
		if !strings.HasPrefix(slot.path, "assets/") {
			continue
		}
		clean := filepath.ToSlash(filepath.Clean(slot.path))
		if !strings.HasPrefix(clean, "assets/") {
			return nil, nil, fmt.Errorf("card image %s must stay inside assets/", slot.path)
		}
		if opts.SkipMedia {
			saved := f.State.CardImages[slot.field]
			if saved.URL != "" {
				out[slot.field] = saved.URL
			} else if old := f.State.Assets[slot.path]; old.URL != "" {
				out[slot.field] = old.URL
			} else {
				return nil, nil, fmt.Errorf("card image %s has not been uploaded; run push without --skip-media", slot.path)
			}
			continue
		}
		local := filepath.Join(f.Dir, filepath.FromSlash(clean))
		digest, err := card.FileDigest(local)
		if err != nil {
			return nil, nil, err
		}
		saved := f.State.CardImages[slot.field]
		if saved.SHA256 == digest && saved.URL != "" {
			out[slot.field] = saved.URL
			byDigest[digest] = saved
			outcomes = append(outcomes, media.Outcome{Path: slot.path, URL: saved.URL})
			continue
		}
		if saved, ok := byDigest[digest]; ok {
			out[slot.field] = saved.URL
			f.State.CardImages[slot.field] = saved
			continue
		}
		if opts.DryRun {
			outcomes = append(outcomes, media.Outcome{Path: slot.path, Uploaded: true})
			continue
		}
		if roleID == "" {
			return nil, nil, fmt.Errorf("card image %s needs a card identity before upload", slot.path)
		}
		image, err := media.Upload(ctx, c, local, "", roleID)
		if err != nil {
			return nil, nil, fmt.Errorf("upload card image %s: %w", slot.path, err)
		}
		saved = card.Asset{SHA256: digest, URL: image.ImageURL, ID: fmt.Sprint(image.ImageID), StorageScope: "card"}
		f.State.CardImages[slot.field] = saved
		f.State.CardImagesRoleID = roleID
		f.State.API = c.API
		byDigest[digest] = saved
		out[slot.field] = saved.URL
		outcomes = append(outcomes, media.Outcome{Path: slot.path, URL: saved.URL, Uploaded: true})
		if err = f.SaveState(); err != nil {
			return nil, nil, err
		}
	}
	if value, ok := out["roleBackground"]; ok {
		out["roleAvatar"] = value
	}
	return out, outcomes, nil
}
