// Package media uploads local assets to the provider's media library and
// tracks what was uploaded so unchanged files are skipped.
package media

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

// Capabilities is the subset of GET /open/v1/image/list the CLI relies on.
type Capabilities struct {
	RelativePaths bool
	Moves         bool // renaming or moving a folder or file moves its path and URL
	Overwrite     bool
	LibraryPrefix string
	MaxFileBytes  int64
	Formats       []string
}

type listResponse struct {
	Code int `json:"code"`
	Data struct {
		Capabilities struct {
			RelativePaths bool     `json:"relativePaths"`
			Moves         bool     `json:"moves"`
			Overwrite     bool     `json:"overwrite"`
			MaxFileBytes  int64    `json:"maxFileBytes"`
			Formats       []string `json:"formats"`
		} `json:"capabilities"`
		LibraryPrefix string `json:"libraryPrefix"`
	} `json:"data"`
}

// Probe reads the library capabilities with one small list request.
func Probe(ctx context.Context, c *api.Client) (Capabilities, error) {
	var resp listResponse
	q := url.Values{"pageSize": {"1"}}
	if err := c.OpenGet(ctx, "/image/list", q, &resp); err != nil {
		return Capabilities{}, fmt.Errorf("read media library capabilities: %w", err)
	}
	return Capabilities{
		RelativePaths: resp.Data.Capabilities.RelativePaths,
		Moves:         resp.Data.Capabilities.Moves,
		Overwrite:     resp.Data.Capabilities.Overwrite,
		LibraryPrefix: strings.TrimRight(resp.Data.LibraryPrefix, "/"),
		MaxFileBytes:  resp.Data.Capabilities.MaxFileBytes,
		Formats:       resp.Data.Capabilities.Formats,
	}, nil
}

// UploadResult is the provider's upload response.
type UploadResult struct {
	ImageID         any    `json:"imageId"` // number, or string when the provider deduplicated by content
	ImageURL        string `json:"imageUrl"`
	Kind            string `json:"kind"`
	MimeType        string `json:"mimeType"`
	ByteSize        int64  `json:"byteSize"`
	FileName        string `json:"fileName"`
	Replaced        bool   `json:"replaced"`
	ModerationState string `json:"moderationState"`
}

type uploadResponse struct {
	Code int          `json:"code"`
	Data UploadResult `json:"data"`
}

// Upload sends one file. relativePath is used when the provider supports it;
// roleID may be empty.
func Upload(ctx context.Context, c *api.Client, localPath, relativePath, roleID string) (UploadResult, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return UploadResult{}, err
	}
	defer f.Close()
	fields := map[string][]string{}
	if relativePath != "" {
		fields["relativePath"] = []string{relativePath}
	}
	if roleID != "" {
		fields["roleId"] = []string{roleID}
	}
	var resp uploadResponse
	if err := c.UploadFile(ctx, "/image/upload", fields, "file", filepath.Base(localPath), f, &resp); err != nil {
		return UploadResult{}, err
	}
	if resp.Data.ImageURL == "" {
		return UploadResult{}, fmt.Errorf("upload %s: no URL in response", filepath.Base(localPath))
	}
	return resp.Data, nil
}

// Outcome describes one asset after Sync.
type Outcome struct {
	Path     string `json:"path"`
	URL      string `json:"url"`
	Uploaded bool   `json:"uploaded"`
	Replaced bool   `json:"replaced,omitempty"`
	Previous string `json:"previous,omitempty"` // library path it was served from before a folder change; still in the library
	Err      string `json:"error,omitempty"`
}

// Folder picks the library folder for the card's assets: media.folder when
// set, else the folder recorded in state, else the folder earlier uploads
// went to, else one derived from the card name (fresh=true: not yet ours).
func Folder(f *card.Folder) (folder string, explicit, fresh bool) {
	if name, ok := f.LibraryFolder(); ok {
		return name, true, false
	}
	if f.State.AssetFolder != "" {
		return f.State.AssetFolder, false, false
	}
	keys := make([]string, 0, len(f.State.Assets))
	for k := range f.State.Assets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fn := f.State.Assets[k].FileName
		if i := strings.Index(fn, "/"); i > 0 {
			return fn[:i], false, false
		}
	}
	name, _ := f.LibraryFolder()
	return name, false, true
}

// FolderOf returns the library folder a served URL points into, or "" when
// the URL is not a <libraryPrefix>/<folder>/<file> library path.
func FolderOf(libraryPrefix, rawURL string) string {
	rest, ok := strings.CutPrefix(rawURL, strings.TrimRight(libraryPrefix, "/")+"/")
	if !ok || libraryPrefix == "" {
		return ""
	}
	seg, _, ok := strings.Cut(rest, "/")
	if !ok {
		return ""
	}
	folder, err := url.PathUnescape(seg)
	if err != nil {
		return ""
	}
	return folder
}

// followMoves updates the recorded URLs of files that were renamed or moved in
// the library since the last push (the URL is the path, so the old one no longer
// serves), and forgets files that were deleted, so they are uploaded again.
// When the files moved to another folder, that folder becomes the card's.
func followMoves(ctx context.Context, c *api.Client, f *card.Folder, libraryPrefix string) error {
	wanted := map[string]string{} // id -> relative asset path
	for rel, a := range f.State.Assets {
		if a.ID != "" {
			wanted[a.ID] = rel
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	served := map[string]string{}
	complete := false
	for page := 1; page <= 20 && len(served) < len(wanted); page++ {
		var out struct {
			Data struct {
				Total     int64 `json:"total"`
				ImageList []struct {
					ID  any    `json:"id"`
					URL string `json:"imageUrl"`
				} `json:"imageList"`
			} `json:"data"`
		}
		q := url.Values{"kind": {"all"}, "pageSize": {"100"}, "pageNum": {fmt.Sprint(page)}}
		if err := c.OpenGet(ctx, "/image/list", q, &out); err != nil {
			return fmt.Errorf("read media library: %w", err)
		}
		for _, it := range out.Data.ImageList {
			if id := fmt.Sprint(it.ID); wanted[id] != "" {
				served[id] = it.URL
			}
		}
		if int64(page*100) >= out.Data.Total || len(out.Data.ImageList) == 0 {
			complete = true
			break
		}
	}
	folders := map[string]bool{}
	for id, rel := range wanted {
		a := f.State.Assets[rel]
		u, ok := served[id]
		switch {
		case !ok && complete:
			delete(f.State.Assets, rel) // deleted in the library: upload again
		case ok && u != a.URL:
			a.URL = u
			f.State.Assets[rel] = a
			if dir := FolderOf(libraryPrefix, u); dir != "" {
				folders[dir] = true
			}
		}
	}
	if len(folders) != 1 {
		return nil
	}
	var moved string
	for dir := range folders {
		moved = dir
	}
	if moved == f.State.AssetFolder {
		return nil
	}
	if name, explicit := f.LibraryFolder(); explicit && name != moved {
		return fmt.Errorf("this card's files were moved to the media folder %q in the library; set media.folder in card.json to %q so the next upload goes there too", moved, moved)
	}
	f.State.AssetFolder = moved
	return nil
}

// foreignFiles lists files already under folder/ that this card did not upload.
func foreignFiles(ctx context.Context, c *api.Client, f *card.Folder, libraryPrefix, folder string) ([]string, error) {
	var out struct {
		Data struct {
			ImageList []struct {
				FileName string `json:"fileName"`
				URL      string `json:"imageUrl"`
			} `json:"imageList"`
		} `json:"data"`
	}
	q := url.Values{"q": {folder + "/"}, "kind": {"all"}, "pageSize": {"20"}}
	if err := c.OpenGet(ctx, "/image/list", q, &out); err != nil {
		return nil, fmt.Errorf("check media folder %q: %w", folder, err)
	}
	ours := map[string]bool{}
	for _, a := range f.State.Assets {
		ours[a.URL] = true
	}
	var foreign []string
	for _, it := range out.Data.ImageList {
		if strings.HasPrefix(it.URL, libraryPrefix+"/"+escapePath(folder)+"/") && !ours[it.URL] {
			foreign = append(foreign, it.FileName)
		}
	}
	return foreign, nil
}

// escapePath percent-escapes each segment the way the library serves it.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

// Sync uploads every referenced asset of the folder that is new, changed or
// served from another library folder, records it in the folder state and
// returns the path→URL map, including referenced directories. assets/<path>
// goes to <folder>/<path> (see Folder). Failures are reported per file; the
// map still contains every asset that has a URL.
func Sync(ctx context.Context, c *api.Client, f *card.Folder, caps Capabilities, dryRun bool) (map[string]string, []Outcome, error) {
	present, missing := f.AssetRefs()
	urls := map[string]string{}
	var outcomes []Outcome
	if caps.RelativePaths {
		if err := followMoves(ctx, c, f, caps.LibraryPrefix); err != nil {
			return nil, nil, err
		}
	}
	folder, _, fresh := Folder(f)
	if caps.RelativePaths && fresh && len(present) > 0 {
		foreign, err := foreignFiles(ctx, c, f, caps.LibraryPrefix, folder)
		if err != nil {
			return nil, nil, err
		}
		if len(foreign) > 0 {
			return nil, nil, fmt.Errorf("media folder %q already holds files this card did not upload (%s). Set media.folder in card.json to a new name, or to %q to share that folder on purpose; an upload with the same path replaces that file for every card that uses it", folder, strings.Join(foreign[:min(3, len(foreign))], ", "), folder)
		}
	}
	for _, m := range missing {
		outcomes = append(outcomes, Outcome{Path: m, Err: "file not found"})
	}
	if f.State.Assets == nil {
		f.State.Assets = map[string]card.Asset{}
	}
	for _, rel := range present {
		local := filepath.Join(f.Dir, filepath.FromSlash(rel))
		digest, err := card.FileDigest(local)
		if err != nil {
			outcomes = append(outcomes, Outcome{Path: rel, Err: err.Error()})
			continue
		}
		remotePath := ""
		if caps.RelativePaths {
			remotePath = path.Join(folder, strings.TrimPrefix(rel, card.AssetsDir+"/"))
		}
		prev, had := f.State.Assets[rel]
		// Compare served URLs, not names: a library rename changes the shown
		// name but not the path a file is served from.
		want := ""
		if remotePath != "" && caps.LibraryPrefix != "" {
			want = caps.LibraryPrefix + "/" + escapePath(remotePath)
		}
		moved := had && want != "" && prev.FileName != "" && prev.URL != "" && prev.URL != want
		if had && prev.SHA256 == digest && prev.URL != "" && !moved {
			urls[rel] = prev.URL
			outcomes = append(outcomes, Outcome{Path: rel, URL: prev.URL})
			continue
		}
		previous := ""
		if moved {
			previous = prev.URL
		}
		if caps.MaxFileBytes > 0 {
			if st, err := os.Stat(local); err == nil && st.Size() > caps.MaxFileBytes {
				outcomes = append(outcomes, Outcome{Path: rel, Err: fmt.Sprintf("larger than the provider limit (%d bytes)", caps.MaxFileBytes)})
				continue
			}
		}
		if dryRun {
			outcomes = append(outcomes, Outcome{Path: rel, Uploaded: true, Previous: previous})
			continue
		}
		res, err := Upload(ctx, c, local, remotePath, "")
		if err != nil {
			outcomes = append(outcomes, Outcome{Path: rel, Err: err.Error()})
			continue
		}
		// For named uploads the provider returns the stable library URL
		// (<libraryPrefix>/<relativePath>) as imageUrl; for unnamed ones it
		// returns the object URL. Either is what the card should reference.
		served := res.ImageURL
		id := ""
		if res.ImageID != nil {
			id = fmt.Sprint(res.ImageID)
		}
		f.State.Assets[rel] = card.Asset{SHA256: digest, URL: served, FileName: res.FileName, ID: id}
		urls[rel] = served
		outcomes = append(outcomes, Outcome{Path: rel, URL: served, Uploaded: true, Replaced: res.Replaced, Previous: previous})
	}
	// A referenced directory is served at the URL its files share, so card
	// code can append a file name at runtime.
	for _, dir := range f.AssetDirs() {
		if !caps.RelativePaths {
			outcomes = append(outcomes, Outcome{Path: dir, Err: "this provider cannot serve files by folder path; reference each file literally"})
			continue
		}
		for rel, u := range urls {
			if tail := escapePath(strings.TrimPrefix(rel, dir)); strings.HasPrefix(rel, dir) && strings.HasSuffix(u, "/"+tail) {
				urls[dir] = strings.TrimSuffix(u, tail)
				break
			}
		}
	}
	if !dryRun && caps.RelativePaths {
		f.State.AssetFolder = folder
	}
	return urls, outcomes, nil
}
