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
)

// Sections is the stable order of sections.
var Sections = []string{SectionCard, SectionWelcome, SectionWorldbook, SectionAuthorAsset}

// Payload is the trial-card document built from a folder. Each section is
// nil when the folder has nothing for it.
type Payload struct {
	Name        string
	Card        map[string]any
	Welcome     map[string]any
	Worldbook   map[string]any
	AuthorAsset map[string]any
}

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
	if u := resolveMedia(m.Media.Portrait, urls); u != "" {
		fields["roleAvatar"] = u
	}
	if u := resolveMedia(m.Media.Background, urls); u != "" {
		fields["roleBackground"] = u
	}
	p.Card = fields

	if w := Body(f.Welcome); w != "" || len(f.Alternates) > 0 || len(m.Prologue) > 0 {
		welcome := map[string]any{"roleWelcome": Rewrite(w, urls)}
		alts := make([]string, 0, len(f.Alternates))
		for _, o := range f.Alternates {
			if t := Body(o.Text); t != "" {
				alts = append(alts, Rewrite(t, urls))
			}
		}
		welcome["alternates"] = alts
		pro := make([]string, 0, len(m.Prologue))
		for _, s := range m.Prologue {
			if strings.TrimSpace(s) != "" {
				pro = append(pro, s)
			}
		}
		welcome["prologue"] = pro
		p.Welcome = welcome
	}

	if f.Lorebook != nil {
		entries := make([]map[string]any, 0, len(f.Lorebook.Entries))
		for _, e := range f.Lorebook.Entries {
			if strings.TrimSpace(e.Content) == "" || e.Disabled {
				continue
			}
			entry := map[string]any{
				"name":     e.Name,
				"content":  Rewrite(e.Content, urls),
				"keywords": nonNil(e.Keywords),
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

	if f.Rules != nil {
		rules := make([]map[string]any, 0, len(f.Rules.Rules))
		for i, r := range f.Rules.Rules {
			id := r.ID
			if id == "" {
				id = fmt.Sprintf("rule-%02d", i+1)
			}
			rules = append(rules, map[string]any{
				"id": id, "name": r.Name, "find": r.Find, "replace": Rewrite(r.Replace, urls), "enabled": r.Enabled,
			})
		}
		layer := f.Rules.MountLayer
		if layer == "" {
			layer = "under"
		}
		aa := map[string]any{"rules": rules, "mountLayer": layer, "mountTrigger": f.Rules.MountTrigger}
		if f.Rules.PageMode != "" {
			aa["pageMode"] = f.Rules.PageMode
		}
		p.AuthorAsset = aa
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
	RoleID             string          `json:"characterRoleId"`
	RoleName           string          `json:"roleName"`
	RoleDesc           string          `json:"roleDesc"`
	RoleTag            []string        `json:"roleTag"`
	RoleType           string          `json:"roleType"`
	RoleSex            string          `json:"roleSex"`
	UserName           string          `json:"userName"`
	Nickname           string          `json:"nickname"`
	RoleAvatar         string          `json:"roleAvatar"`
	RoleBackground     string          `json:"roleBackground"`
	RoleDetailDesc     string          `json:"roleDetailDesc"`
	RoleWelcome        string          `json:"roleWelcome"`
	RoleWelcomeAlts    []string        `json:"roleWelcomeAlternates"`
	RolePrologue       []string        `json:"rolePrologue"`
	TalkExample        []TalkExample   `json:"talkExample"`
	RoleOutputContract string          `json:"roleOutputContract"`
	CustomInstructions string          `json:"customInstructions"`
	CardMeta           json.RawMessage `json:"cardMeta"`
	Language           string          `json:"language"`
	Visibility         string          `json:"roleVisibility"`
	ReviewStatus       string          `json:"reviewStatus"`
	CreationMethod     string          `json:"creationMethod"`
}

// RemoteEntry is one Lorebook entry from GET /open/v1/worldbook/entry/list.
type RemoteEntry struct {
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
	Rules        []DisplayRule `json:"rules"`
	MountTrigger string        `json:"mountTrigger"`
	MountLayer   string        `json:"mountLayer"`
	PageMode     string        `json:"pageMode"`
	Status       string        `json:"status"`
	Version      int64         `json:"version"`
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
		Media:              Media{Portrait: d.RoleAvatar, Background: d.RoleBackground},
	}
	if d.RoleBackground == d.RoleAvatar {
		f.Manifest.Media.Background = ""
	}
	if len(d.CardMeta) > 0 && string(d.CardMeta) != "null" && string(d.CardMeta) != "{}" {
		f.Manifest.CardMeta = d.CardMeta
	}
	f.Definition = d.RoleDetailDesc
	f.Welcome = d.RoleWelcome
	for i, a := range d.RoleWelcomeAlts {
		f.Alternates = append(f.Alternates, Opening{File: fmt.Sprintf("%s/alt-%02d.md", OpeningsDir, i+1), Text: a})
	}
	if lorebookID != "" {
		lb := &Lorebook{ID: lorebookID, Name: lorebookName, Entries: []LorebookEntry{}}
		for _, e := range entries {
			lb.Entries = append(lb.Entries, LorebookEntry{
				ID: e.EntryID, Name: e.Name, Content: e.Content, Category: e.Category,
				Keywords: nonNil(e.Keywords), SecondaryKeywords: e.SecondaryKeywords,
				Constant: e.IsConstant, Disabled: !e.IsEnabled, TriggerRegion: e.TriggerRegion,
				MatchOptions: nullToNil(e.MatchOptions),
			})
		}
		f.Lorebook = lb
	}
	if asset != nil && (len(asset.Rules) > 0 || asset.MountTrigger != "" || asset.PageMode != "" && asset.PageMode != "classic") {
		f.Rules = &Rules{Rules: asset.Rules, MountTrigger: asset.MountTrigger, MountLayer: asset.MountLayer, PageMode: asset.PageMode}
		f.State.AuthorAssetVersion = asset.Version
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
