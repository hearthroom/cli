package card

import (
	"regexp"
	"strings"
)

const legacyFengyueCSSID = "hearthroom-fengyue-inner-css"

var legacyStyleClose = regexp.MustCompile(`(?i)</style(?:[\t\n\f\r />]|$)`)

// Only the unchanged built-in carrier emitted by the web importer is migrated.
// Presence, including an empty card.css, is authoritative; edited/disabled/duplicate rules stay rules.
func (f *Folder) migrateLegacyCardCSS() {
	if f.CardCSS != nil || f.Rules == nil {
		return
	}
	index, count := -1, 0
	for i, r := range f.Rules.Rules {
		if r.ID == legacyFengyueCSSID {
			index = i
			count++
		}
	}
	if count != 1 {
		return
	}
	r := f.Rules.Rules[index]
	if !r.Enabled || r.Find != `/([\s\S])$/` || !strings.HasPrefix(r.Replace, "$1<style>") || !strings.HasSuffix(r.Replace, "</style>") {
		return
	}
	css := strings.TrimSuffix(strings.TrimPrefix(r.Replace, "$1<style>"), "</style>")
	if legacyStyleClose.MatchString(css) {
		return
	}
	dialect := r.CSSDialect
	if dialect == "" {
		dialect = "fengyue"
	}
	f.CardCSS = &css
	if f.Rules.CardCSSDialect == nil {
		f.Rules.CardCSSDialect = &dialect
	}
	rules := append([]DisplayRule(nil), f.Rules.Rules[:index]...)
	f.Rules.Rules = append(rules, f.Rules.Rules[index+1:]...)
}
