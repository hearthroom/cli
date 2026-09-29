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
	"strings"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

// Capabilities is the subset of GET /open/v1/image/list the CLI relies on.
type Capabilities struct {
	RelativePaths bool
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
	Err      string `json:"error,omitempty"`
}

// Sync uploads every referenced asset of the folder that is new or changed,
// records it in the folder state and returns the path→URL map. prefix is the
// remote folder name for relative paths (usually the card key). Failures are
// reported per file; the map still contains every asset that has a URL.
func Sync(ctx context.Context, c *api.Client, f *card.Folder, caps Capabilities, prefix string, dryRun bool) (map[string]string, []Outcome, error) {
	present, missing := f.AssetRefs()
	urls := map[string]string{}
	var outcomes []Outcome
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
		if prev, ok := f.State.Assets[rel]; ok && prev.SHA256 == digest && prev.URL != "" {
			urls[rel] = prev.URL
			outcomes = append(outcomes, Outcome{Path: rel, URL: prev.URL})
			continue
		}
		if caps.MaxFileBytes > 0 {
			if st, err := os.Stat(local); err == nil && st.Size() > caps.MaxFileBytes {
				outcomes = append(outcomes, Outcome{Path: rel, Err: fmt.Sprintf("larger than the provider limit (%d bytes)", caps.MaxFileBytes)})
				continue
			}
		}
		if dryRun {
			outcomes = append(outcomes, Outcome{Path: rel, Uploaded: true})
			continue
		}
		remotePath := ""
		if caps.RelativePaths {
			remotePath = path.Join(prefix, strings.TrimPrefix(rel, card.AssetsDir+"/"))
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
		f.State.Assets[rel] = card.Asset{SHA256: digest, URL: served, FileName: res.FileName}
		urls[rel] = served
		outcomes = append(outcomes, Outcome{Path: rel, URL: served, Uploaded: true, Replaced: res.Replaced})
	}
	return urls, outcomes, nil
}
