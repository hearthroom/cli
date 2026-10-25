package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/output"
)

// cardState is `card state`: the author's view of the card's shared state (the polls and counters
// declared in rules.json sharedState), the author switch, and resets. All on the community site.
func (a *App) cardState() *cobra.Command {
	c := &cobra.Command{Use: "state", Short: "Show or manage the shared state players write on the linked card", Long: `Shared state is what rules.json declares under sharedState: polls players vote
in and counters they add to (co-op goals, leaderboards). Players can write only
after they have played the card; the platform stores numbers and declared option
ids, never player text.

  state show [dir]            live and draft totals, the switches and the declaration
  state off|on [dir]          stop or resume player reads and writes (data is kept)
  state reset <key> [dir]     set one key back to zero (--draft for your playtest data)
  state clear-draft [dir]     drop everything written while you playtested the draft

Your own playtests (Play draft on the card page, private cards) write to the draft data and
never mix into what players see.`}
	show := &cobra.Command{Use: "show [dir]", Short: "Show live and draft totals and the switches", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return a.runStateShow(cmd, args) }}
	off := &cobra.Command{Use: "off [dir]", Short: "Stop player reads and writes (data is kept)", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return a.runStateToggle(cmd, args, false) }}
	on := &cobra.Command{Use: "on [dir]", Short: "Resume player reads and writes", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return a.runStateToggle(cmd, args, true) }}
	var draft bool
	reset := &cobra.Command{Use: "reset <key> [dir]", Short: "Set one key back to zero", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		ns := "live"
		if draft {
			ns = "draft"
		}
		return a.runStatePost(cmd, args[1:], "/"+url.PathEscape(args[0])+"/reset", map[string]any{"ns": ns}, fmt.Sprintf("Reset %s (%s).", args[0], ns))
	}}
	reset.Flags().BoolVar(&draft, "draft", false, "reset your playtest data instead of the live data")
	clear := &cobra.Command{Use: "clear-draft [dir]", Short: "Drop the data written while you playtested the draft", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return a.runStatePost(cmd, args, "/draft/clear", map[string]any{}, "Cleared the draft data.")
	}}
	c.AddCommand(show, off, on, reset, clear)
	return c
}

func (a *App) statePath(args []string) (string, error) {
	f, err := loadFolder(args)
	if err != nil {
		return "", err
	}
	if f.State.Target != "owned" || f.State.RoleID == "" {
		return "", output.Exitf(2, "state needs a linked owned card; use card push --create first")
	}
	if f.State.API != "" && f.State.API != a.Client().API {
		return "", output.Exitf(2, "this folder is linked to a different provider")
	}
	if err = a.RequireAuth(); err != nil {
		return "", err
	}
	return "/me/cards/" + url.PathEscape(f.State.RoleID) + "/shared-state", nil
}

type stateView struct {
	PlatformEnabled bool                       `json:"platformEnabled"`
	AuthorOff       bool                       `json:"authorOff"`
	PlatformOff     bool                       `json:"platformOff"`
	Declaration     json.RawMessage            `json:"declaration"`
	Live            map[string]json.RawMessage `json:"live"`
	Draft           map[string]json.RawMessage `json:"draft"`
}

func (a *App) runStateShow(cmd *cobra.Command, args []string) error {
	path, err := a.statePath(args)
	if err != nil {
		return err
	}
	var v stateView
	if err = a.Client().SiteDo(cmd.Context(), http.MethodGet, path, nil, &v); err != nil {
		return err
	}
	if a.Out.JSON {
		return a.Out.JSONValue(v)
	}
	switch {
	case !v.PlatformEnabled:
		a.Out.Line("Shared state: not available on this site yet")
	case v.PlatformOff:
		a.Out.Line("Shared state: turned off for this card by the site")
	case v.AuthorOff:
		a.Out.Line("Shared state: off (state on resumes it)")
	default:
		a.Out.Line("Shared state: on")
	}
	keys := make([]string, 0, len(v.Live))
	for k := range v.Live {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		a.Out.Line("Nothing declared: add sharedState to rules.json and push.")
	}
	for _, k := range keys {
		a.Out.Line("  %s  live %s  draft %s", k, describeState(v.Live[k]), describeState(v.Draft[k]))
	}
	return nil
}

// describeState prints one key's totals: per-option votes for a poll, the total (and goal) for a counter.
func describeState(raw json.RawMessage) string {
	var s struct {
		Kind   string         `json:"kind"`
		Counts map[string]int `json:"counts"`
		Total  int64          `json:"total"`
		Goal   int64          `json:"goal"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return "-"
	}
	if s.Kind == "poll" {
		opts := make([]string, 0, len(s.Counts))
		for o := range s.Counts {
			opts = append(opts, o)
		}
		sort.Strings(opts)
		parts := make([]string, 0, len(opts))
		for _, o := range opts {
			parts = append(parts, fmt.Sprintf("%s %d", o, s.Counts[o]))
		}
		return strings.Join(parts, ", ")
	}
	if s.Goal > 0 {
		return fmt.Sprintf("%d/%d", s.Total, s.Goal)
	}
	return fmt.Sprintf("%d", s.Total)
}

func (a *App) runStateToggle(cmd *cobra.Command, args []string, enabled bool) error {
	path, err := a.statePath(args)
	if err != nil {
		return err
	}
	var r map[string]any
	if err = a.Client().SiteDo(cmd.Context(), http.MethodPut, path, map[string]any{"enabled": enabled}, &r); err != nil {
		return err
	}
	if a.Out.JSON {
		return a.Out.JSONValue(r)
	}
	if enabled {
		a.Out.Line("Shared state is on.")
	} else {
		a.Out.Line("Shared state is off; the data is kept.")
	}
	return nil
}

func (a *App) runStatePost(cmd *cobra.Command, args []string, suffix string, body map[string]any, done string) error {
	path, err := a.statePath(args)
	if err != nil {
		return err
	}
	var r map[string]any
	if err = a.Client().SiteDo(cmd.Context(), http.MethodPost, path+suffix, body, &r); err != nil {
		return err
	}
	if a.Out.JSON {
		return a.Out.JSONValue(r)
	}
	a.Out.Line("%s", done)
	return nil
}
