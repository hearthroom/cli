package sync

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

// openingPageHashKey records, in LocalHashes, the digest of the large opening page last uploaded
// with the raw page upload, so a push whose page did not change does not upload it again.
const openingPageHashKey = "openingPage"

// splitLargePage copies an author asset for the wire. A page above the inline size is left out —
// the provider keeps its stored page when the field is absent — and returned for the raw upload.
func splitLargePage(aa map[string]any) (map[string]any, *string) {
	body := make(map[string]any, len(aa)+1)
	for k, v := range aa {
		body[k] = v
	}
	if page, ok := body["openingPage"].(string); ok && len(page) > openingPageInlineMax {
		delete(body, "openingPage")
		return body, &page
	}
	return body, nil
}

// wireAuthorAsset is the author asset as push sends it. An empty opening.html, or a removed one
// whose page this folder pushed or pulled, sends openingPageClear: "" alone keeps a stored page
// above the inline size. A provider without the flag ignores it and clears with "" as before.
func wireAuthorAsset(f *card.Folder, aa map[string]any) (map[string]any, *string) {
	body, large := splitLargePage(aa)
	if page, ok := aa["openingPage"].(string); ok {
		if strings.TrimSpace(page) == "" {
			// The provider refuses the flag beside a non-empty page, so whitespace goes as "".
			body["openingPage"] = ""
			body["openingPageClear"] = true
		}
	} else if f.State.OpeningPageSent {
		body["openingPageClear"] = true
	}
	return body, large
}

// noteOpeningPage records what the card holds after a successful save of body (and of large).
func noteOpeningPage(f *card.Folder, aa, body map[string]any, large *string) {
	if large != nil {
		f.State.LocalHashes[openingPageHashKey] = card.Digest(*large)
	} else {
		delete(f.State.LocalHashes, openingPageHashKey)
	}
	if page, ok := aa["openingPage"].(string); ok {
		f.State.OpeningPageSent = strings.TrimSpace(page) != ""
	} else if body["openingPageClear"] == true {
		f.State.OpeningPageSent = false
	}
}

// largePageChanged: the large page differs from the one last uploaded to this card.
func largePageChanged(f *card.Folder, large *string, force bool) bool {
	return large != nil && (force || f.State.LocalHashes[openingPageHashKey] != card.Digest(*large))
}

// putOpeningPage sends a large page with the raw upload under the author asset's version lock and
// returns the version it produced. A conflict re-reads the version and tries once more, as the
// author asset save does.
func putOpeningPage(ctx context.Context, c *api.Client, roleID string, version int64, page string) (int64, error) {
	var out struct {
		Version int64 `json:"version"`
	}
	put := func(v int64) error {
		return c.OpenPutRaw(ctx, "/role/"+url.PathEscape(roleID)+"/opening-page", url.Values{"version": {strconv.FormatInt(v, 10)}},
			"text/html; charset=utf-8", strings.NewReader(page), int64(len(page)), &out)
	}
	err := put(version)
	if api.IsStatus(err, 409) {
		var cur card.RemoteAsset
		if rerr := c.OpenGet(ctx, "/role/author-asset", url.Values{"roleId": {roleID}}, &cur); rerr == nil {
			version = cur.Version
			err = put(version)
		}
	}
	if err != nil {
		return 0, fmt.Errorf("upload %s (%d bytes): %w", card.OpeningFile, len(page), openingPageRejected(err))
	}
	if out.Version == 0 {
		out.Version = version + 1
	}
	return out.Version, nil
}

// readOpeningPage fetches a page the provider did not inline. On failure it returns a warning and
// pull keeps the local opening.html rather than writing an empty one.
func readOpeningPage(ctx context.Context, c *api.Client, ra card.RemoteAsset) (string, string) {
	keep := fmt.Sprintf("%s was left as it was", card.OpeningFile)
	if ra.OpeningPageExpired {
		return "", fmt.Sprintf("the card's opening page (%d bytes) has expired on the provider and cannot be read; %s", ra.OpeningPageBytes, keep)
	}
	if ra.OpeningPageURL == "" {
		return "", fmt.Sprintf("the provider reported a %d-byte opening page without a link to read it; %s", ra.OpeningPageBytes, keep)
	}
	body, err := c.OpenStream(ctx, ra.OpeningPageURL)
	if api.IsStatus(err, 410) {
		return "", fmt.Sprintf("the card's opening page (%d bytes) has expired on the provider and cannot be read; %s", ra.OpeningPageBytes, keep)
	}
	if err != nil {
		return "", fmt.Sprintf("the card's opening page could not be read (%v); %s", err, keep)
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, ra.OpeningPageBytes+1))
	switch {
	case err != nil:
		return "", fmt.Sprintf("the card's opening page could not be read (%v); %s", err, keep)
	case int64(len(raw)) != ra.OpeningPageBytes:
		return "", fmt.Sprintf("the card's opening page arrived as %d bytes, not the %d the provider reported; %s", len(raw), ra.OpeningPageBytes, keep)
	case !utf8.Valid(raw):
		return "", fmt.Sprintf("the card's opening page is not valid UTF-8; %s", keep)
	}
	return string(raw), ""
}
