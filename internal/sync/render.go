package sync

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/hearthroom/cli/internal/api"
)

// RenderReport is GET /open/v1/role/render: one opening after the card's
// display rules, as the player's renderer receives it, plus a static scan.
type RenderReport struct {
	RoleID      string        `json:"roleId"`
	Opening     RenderOpening `json:"opening"`
	Names       RenderNames   `json:"names"`
	Source      string        `json:"source"`
	Rendered    string        `json:"rendered"`
	AuthorAsset *RenderAsset  `json:"authorAsset"`
	Rules       []RenderRule  `json:"rules"`
	Report      RenderScan    `json:"report"`
	Warnings    []string      `json:"warnings"`
	Capture     RenderCapture `json:"capture"`
	PreviewURL  string        `json:"previewUrl,omitempty"` // filled in by the CLI, not the provider
}

type RenderOpening struct {
	Index int `json:"index"`
	Count int `json:"count"`
}

type RenderNames struct {
	Player    string `json:"player"`
	Character string `json:"character"`
}

type RenderAsset struct {
	Version      int64  `json:"version"`
	PageMode     string `json:"pageMode"`
	MountLayer   string `json:"mountLayer"`
	MountTrigger string `json:"mountTrigger"`
	CardFormat   string `json:"cardFormat"`
	Rules        int    `json:"rules"`
	Enabled      int    `json:"enabled"`
}

type RenderRule struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"` // applied | unmatched | disabled | empty | rolled_back
	Reason string `json:"reason,omitempty"`
}

type RenderScan struct {
	Chars           int                 `json:"chars"`
	EstimatedTokens int                 `json:"estimatedTokens"`
	Tags            map[string]int      `json:"tags"`
	Components      []string            `json:"components"`
	Scripts         int                 `json:"scripts"`
	Styles          int                 `json:"styles"`
	InlineHandlers  int                 `json:"inlineHandlers"`
	ExternalURLs    []string            `json:"externalUrls"`
	Unsupported     []RenderUnsupported `json:"unsupported"`
	CrossLineRules  bool                `json:"crossLineRules"`
}

type RenderUnsupported struct {
	API   string   `json:"api"`
	Count int      `json:"count"`
	Where []string `json:"where"`
	Hint  string   `json:"hint"`
}

type RenderCapture struct {
	ServerCaptureAvailable bool   `json:"serverCaptureAvailable"`
	Recommended            string `json:"recommended"`
}

// Render asks the provider to run the display rules over one opening of the
// pushed card. opening 0 is the main opening; N is the Nth alternate.
func Render(ctx context.Context, c *api.Client, roleID string, opening int) (*RenderReport, error) {
	q := url.Values{"roleId": {roleID}}
	if opening > 0 {
		q.Set("opening", strconv.Itoa(opening))
	}
	var r RenderReport
	if err := c.OpenGet(ctx, "/role/render", q, &r); err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	if r.Rules == nil {
		r.Rules = []RenderRule{}
	}
	if r.Warnings == nil {
		r.Warnings = []string{}
	}
	if r.Report.Unsupported == nil {
		r.Report.Unsupported = []RenderUnsupported{}
	}
	return &r, nil
}

// PreviewURL is where the community site plays the pushed card; the same
// page the web editor's test panel opens. Site is the community site base.
func PreviewURL(site, roleID string) string {
	return site + "/play/" + url.PathEscape(roleID)
}
