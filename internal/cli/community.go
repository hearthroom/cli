package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	extraCommands = append(extraCommands, func(a *App) []*cobra.Command {
		return []*cobra.Command{a.searchCommand(), a.tagsCommand(), a.authorCommand(), a.whoamiCommand(), a.walletCommand(), a.modelsCommand()}
	})
}

// site card as the community API lists it; extra fields are kept in Raw.
type siteCard struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Summary string   `json:"summary"`
	Tags    []string `json:"tags"`
	Author  *struct {
		Handle string `json:"handle"`
		Name   string `json:"name"`
	} `json:"author"`
	Zone string `json:"zone"`
}

type siteList struct {
	Items   []json.RawMessage `json:"items"`
	HasNext bool              `json:"hasNext"`
	Limit   int               `json:"limit"`
	Offset  int               `json:"offset"`
	Total   *int              `json:"total"`
}

func (a *App) searchCommand() *cobra.Command {
	var zone, sort, lang, author string
	var tags []string
	var limit, offset int
	c := &cobra.Command{
		Use:   "search [text]",
		Short: "Browse or search the community board (no sign-in needed)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
			if len(args) > 0 {
				q.Set("q", args[0])
			}
			if zone != "" {
				q.Set("zone", zone)
			}
			if sort != "" {
				q.Set("sort", sort)
			}
			if lang != "" {
				q.Set("lang", lang)
			}
			if author != "" {
				q.Set("author", author)
			}
			for _, t := range tags {
				q.Add("tag", t)
			}
			var raw json.RawMessage
			if err := a.Client().SiteGet(cmd.Context(), "/cards", q, &raw, false); err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(raw)
			}
			var list siteList
			if err := json.Unmarshal(raw, &list); err != nil {
				return err
			}
			rows := make([][]string, 0, len(list.Items))
			for _, item := range list.Items {
				var c siteCard
				_ = json.Unmarshal(item, &c)
				by := ""
				if c.Author != nil {
					by = c.Author.Handle
				}
				rows = append(rows, []string{c.ID, clip(c.Name, 28), by, clip(strings.Join(c.Tags, ","), 30), clip(c.Summary, 48)})
			}
			a.Out.Table([]string{"ID", "NAME", "AUTHOR", "TAGS", "SUMMARY"}, rows)
			if list.HasNext {
				a.Out.Line("More: --offset %d", offset+limit)
			}
			return nil
		},
	}
	c.Flags().StringVar(&zone, "zone", "", "content language zone: zh, en, ja, ko or all (site default: zh)")
	c.Flags().StringVar(&sort, "sort", "", "new or top")
	c.Flags().StringVar(&lang, "lang", "", "display language for card text")
	c.Flags().StringVar(&author, "author", "", "author handle (ignores --zone)")
	c.Flags().StringArrayVar(&tags, "tag", nil, "require a tag (repeatable)")
	c.Flags().IntVar(&limit, "limit", 20, "entries per page")
	c.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	return c
}

func (a *App) cardView() *cobra.Command {
	return &cobra.Command{
		Use:   "view <id>",
		Short: "Show a community card",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw json.RawMessage
			if err := a.Client().SiteGet(cmd.Context(), "/cards/"+url.PathEscape(args[0]), url.Values{"view": {"0"}}, &raw, true); err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(raw)
			}
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				return err
			}
			for _, k := range []string{"id", "name", "zone", "summary", "tags", "author", "createdAt", "updatedAt"} {
				if v, ok := m[k]; ok {
					a.Out.Line("%-10s %s", k+":", flat(v))
				}
			}
			if op, ok := m["opening"]; ok {
				a.Out.Line("")
				a.Out.Line("%s", flat(op))
			}
			return nil
		},
	}
}

func (a *App) tagsCommand() *cobra.Command {
	var q, zone string
	var limit int
	c := &cobra.Command{
		Use:   "tags",
		Short: "List community tags",
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{"limit": {strconv.Itoa(limit)}}
			if q != "" {
				query.Set("q", q)
			}
			if zone != "" {
				query.Set("zone", zone)
			}
			var raw json.RawMessage
			if err := a.Client().SiteGet(cmd.Context(), "/tags", query, &raw, false); err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(raw)
			}
			var list siteList
			_ = json.Unmarshal(raw, &list)
			rows := [][]string{}
			for _, item := range list.Items {
				var m map[string]any
				_ = json.Unmarshal(item, &m)
				rows = append(rows, []string{flat(firstOf(m, "tag", "name")), flat(firstOf(m, "n", "count"))})
			}
			a.Out.Table([]string{"TAG", "CARDS"}, rows)
			return nil
		},
	}
	c.Flags().StringVar(&q, "q", "", "tag search text")
	c.Flags().StringVar(&zone, "zone", "", "content language zone")
	c.Flags().IntVar(&limit, "limit", 40, "entries")
	return c
}

func (a *App) authorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "author <handle>",
		Short: "Show a community author",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw json.RawMessage
			if err := a.Client().SiteGet(cmd.Context(), "/authors/"+url.PathEscape(args[0]), nil, &raw, false); err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(raw)
			}
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			for _, k := range []string{"handle", "name", "bio", "cardCount", "followers"} {
				if v, ok := m[k]; ok {
					a.Out.Line("%-10s %s", k+":", flat(v))
				}
			}
			return nil
		},
	}
}

func (a *App) whoamiCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the signed-in account on the provider and the community site",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			me, err := a.whoami(cmd.Context())
			if err != nil {
				return err
			}
			var site json.RawMessage
			siteErr := a.Client().SiteGet(cmd.Context(), "/me", nil, &site, true)
			if a.Out.JSON {
				out := map[string]any{"provider": me}
				if siteErr == nil {
					out["site"] = site
				} else {
					out["siteError"] = siteErr.Error()
				}
				return a.Out.JSONValue(out)
			}
			a.Out.Line("Provider %s: %s (user %d, %s)", a.API, displayName(me), me.AccountNumID, me.AccountType)
			if siteErr != nil {
				a.Out.Line("Site %s: not available (%v)", a.Site, siteErr)
				return nil
			}
			var m map[string]any
			_ = json.Unmarshal(site, &m)
			a.Out.Line("Site %s: %s", a.Site, flat(firstOf(m, "handle", "name", "displayName")))
			return nil
		},
	}
}

func (a *App) walletCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "wallet",
		Short: "Show your credit balance and plans",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			var raw json.RawMessage
			if err := a.Client().OpenGet(cmd.Context(), "/me/wallet", nil, &raw); err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(raw)
			}
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sortStrings(keys)
			for _, k := range keys {
				a.Out.Line("%-24s %s", k+":", clip(flat(m[k]), 80))
			}
			return nil
		},
	}
}

type modelGroup struct {
	Group    string `json:"group"`
	Families []struct {
		Family   string `json:"family"`
		Variants []struct {
			Name      string `json:"name"`
			Value     string `json:"value"`
			CostScore int    `json:"costScore"`
			IsMember  bool   `json:"isMember"`
			Channel   string `json:"channel"`
			Status    *struct {
				Status string `json:"status"`
			} `json:"status"`
			SupportsMultiPass bool   `json:"supportsMultiPass"`
			BillingType       string `json:"billingType"`
		} `json:"variants"`
	} `json:"families"`
}

func (a *App) modelsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "List models the provider offers",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			var raw json.RawMessage
			if err := a.Client().OpenGet(cmd.Context(), "/models", nil, &raw); err != nil {
				return err
			}
			if a.Out.JSON {
				return a.Out.JSONValue(raw)
			}
			var groups []modelGroup
			if err := json.Unmarshal(raw, &groups); err != nil {
				return err
			}
			rows := [][]string{}
			for _, g := range groups {
				for _, f := range g.Families {
					for _, v := range f.Variants {
						status := ""
						if v.Status != nil {
							status = v.Status.Status
						}
						member := ""
						if v.IsMember {
							member = "member"
						}
						rows = append(rows, []string{v.Value, clip(v.Name, 28), g.Group, strconv.Itoa(v.CostScore), member, v.Channel, status})
					}
				}
			}
			a.Out.Table([]string{"MODEL", "NAME", "GROUP", "COST", "PLAN", "LANE", "STATUS"}, rows)
			return nil
		},
	}
}

func clip(s string, n int) string {
	r := []rune(strings.ReplaceAll(s, "\n", " "))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}

func flat(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			parts = append(parts, flat(e))
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		if s := firstOf(x, "handle", "name", "id"); s != nil {
			return flat(s)
		}
		b, _ := json.Marshal(x)
		return string(b)
	default:
		return fmt.Sprint(x)
	}
}

func firstOf(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil && v != "" {
			return v
		}
	}
	return nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
