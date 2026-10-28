package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

func terminalText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
}
func formatRecallDebug(raw json.RawMessage) string {
	var data struct {
		Status            string
		Partial           bool
		Omitted, Redacted int
		Resources         []recallDebugEntry
		Selections        []struct {
			Scope, Member string
			Entries       []recallDebugEntry
		}
		Tools []struct {
			Sequence                      int
			Tool, Resource, Query, Result string
			IDs                           []string
			Hits                          []struct {
				ID            string `json:"entryId"`
				Kind, Content string
				Truncated     bool
			}
		}
		Dispatches []struct {
			Ordinal int
			Phase   string
			Hits    []struct {
				ID   string `json:"entryId"`
				Kind string
			}
		}
	}
	var b strings.Builder
	b.WriteString("\nLorebook recall details\n")
	if json.Unmarshal(raw, &data) != nil {
		data.Status = "unavailable"
	}
	status := data.Status
	if data.Partial {
		status = "partial"
	}
	b.WriteString(recallLabel("status." + status))
	b.WriteByte('\n')
	if data.Status == "ready" && !data.Partial && len(data.Resources) == 0 && len(data.Selections) == 0 && len(data.Tools) == 0 {
		b.WriteString(recallLabel("no_entries"))
		b.WriteByte('\n')
	}
	for _, sel := range data.Selections {
		fmt.Fprintf(&b, "[%s %s]\n", recallLabel("scope."+sel.Scope), sel.Member)
		for _, e := range sel.Entries {
			renderRecallEntry(&b, e)
		}
	}
	for _, e := range data.Resources {
		renderRecallEntry(&b, e)
	}
	for _, tool := range data.Tools {
		fmt.Fprintf(&b, "  Tool step %d: %s %s %s %v / %s\n", tool.Sequence, tool.Tool, tool.Resource, tool.Query, tool.IDs, recallLabel(tool.Result))
		if tool.Result == "pending" || tool.Result == "replayed" {
			b.WriteString("    " + recallLabel(tool.Result+".description") + "\n")
		}
		for _, hit := range tool.Hits {
			fmt.Fprintf(&b, "    %s (%s)\n", hit.ID, recallLabel(hit.Kind))
			if hit.Content != "" {
				fmt.Fprintf(&b, "    %s\n", strings.ReplaceAll(hit.Content, "\n", "\n    "))
			}
		}
	}
	for _, call := range data.Dispatches {
		fmt.Fprintf(&b, "  Model call %d [%s]:", call.Ordinal, recallLabel(call.Phase))
		for _, h := range call.Hits {
			fmt.Fprintf(&b, " %s(%s)", h.ID, recallLabel("exposure."+h.Kind))
		}
		b.WriteByte('\n')
	}
	for _, count := range []struct {
		key string
		n   int
	}{{"omitted", data.Omitted}, {"redacted", data.Redacted}} {
		if count.n > 0 {
			form := "other"
			if count.n == 1 {
				form = "one"
			}
			b.WriteString(strings.ReplaceAll(recallLabel(count.key+"."+form), "{count}", fmt.Sprint(count.n)) + "\n")
		}
	}
	return terminalText(b.String())
}

type recallDebugEntry struct {
	ID                                                 string `json:"entryId"`
	Name, Source, Lifecycle, Decision, Reason, Content string
	ContentTruncated                                   bool
	Keys                                               []struct{ Key, Kind, Mode, Region string }
	VectorScore                                        *float64
	VectorRank                                         int
	RerankScore                                        *float64
	RerankRank                                         int
}

func renderRecallEntry(b *strings.Builder, e recallDebugEntry) {
	fmt.Fprintf(b, "  %s [%s] — %s / %s / %s", e.Name, e.ID, recallLabel(e.Decision), recallLabel("lifecycle."+e.Lifecycle), recallLabel("source."+e.Source))
	if e.Reason != "" {
		fmt.Fprintf(b, " (%s)", recallLabel(e.Reason))
	}
	b.WriteByte('\n')
	for _, key := range e.Keys {
		fmt.Fprintf(b, "    %s %s [%s]: %s\n", recallLabel(key.Kind), recallLabel(key.Mode), recallLabel("region."+key.Region), key.Key)
	}
	if e.VectorScore != nil {
		fmt.Fprintf(b, "    Vector similarity: %.4f\n", *e.VectorScore)
		if e.VectorRank > 0 {
			fmt.Fprintf(b, "    Vector rank: %d\n", e.VectorRank)
		}
	}
	if e.RerankScore != nil {
		fmt.Fprintf(b, "    Rerank score: %.4f\n", *e.RerankScore)
		if e.RerankRank > 0 {
			fmt.Fprintf(b, "    Rerank rank: %d\n", e.RerankRank)
		}
	}
	if e.Content != "" {
		fmt.Fprintf(b, "    %s\n", strings.ReplaceAll(e.Content, "\n", "\n    "))
	}
	if e.ContentTruncated {
		b.WriteString("    " + recallLabel("content_truncated") + "\n")
	}
}
