package card

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Section names as the provider's trial-card contract spells them.
const (
	SectionCard        = "card"
	SectionWelcome     = "welcome"
	SectionWorldbook   = "worldbook"
	SectionAuthorAsset = "authorAsset"
	// SectionState is the card's variables (variables.json).
	SectionState = "state"
)

// Sections is the stable order of sections.
var Sections = []string{SectionCard, SectionWelcome, SectionWorldbook, SectionAuthorAsset, SectionState}

// Payload is the trial-card document built from a folder. Each section is
// nil when the folder has nothing for it.
type Payload struct {
	Name        string
	Card        map[string]any
	Welcome     map[string]any
	Worldbook   map[string]any
	AuthorAsset map[string]any
	State       map[string]any
	// StateRemoved: variables.json is empty, so the card's variables are removed.
	StateRemoved bool
}

// stateRemovedDigest stands for "no variables" so a removal counts as a change once.
var stateRemovedDigest = Digest(map[string]any{"removed": true})

// Section returns the section body by name (nil when absent).
func (p Payload) Section(name string) map[string]any {
	switch name {
	case SectionCard:
		return p.Card
	case SectionWelcome:
		return p.Welcome
	case SectionWorldbook:
		return p.Worldbook
	case SectionAuthorAsset:
		return p.AuthorAsset
	case SectionState:
		return p.State
	}
	return nil
}

// Digests hashes every present section for change detection.
func (p Payload) Digests() map[string]string {
	out := map[string]string{}
	for _, s := range Sections {
		if body := p.Section(s); body != nil {
			out[s] = Digest(body)
		}
	}
	if p.StateRemoved {
		out[SectionState] = stateRemovedDigest
	}
	return out
}

// Build converts the folder into API shapes. urls maps relative asset paths
// to served URLs and is applied to media fields and text references.
func (f *Folder) Build(urls map[string]string) (Payload, error) {
	m := f.Manifest
	name := strings.TrimSpace(m.Name)
	if name == "" {
		return Payload{}, fmt.Errorf("%s: name is required", ManifestFile)
	}
	p := Payload{Name: name}

	fields := map[string]any{"roleName": name}
	if v := strings.TrimSpace(m.Summary); v != "" {
		fields["roleDesc"] = v
	}
	if m.Tags != nil {
		fields["roleTag"] = m.Tags
	}
	if m.Type != "" {
		fields["roleType"] = m.Type
	}
	if m.Sex != "" {
		fields["roleSex"] = m.Sex
	}
	if m.PlayerName != "" {
		fields["userName"] = m.PlayerName
	}
	if m.Nickname != "" {
		fields["nickname"] = m.Nickname
	}
	if v := Body(f.Definition); v != "" {
		fields["roleDetailDesc"] = Rewrite(v, urls)
	}
	// Sent only when intro.md exists: an empty file clears the card page
	// introduction, a missing one leaves it (and the digest) alone.
	if f.Intro != nil {
		fields["roleReadme"] = Rewrite(Body(*f.Intro), urls)
	}
	if v := strings.TrimSpace(m.OutputContract); v != "" {
		fields["roleOutputContract"] = v
	}
	if v := strings.TrimSpace(m.CustomInstructions); v != "" {
		fields["customInstructions"] = Rewrite(v, urls)
	}
	if len(m.TalkExample) > 0 {
		ex := make([]map[string]string, 0, len(m.TalkExample))
		for _, t := range m.TalkExample {
			if strings.TrimSpace(t.Content) == "" {
				continue
			}
			role := t.RoleType
			if role == "" {
				role = "user"
			}
			ex = append(ex, map[string]string{"roleType": role, "content": t.Content})
		}
		fields["talkExample"] = ex
	}
	if len(m.CardMeta) > 0 && string(m.CardMeta) != "null" {
		var meta any
		if err := json.Unmarshal(m.CardMeta, &meta); err != nil {
			return Payload{}, fmt.Errorf("%s: cardMeta: %w", ManifestFile, err)
		}
		fields["cardMeta"] = meta
	}
	// Sent only when card.json has the key: an empty object clears the server's
	// defaults, an absent key leaves them alone (and leaves the digest unchanged).
	if m.ResponseDefaults != nil {
		fields["responseDefaults"] = map[string]string(m.ResponseDefaults)
	}
	if u := resolveMedia(m.Media.Portrait, urls); u != "" {
		fields["roleAvatar"] = u
	}
	if u := resolveMedia(m.Media.Background, urls); u != "" {
		fields["roleBackground"] = u
	}
	if u := resolveMedia(m.Media.BackgroundLandscape, urls); u != "" {
		fields["roleBackgroundLandscape"] = u
	}
	if u := resolveMedia(m.Media.Share, urls); u != "" {
		fields["roleShareImage"] = u
	}
	p.Card = fields

	welcome := map[string]any{}
	if f.WelcomePresent || f.Welcome != "" {
		welcome["roleWelcome"] = Rewrite(Body(f.Welcome), urls)
	}
	if f.Alternates != nil {
		alts := make([]string, 0, len(f.Alternates))
		for _, o := range f.Alternates {
			if t := Body(o.Text); t != "" {
				alts = append(alts, Rewrite(t, urls))
			}
		}
		welcome["alternates"] = alts
	}
	if m.Prologue != nil {
		pro := make([]string, 0, len(m.Prologue))
		for _, s := range m.Prologue {
			if strings.TrimSpace(s) != "" {
				pro = append(pro, s)
			}
		}
		welcome["prologue"] = pro
	}
	if len(welcome) > 0 {
		p.Welcome = welcome
	}

	if f.Lorebook != nil {
		entries := make([]map[string]any, 0, len(f.Lorebook.Entries))
		for i, e := range f.Lorebook.Entries {
			if strings.TrimSpace(e.Content) == "" {
				return Payload{}, fmt.Errorf("lorebook.json: entry %d (%q) has empty content; restore its content or remove the entry explicitly", i+1, e.Name)
			}
			entry := map[string]any{
				"name":      e.Name,
				"content":   Rewrite(e.Content, urls),
				"keywords":  nonNil(e.Keywords),
				"isEnabled": !e.Disabled,
			}
			if e.SemanticRecall != nil {
				entry["semanticRecall"] = *e.SemanticRecall
			}
			if len(e.SecondaryKeywords) > 0 {
				entry["secondaryKeywords"] = e.SecondaryKeywords
			}
			if e.Category != "" {
				entry["category"] = e.Category
			}
			if e.Constant {
				entry["isConstant"] = true
			}
			if e.TriggerRegion != "" {
				entry["triggerRegion"] = e.TriggerRegion
			}
			if len(e.MatchOptions) > 0 && string(e.MatchOptions) != "null" {
				var mo any
				if err := json.Unmarshal(e.MatchOptions, &mo); err != nil {
					return Payload{}, fmt.Errorf("%s: entry %q matchOptions: %w", LorebookFile, e.Name, err)
				}
				entry["matchOptions"] = mo
			}
			entries = append(entries, entry)
		}
		wb := map[string]any{"entries": entries}
		if n := strings.TrimSpace(f.Lorebook.Name); n != "" {
			wb["name"] = n
		}
		if f.Lorebook.Format != "" {
			wb["format"] = f.Lorebook.Format
		}
		p.Worldbook = wb
	}

	if f.Rules != nil || f.CardCSS != nil {
		assetRules := f.Rules
		if assetRules == nil {
			assetRules = &Rules{}
		}
		rules := make([]map[string]any, 0, len(assetRules.Rules))
		for i, r := range assetRules.Rules {
			id := r.ID
			if id == "" {
				id = fmt.Sprintf("rule-%02d", i+1)
			}
			rule := map[string]any{
				"id": id, "name": r.Name, "find": r.Find, "replace": Rewrite(r.Replace, urls), "enabled": r.Enabled,
			}
			if r.Targets != nil {
				rule["targets"] = *r.Targets
			}
			if r.CSSDialect != "" {
				rule["cssDialect"] = r.CSSDialect
			}
			rules = append(rules, rule)
		}
		layer := assetRules.MountLayer
		if layer == "" {
			layer = "over"
		}
		aa := map[string]any{"rules": rules, "mountLayer": layer, "mountTrigger": assetRules.MountTrigger}
		if f.CardCSS != nil {
			aa["cardCss"] = Rewrite(*f.CardCSS, urls)
		}
		if assetRules.CardCSSDialect != nil {
			aa["cardCssDialect"] = *assetRules.CardCSSDialect
		}
		if assetRules.PageMode != "" {
			aa["pageMode"] = assetRules.PageMode
		}
		if assetRules.CardFormat != "" {
			aa["cardFormat"] = assetRules.CardFormat
		}
		// Sent only when written, so folders without it keep their digests and the site's choice.
		if assetRules.OpeningMode != "" {
			aa["openingMode"] = assetRules.OpeningMode
		}
		// Sent only when the file has one, so cards without a declaration keep their digests; the
		// server keeps an omitted declaration, so clearing it means writing "sharedState": {}.
		if len(assetRules.SharedState) > 0 {
			aa["sharedState"] = assetRules.SharedState
		}
		// Same rule as sharedState: sent only when opening.html exists (an empty file clears the page),
		// so folders without one keep their digests and the stored page.
		if f.OpeningPage != nil {
			aa["openingPage"] = Rewrite(*f.OpeningPage, urls)
		}
		p.AuthorAsset = aa
	}

	if v := f.Variables; v != nil {
		if v.Empty {
			p.StateRemoved = true
		} else if body, ok := v.Body(urls); ok {
			p.State = body
		} else {
			return Payload{}, fmt.Errorf("%s: data is missing; add the starting values as a JSON object", VariablesFile)
		}
	}
	return p, nil
}

func resolveMedia(ref string, urls map[string]string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if u, ok := urls[ref]; ok {
		return u
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	return ""
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// RemoteDetail is the subset of GET /open/v1/role/detail the folder uses.
type RemoteDetail struct {
	RoleID                  string            `json:"characterRoleId"`
	RoleName                string            `json:"roleName"`
	RoleDesc                string            `json:"roleDesc"`
	RoleTag                 []string          `json:"roleTag"`
	RoleType                string            `json:"roleType"`
	RoleSex                 string            `json:"roleSex"`
	UserName                string            `json:"userName"`
	Nickname                string            `json:"nickname"`
	RoleAvatar              string            `json:"roleAvatar"`
	RoleBackground          string            `json:"roleBackground"`
	RoleBackgroundLandscape string            `json:"roleBackgroundLandscape"`
	RoleShareImage          string            `json:"roleShareImage"`
	RoleDetailDesc          string            `json:"roleDetailDesc"`
	RoleReadme              string            `json:"roleReadme"`
	RoleWelcome             string            `json:"roleWelcome"`
	RoleWelcomeAlts         []string          `json:"roleWelcomeAlternates"`
	RolePrologue            []string          `json:"rolePrologue"`
	TalkExample             []TalkExample     `json:"talkExample"`
	RoleOutputContract      string            `json:"roleOutputContract"`
	CustomInstructions      string            `json:"customInstructions"`
	CardMeta                json.RawMessage   `json:"cardMeta"`
	ResponseDefaults        map[string]string `json:"responseDefaults"`
	Language                string            `json:"language"`
	Visibility              string            `json:"roleVisibility"`
	ReviewStatus            string            `json:"reviewStatus"`
	CreationMethod          string            `json:"creationMethod"`
	State                   *RemoteVariables  `json:"state"`
}

// RemoteEntry is one Lorebook entry from GET /open/v1/worldbook/entry/list.
type RemoteEntry struct {
	SemanticRecall    *bool           `json:"semanticRecall,omitempty"`
	EntryID           string          `json:"entryId"`
	Name              string          `json:"name"`
	Content           string          `json:"content"`
	Category          string          `json:"category"`
	Keywords          []string        `json:"keywords"`
	SecondaryKeywords []string        `json:"secondaryKeywords"`
	IsConstant        bool            `json:"isConstant"`
	IsEnabled         bool            `json:"isEnabled"`
	TriggerRegion     string          `json:"triggerRegion"`
	MatchOptions      json.RawMessage `json:"matchOptions"`
	SortOrder         int             `json:"sortOrder"`
	Priority          int             `json:"priority"`
}

// RemoteAsset is GET /open/v1/role/author-asset.
type RemoteAsset struct {
	Rules          []DisplayRule   `json:"rules"`
	MountTrigger   string          `json:"mountTrigger"`
	MountLayer     string          `json:"mountLayer"`
	PageMode       string          `json:"pageMode"`
	CardFormat     string          `json:"cardFormat"`
	SharedState    json.RawMessage `json:"sharedState"`
	OpeningPage    string          `json:"openingPage"`
	OpeningMode    string          `json:"openingMode"`
	CardCSS        *string         `json:"cardCss"`
	CardCSSDialect *string         `json:"cardCssDialect"`
	Status         string          `json:"status"`
	Version        int64           `json:"version"`
}

// FromRemote builds a folder from provider responses. lorebook may be nil.
func FromRemote(dir string, d RemoteDetail, lorebookID, lorebookName string, entries []RemoteEntry, asset *RemoteAsset) *Folder {
	f := &Folder{Dir: dir}
	f.Manifest = Manifest{
		FormatVersion:      FormatVersion,
		Name:               d.RoleName,
		Summary:            d.RoleDesc,
		Tags:               d.RoleTag,
		Type:               d.RoleType,
		Sex:                d.RoleSex,
		PlayerName:         d.UserName,
		Nickname:           d.Nickname,
		OutputContract:     d.RoleOutputContract,
		CustomInstructions: d.CustomInstructions,
		TalkExample:        d.TalkExample,
		Prologue:           d.RolePrologue,
		Language:           d.Language,
		Media:              Media{Portrait: d.RoleAvatar, Background: d.RoleBackground, BackgroundLandscape: d.RoleBackgroundLandscape, Share: d.RoleShareImage},
	}
	if d.RoleBackground == d.RoleAvatar {
		f.Manifest.Media.Background = ""
	}
	if len(d.CardMeta) > 0 && string(d.CardMeta) != "null" && string(d.CardMeta) != "{}" {
		f.Manifest.CardMeta = d.CardMeta
	}
	if len(d.ResponseDefaults) > 0 {
		f.Manifest.ResponseDefaults = ResponseDefaults(d.ResponseDefaults)
	}
	f.Definition = d.RoleDetailDesc
	if strings.TrimSpace(d.RoleReadme) != "" {
		readme := d.RoleReadme
		f.Intro = &readme
	}
	f.Welcome = d.RoleWelcome
	f.WelcomePresent = true
	f.Alternates = []Opening{}
	f.Manifest.Prologue = nonNil(d.RolePrologue)
	for i, a := range d.RoleWelcomeAlts {
		f.Alternates = append(f.Alternates, Opening{File: fmt.Sprintf("%s/alt-%02d.md", OpeningsDir, i+1), Text: a})
	}
	if lorebookID != "" {
		lb := &Lorebook{ID: lorebookID, Name: lorebookName, Entries: []LorebookEntry{}}
		for _, e := range entries {
			lb.Entries = append(lb.Entries, LorebookEntry{
				ID: e.EntryID, Name: e.Name, Content: e.Content, Category: e.Category,
				Keywords: nonNil(e.Keywords), SecondaryKeywords: e.SecondaryKeywords,
				SemanticRecall: e.SemanticRecall, Constant: e.IsConstant, Disabled: !e.IsEnabled, TriggerRegion: e.TriggerRegion,
				MatchOptions: nullToNil(e.MatchOptions),
			})
		}
		f.Lorebook = lb
		for _, e := range entries {
			f.State.LorebookEntryIDs = append(f.State.LorebookEntryIDs, e.EntryID)
		}
	}
	shared := declaredSharedState(asset)
	// The opening mode is written only when it differs from the card format's default (AI Fengyue cards
	// open on the page, others on the message): other folders stay as they were, and pushing this one
	// to a new card, where the site picks the default from the format, keeps the author's choice.
	openingMode := ""
	if asset != nil && (asset.OpeningMode == "page" || asset.OpeningMode == "greeting") {
		def := "greeting"
		if asset.CardFormat == "fengyue" {
			def = "page"
		}
		if asset.OpeningMode != def {
			openingMode = asset.OpeningMode
		}
	}
	if asset != nil && (len(asset.Rules) > 0 || asset.MountTrigger != "" || asset.PageMode != "" && asset.PageMode != "classic" || shared != nil || strings.TrimSpace(asset.OpeningPage) != "" || openingMode != "" || asset.CardCSS != nil || asset.CardCSSDialect != nil) {
		f.Rules = &Rules{Rules: asset.Rules, MountTrigger: asset.MountTrigger, MountLayer: asset.MountLayer, PageMode: asset.PageMode, CardFormat: asset.CardFormat, OpeningMode: openingMode, SharedState: shared, CardCSSDialect: asset.CardCSSDialect}
		f.CardCSS = asset.CardCSS
		f.migrateLegacyCardCSS()
		if strings.TrimSpace(asset.OpeningPage) != "" {
			page := asset.OpeningPage
			f.OpeningPage = &page
		}
		f.State.AuthorAssetVersion = asset.Version
	}
	if d.State != nil {
		// A state the CLI cannot read back is left out rather than written wrong.
		if v, err := RebuildVariables(*d.State); err == nil {
			f.Variables = v
		}
	}
	f.State.RoleID = d.RoleID
	f.State.LorebookID = lorebookID
	if d.CreationMethod == "trial" {
		f.State.Target = "trial"
	} else {
		f.State.Target = "owned"
	}
	return f
}

func nullToNil(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return raw
}

// declaredSharedState is the server's declaration, or nil when nothing is declared ({} or absent),
// so pulled rules.json files only carry the field when it means something.
func declaredSharedState(asset *RemoteAsset) json.RawMessage {
	if asset == nil {
		return nil
	}
	s := strings.TrimSpace(string(asset.SharedState))
	if s == "" || s == "{}" || s == "null" {
		return nil
	}
	return asset.SharedState
}
