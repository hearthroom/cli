package sync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

// PullOptions control a pull.
type PullOptions struct {
	// Download fetches referenced media into assets/ and rewrites references.
	Download bool
	// Force allows writing into a folder that already has a card.json.
	Force bool
}

// PullResult reports what was written.
type PullResult struct {
	RoleID     string   `json:"roleId"`
	Dir        string   `json:"dir"`
	Files      []string `json:"files"`
	Lorebook   string   `json:"lorebookId,omitempty"`
	Downloaded []string `json:"downloaded,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
}

// Pull reads an owned card into dir.
func Pull(ctx context.Context, c *api.Client, roleID, dir string, opts PullOptions) (*PullResult, error) {
	if _, err := os.Stat(filepath.Join(dir, card.ManifestFile)); err == nil && !opts.Force {
		return nil, fmt.Errorf("%s already contains a card; pass --force to overwrite its files", dir)
	}
	var d card.RemoteDetail
	if err := c.OpenGet(ctx, "/role/detail", url.Values{"roleId": {roleID}}, &d); err != nil {
		return nil, fmt.Errorf("read card: %w", err)
	}
	if d.RoleID == "" {
		d.RoleID = roleID
	}
	res := &PullResult{RoleID: roleID, Dir: dir}

	var b bindingsOutput
	bookID, bookName := "", ""
	var entries []card.RemoteEntry
	if err := c.OpenGet(ctx, "/worldbook/bindings", url.Values{"roleId": {roleID}}, &b); err != nil {
		res.Warnings = append(res.Warnings, "Lorebook bindings could not be read: "+err.Error())
	} else if len(b.Bindings) > 0 {
		bookID, bookName = b.Bindings[0].WorldbookID, b.Bindings[0].Name
		var el entryListOutput
		if err := c.OpenGet(ctx, "/worldbook/entry/list", url.Values{"worldbookId": {bookID}}, &el); err != nil {
			res.Warnings = append(res.Warnings, "Lorebook entries could not be read: "+err.Error())
		} else {
			entries = el.Entries
		}
		if len(b.Bindings) > 1 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("card has %d bound Lorebooks; only the first was pulled", len(b.Bindings)))
		}
	}
	var asset *card.RemoteAsset
	var ra card.RemoteAsset
	if err := c.OpenGet(ctx, "/role/author-asset", url.Values{"roleId": {roleID}}, &ra); err != nil {
		if !api.IsStatus(err, 404) {
			res.Warnings = append(res.Warnings, "display rules could not be read: "+err.Error())
		}
	} else if ra.Status != "none" || len(ra.Rules) > 0 {
		asset = &ra
	}

	f := card.FromRemote(dir, d, bookID, bookName, entries, asset)
	f.State.API = c.API
	res.Lorebook = bookID

	if opts.Download {
		if err := os.MkdirAll(filepath.Join(dir, card.AssetsDir), 0o755); err != nil {
			return nil, err
		}
		urls := map[string]string{}
		fetch := func(ref *string, fallback string) {
			u := strings.TrimSpace(*ref)
			if u == "" || !strings.HasPrefix(u, "http") {
				return
			}
			if rel, ok := urls[u]; ok {
				*ref = rel
				return
			}
			name := fileNameFor(u, fallback)
			rel := card.AssetsDir + "/" + name
			if err := download(ctx, c, u, filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("download %s: %v", u, err))
				return
			}
			digest, _ := card.FileDigest(filepath.Join(dir, filepath.FromSlash(rel)))
			if f.State.Assets == nil {
				f.State.Assets = map[string]card.Asset{}
			}
			f.State.Assets[rel] = card.Asset{SHA256: digest, URL: u}
			urls[u] = rel
			res.Downloaded = append(res.Downloaded, rel)
			*ref = rel
		}
		fetch(&f.Manifest.Media.Portrait, "portrait")
		fetch(&f.Manifest.Media.Background, "background")
	}

	if err := f.Save(); err != nil {
		return nil, err
	}
	res.Files = []string{card.ManifestFile, card.DefinitionFile, card.WelcomeFile}
	for _, o := range f.Alternates {
		res.Files = append(res.Files, o.File)
	}
	if f.Lorebook != nil {
		res.Files = append(res.Files, card.LorebookFile)
	}
	if f.Rules != nil {
		res.Files = append(res.Files, card.RulesFile)
	}
	return res, nil
}

func fileNameFor(rawURL, fallback string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fallback
	}
	name := path.Base(u.Path)
	if name == "" || name == "." || name == "/" {
		return fallback
	}
	if unescaped, err := url.PathUnescape(name); err == nil {
		name = unescaped
	}
	return name
}

func download(ctx context.Context, c *api.Client, rawURL, dest string) error {
	body, _, err := c.Download(ctx, rawURL)
	if err != nil {
		return err
	}
	defer body.Close()
	tmp := dest + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, body); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

// ErrNoTarget is returned when a folder was never pushed.
var ErrNoTarget = errors.New("this folder has not been pushed yet; run `hearthroom card push` first")
