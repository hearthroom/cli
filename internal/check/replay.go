package check

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Health is what real replies say about the status protocol: how often the block is
// written, how it drifts, which keys the model forgets, and what it costs the story.
type Health struct {
	Replies           int                `json:"replies"`
	WithBlock         int                `json:"withBlock"`
	BlockRate         float64            `json:"blockRate"`
	MissingClose      int                `json:"missingClose"`
	SkippedLines      int                `json:"skippedLines"`
	FullWidthLines    int                `json:"fullWidthLines"`
	ChoicesBlocks     int                `json:"choicesBlocks"`
	Overhead          float64            `json:"overhead"`  // block characters / reply characters
	Threshold         float64            `json:"threshold"` // the card's declared ceiling
	OverThreshold     bool               `json:"overThreshold"`
	WorstReply        float64            `json:"worstReply"`
	Keys              map[string]KeyRate `json:"keys"`
	RequiredKeysBelow []string           `json:"requiredKeysBelow90"`
	VolatileKeys      []string           `json:"volatileKeys"`
	// Markers the rules consume (any marker, not only [status]): how often each appears
	// and how much of the replies it wraps. A marker that wraps prose (dialogue, a
	// chapter head) counts its whole span, so read the per-marker shares before cutting.
	Markers map[string]MarkerRate `json:"markers"`
}

// MarkerRate is how one rule-consumed marker shows up across the replies.
type MarkerRate struct {
	Rate     float64 `json:"rate"`     // share of replies containing it at least once
	PerReply float64 `json:"perReply"` // occurrences per reply
	Share    float64 `json:"share"`    // characters inside it / all reply characters
}

// KeyRate is how often one key appeared.
type KeyRate struct {
	Count int     `json:"count"`
	Rate  float64 `json:"rate"`
}

// ReplayOptions tunes the health report.
type ReplayOptions struct {
	Threshold    float64  // 0 means 0.15
	RequiredKeys []string // keys the schema expects on every ordinary turn
	VolatileKeys []string // scene-only keys, never counted as forgotten
	Markers      []Marker // what the display rules consume (from Result.Markers)
}

// markerRegexp matches one occurrence of a marker and what it wraps: up to its closer
// when the closer comes before another marker, otherwise up to the next marker start.
func markerRegexp(m Marker) *regexp.Regexp {
	n := regexp.QuoteMeta(m.Name)
	if m.Angle {
		return regexp.MustCompile(`(?s)<` + n + `[^>]*>(?:[^<]|<[^a-zA-Z/])*(?:</` + n + `>)?`)
	}
	return regexp.MustCompile(`(?s)\[` + n + `\][^\[]*(?:\[/` + n + `\])?`)
}

var (
	blockRe     = regexp.MustCompile(`(?s)\[(status|choices)\]((?:.|\n)*?)(?:\[/(?:status|choices)\]|$)`)
	fullWidthRe = regexp.MustCompile(`[：，＝／％｜]`)
	listPrefix  = regexp.MustCompile(`^[-*+•]\s+`)
	keyDecor    = regexp.MustCompile("^[*_`#]+|[*_`#]+$")
)

// ReplayHealth measures replies against the block protocol.
func ReplayHealth(replies []string, opts ReplayOptions) *Health {
	threshold := opts.Threshold
	if threshold == 0 {
		threshold = 0.15
	}
	h := &Health{Replies: len(replies), Threshold: threshold, Keys: map[string]KeyRate{}, RequiredKeysBelow: []string{}, VolatileKeys: opts.VolatileKeys}
	if h.VolatileKeys == nil {
		h.VolatileKeys = []string{}
	}
	counts := map[string]int{}
	blockChars, replyChars := 0, 0
	type markerStat struct{ replies, count, chars int }
	markerStats := map[string]*markerStat{}
	consumedBlock := map[string]bool{}
	markerRes := make([]*regexp.Regexp, len(opts.Markers))
	for i, m := range opts.Markers {
		markerRes[i] = markerRegexp(m)
		markerStats[m.Name] = &markerStat{}
		if !m.Angle && (m.Name == "status" || m.Name == "choices") {
			consumedBlock[m.Name] = true
		}
	}
	for _, reply := range replies {
		replyChars += len([]rune(reply))
		found, chars := 0, 0
		for i, m := range opts.Markers {
			hits := markerRes[i].FindAllString(reply, -1)
			st := markerStats[m.Name]
			if len(hits) > 0 {
				st.replies++
				st.count += len(hits)
			}
			for _, h := range hits {
				st.chars += len([]rune(h))
				chars += len([]rune(h))
			}
		}
		for _, m := range blockRe.FindAllStringSubmatch(reply, -1) {
			// a block a rule consumes (every kit card) is already in chars from the marker pass;
			// its keys are still tallied
			if !consumedBlock[m[1]] {
				chars += len([]rune(m[0]))
			}
			if m[1] == "choices" {
				h.ChoicesBlocks++
				continue
			}
			found++
			if !strings.Contains(m[0], "[/status]") {
				h.MissingClose++
			}
			for _, line := range regexp.MustCompile(`\r?\n|;;`).Split(m[2], -1) {
				t := strings.TrimSpace(line)
				if t == "" {
					continue
				}
				if fullWidthRe.MatchString(t) {
					h.FullWidthLines++
					t = strings.ReplaceAll(t, "：", ":")
				}
				t = listPrefix.ReplaceAllString(t, "")
				i := strings.Index(t, ":")
				if i <= 0 {
					h.SkippedLines++
					continue
				}
				key := strings.ToLower(strings.TrimSpace(keyDecor.ReplaceAllString(strings.TrimSpace(t[:i]), "")))
				if key == "" {
					h.SkippedLines++
					continue
				}
				counts[key]++
			}
		}
		if found > 0 {
			h.WithBlock++
		}
		blockChars += chars
		if replyChars > 0 && len([]rune(reply)) > 0 {
			ratio := float64(chars) / float64(len([]rune(reply)))
			if ratio > h.WorstReply {
				h.WorstReply = ratio
			}
		}
	}
	n := float64(len(replies))
	if n == 0 {
		n = 1
	}
	h.BlockRate = float64(h.WithBlock) / n
	if replyChars > 0 {
		h.Overhead = math.Round(float64(blockChars)/float64(replyChars)*1000) / 1000
	}
	h.OverThreshold = h.Overhead > threshold
	for k, c := range counts {
		h.Keys[k] = KeyRate{Count: c, Rate: float64(c) / n}
	}
	h.Markers = map[string]MarkerRate{}
	for name, st := range markerStats {
		share := 0.0
		if replyChars > 0 {
			share = float64(st.chars) / float64(replyChars)
		}
		h.Markers[name] = MarkerRate{Rate: float64(st.replies) / n, PerReply: float64(st.count) / n, Share: math.Round(share*1000) / 1000}
	}
	need := int(math.Ceil(n * 0.9))
	for _, k := range opts.RequiredKeys {
		if counts[strings.ToLower(k)] < need {
			h.RequiredKeysBelow = append(h.RequiredKeysBelow, k)
		}
	}
	sort.Strings(h.RequiredKeysBelow)
	return h
}

var (
	historySection = regexp.MustCompile(`(?m)^\[(AI|USER|SYSTEM|Character|Player)\]\s*$`)
	sampleHeading  = regexp.MustCompile(`(?m)^## .*$`)
)

// RepliesFrom extracts AI reply texts from any of the transcript shapes an author has:
// `hearthroom play --history` text ([AI] / [USER] sections), `play --history --json` (one JSON
// object per line, or an array / {messages:[…]}), `preview/replies.md` (`## ` headings) or
// blank-line paragraphs.
func RepliesFrom(text string) []string {
	t := strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if t == "" {
		return nil
	}
	if out := repliesFromJSON(t); len(out) > 0 {
		return out
	}
	if locs := historySection.FindAllStringSubmatchIndex(t, -1); len(locs) > 0 {
		var out []string
		for i, loc := range locs {
			label := t[loc[2]:loc[3]]
			end := len(t)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			body := strings.TrimSpace(t[loc[1]:end])
			if (label == "AI" || label == "Character") && body != "" {
				out = append(out, body)
			}
		}
		return out
	}
	if sampleHeading.MatchString(t) {
		var out []string
		for _, part := range sampleHeading.Split(t, -1) {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	var out []string
	for _, p := range regexp.MustCompile(`\n\s*\n`).Split(t, -1) {
		if p = strings.TrimSpace(p); p != "" && (blockRe.MatchString(p) || len([]rune(p)) > 40) {
			out = append(out, p)
		}
	}
	return out
}

func repliesFromJSON(t string) []string {
	var out []string
	take := func(v any) {
		m, ok := v.(map[string]any)
		if !ok {
			return
		}
		role, _ := firstString(m, "role", "roleType", "from")
		content, ok := firstString(m, "content", "text", "message")
		if !ok {
			return
		}
		if role == "" || strings.EqualFold(role, "ai") || strings.EqualFold(role, "assistant") || strings.EqualFold(role, "char") || strings.EqualFold(role, "character") {
			out = append(out, content)
		}
	}
	var doc any
	if err := json.Unmarshal([]byte(t), &doc); err == nil {
		switch d := doc.(type) {
		case []any:
			for _, v := range d {
				take(v)
			}
		case map[string]any:
			if msgs, ok := d["messages"].([]any); ok {
				for _, v := range msgs {
					take(v)
				}
			} else {
				take(d)
			}
		}
		return out
	}
	for _, line := range strings.Split(t, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var v any
		if json.Unmarshal([]byte(line), &v) == nil {
			take(v)
		}
	}
	return out
}

func firstString(m map[string]any, keys ...string) (string, bool) {
	for _, k := range keys {
		if s, ok := m[k].(string); ok {
			return s, true
		}
	}
	return "", false
}

// KitFields reads kit.config.json (the sandbox kit's build input) for the required and
// volatile keys; a folder without one yields empty lists.
func KitFields(configJSON []byte) (required, volatile []string) {
	var cfg struct {
		Schema struct {
			Fields []struct {
				Key      string `json:"key"`
				Volatile bool   `json:"volatile"`
				Hidden   bool   `json:"hidden"`
			} `json:"fields"`
		} `json:"schema"`
	}
	if json.Unmarshal(configJSON, &cfg) != nil {
		return nil, nil
	}
	for _, f := range cfg.Schema.Fields {
		if f.Key == "" {
			continue
		}
		// A hidden field is still written every turn (it is parsed, not drawn), so it is required.
		if f.Volatile {
			volatile = append(volatile, f.Key)
		} else {
			required = append(required, f.Key)
		}
	}
	return required, volatile
}
