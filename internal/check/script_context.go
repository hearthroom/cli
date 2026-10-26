package check

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// scriptViews preserves offsets in two views: comments removed, and executable tokens
// only. Keep literal arguments in text for the event-name and save-key checks.
func scriptViews(source string) (text, code string) {
	clean, bare := []byte(source), []byte(source)
	blank := func(start, end int, comment bool) {
		for p := start; p < end; p++ {
			if source[p] != '\r' && source[p] != '\n' {
				bare[p] = ' '
				if comment {
					clean[p] = ' '
				}
			}
		}
	}
	ident := func(i int) bool {
		if i >= len(source) {
			return false
		}
		c := source[i]
		return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '$' || c >= 128
	}
	lineEnd := func(i int) bool {
		return i >= len(source) || source[i] == '\r' || source[i] == '\n' || strings.HasPrefix(source[i:], "\u2028") || strings.HasPrefix(source[i:], "\u2029")
	}
	keyword := func(word string) bool {
		switch word {
		case "return", "throw", "case", "delete", "void", "typeof", "new", "in", "of", "yield", "await", "else", "do", "instanceof":
			return true
		}
		return false
	}
	var walk func(int, bool) int
	walk = func(i int, interpolation bool) int {
		depth, regexAllowed, word := 0, true, ""
		var parens []bool
		for i < len(source) {
			start, c := i, source[i]
			var next byte
			if i+1 < len(source) {
				next = source[i+1]
			}
			r, size := utf8.DecodeRuneInString(source[i:])
			if unicode.IsSpace(r) {
				i += size
				continue
			}
			legacyClose := false
			if strings.HasPrefix(source[i:], "-->") {
				lineStart := strings.LastIndex(source[:i], "\n") + 1
				legacyClose = strings.TrimSpace(source[lineStart:i]) == ""
			}
			if c == '/' && next == '/' || strings.HasPrefix(source[i:], "<!--") || legacyClose {
				for !lineEnd(i) {
					i++
				}
				blank(start, i, true)
				continue
			}
			if c == '/' && next == '*' {
				end := strings.Index(source[i+2:], "*/")
				if end < 0 {
					i = len(source)
				} else {
					i += end + 4
				}
				blank(start, i, true)
				continue
			}
			if c == '\'' || c == '"' {
				i++
				for i < len(source) {
					if source[i] == '\\' {
						i = min(i+2, len(source))
						continue
					}
					q := source[i]
					i++
					if q == c {
						break
					}
				}
				blank(start, i, false)
				regexAllowed, word = false, ""
				continue
			}
			if c == '`' {
				blank(i, i+1, false)
				i++
				for i < len(source) {
					switch {
					case source[i] == '\\':
						end := min(i+2, len(source))
						blank(i, end, false)
						i = end
					case source[i] == '`':
						blank(i, i+1, false)
						i++
						goto templateDone
					case strings.HasPrefix(source[i:], "${"):
						blank(i, i+2, false)
						i = walk(i+2, true)
					default:
						blank(i, i+1, false)
						i++
					}
				}
			templateDone:
				regexAllowed, word = false, ""
				continue
			}
			if c == '/' && regexAllowed {
				inClass := false
				i++
				for i < len(source) && !lineEnd(i) {
					ch := source[i]
					i++
					if ch == '\\' {
						i = min(i+1, len(source))
						continue
					}
					if ch == '[' {
						inClass = true
					}
					if ch == ']' {
						inClass = false
					}
					if ch == '/' && !inClass {
						break
					}
				}
				for ident(i) {
					i++
				}
				blank(start, i, false)
				regexAllowed, word = false, ""
				continue
			}
			if ident(i) {
				for ident(i) {
					i++
				}
				word = source[start:i]
				regexAllowed = keyword(word)
				continue
			}
			i++
			if c == '{' {
				depth++
			}
			if c == '}' {
				if interpolation && depth == 0 {
					blank(start, i, false)
					return i
				}
				depth--
			}
			if c == '(' {
				control := word == "if" || word == "while" || word == "for" || word == "with" || word == "switch" || word == "catch"
				parens = append(parens, control)
			}
			if c == ')' {
				regexAllowed = false
				if len(parens) > 0 {
					regexAllowed = parens[len(parens)-1]
					parens = parens[:len(parens)-1]
				}
			} else if (c == '+' || c == '-') && next == c {
				i++
				regexAllowed = false
			} else {
				regexAllowed = c != ']' && c != '}' && c != '.'
			}
			word = ""
		}
		return i
	}
	walk(0, false)
	return string(clean), string(bare)
}

var htmlContext = regexp.MustCompile(`(?is)<!--[\s\S]*?(?:-->|$)|<script\b[^>]*>[\s\S]*?</script\s*>`)

// Strip HTML comments outside script raw text before extracting scripts.
func activeHTML(html string) string {
	return htmlContext.ReplaceAllStringFunc(html, func(part string) string {
		if !strings.HasPrefix(part, "<!--") {
			return part
		}
		return strings.Map(func(r rune) rune {
			if r == '\r' || r == '\n' {
				return r
			}
			return ' '
		}, part)
	})
}

// literalMatches reads arguments from text but admits only calls beginning in code.
func literalMatches(pattern *regexp.Regexp, text, code string) [][]string {
	var matches [][]string
	for _, pos := range pattern.FindAllStringSubmatchIndex(text, -1) {
		if code[pos[0]] == ' ' {
			continue
		}
		match := make([]string, len(pos)/2)
		for j := range match {
			if pos[j*2] >= 0 {
				match[j] = text[pos[j*2]:pos[j*2+1]]
			}
		}
		matches = append(matches, match)
	}
	return matches
}
