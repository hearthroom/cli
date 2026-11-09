package importer

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/hearthroom/cli/internal/card"
)

// ParseRelaxedJSON reads strict JSON first, then the JSON5 habits authors use
// in hand-written MagVarUpdate initial variables: comments (// and /* */),
// trailing commas, single-quoted strings and unquoted keys. YAML, EJS and other
// non-JSON text fail, as do hex numbers, Infinity, NaN and multi-line strings.
// Key order is kept. Mirrors hearthroom/web/src/lib/relaxed-json.ts.
func ParseRelaxedJSON(text string) (*card.JSONNode, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	if n, err := card.ParseJSON([]byte(text)); err == nil {
		return n, nil
	}
	p := &relaxedParser{s: text}
	n, err := p.value(0)
	if err != nil {
		return nil, err
	}
	p.space()
	if p.err == nil && p.i < len(p.s) {
		return nil, errRelaxed
	}
	return n, p.err
}

var errRelaxed = errors.New("not JSON")

type relaxedParser struct {
	s   string
	i   int
	err error
}

func (p *relaxedParser) peek(off int) byte {
	if p.i+off < len(p.s) {
		return p.s[p.i+off]
	}
	return 0
}

func (p *relaxedParser) space() {
	for p.i < len(p.s) {
		r, size := utf8.DecodeRuneInString(p.s[p.i:])
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\u00a0' || r == '\ufeff':
			p.i += size
		case r == '/' && p.peek(1) == '/':
			for p.i < len(p.s) && p.s[p.i] != '\n' {
				p.i++
			}
		case r == '/' && p.peek(1) == '*':
			end := strings.Index(p.s[p.i+2:], "*/")
			if end < 0 {
				p.err = errRelaxed
				p.i = len(p.s)
				return
			}
			p.i += 2 + end + 2
		default:
			return
		}
	}
}

func (p *relaxedParser) value(depth int) (*card.JSONNode, error) {
	if depth > 64 {
		return nil, errRelaxed
	}
	p.space()
	if p.err != nil {
		return nil, p.err
	}
	switch c := p.peek(0); {
	case c == '{':
		return p.object(depth)
	case c == '[':
		return p.array(depth)
	case c == '"' || c == '\'':
		s, err := p.str()
		return card.NewJSONString(s), err
	case c == '-' || c == '+' || c == '.' || c >= '0' && c <= '9':
		return p.number()
	}
	switch p.identifier() {
	case "true":
		return &card.JSONNode{Kind: card.JSONBool, Bool: true}, nil
	case "false":
		return &card.JSONNode{Kind: card.JSONBool}, nil
	case "null":
		return &card.JSONNode{Kind: card.JSONNull}, nil
	}
	return nil, errRelaxed
}

func (p *relaxedParser) object(depth int) (*card.JSONNode, error) {
	out := card.NewJSONObject()
	p.i++
	for {
		p.space()
		if p.err != nil {
			return nil, p.err
		}
		if p.peek(0) == '}' {
			p.i++
			return out, nil
		}
		var key string
		if c := p.peek(0); c == '"' || c == '\'' {
			var err error
			if key, err = p.str(); err != nil {
				return nil, err
			}
		} else if key = p.identifier(); key == "" {
			return nil, errRelaxed
		}
		p.space()
		if p.peek(0) != ':' {
			return nil, errRelaxed
		}
		p.i++
		v, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		out.Set(key, v)
		p.space()
		switch p.peek(0) {
		case ',':
			p.i++
		case '}':
			p.i++
			return out, nil
		default:
			return nil, errRelaxed
		}
	}
}

func (p *relaxedParser) array(depth int) (*card.JSONNode, error) {
	out := &card.JSONNode{Kind: card.JSONArray, Arr: []*card.JSONNode{}}
	p.i++
	for {
		p.space()
		if p.err != nil {
			return nil, p.err
		}
		if p.peek(0) == ']' {
			p.i++
			return out, nil
		}
		v, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		out.Arr = append(out.Arr, v)
		p.space()
		switch p.peek(0) {
		case ',':
			p.i++
		case ']':
			p.i++
			return out, nil
		default:
			return nil, errRelaxed
		}
	}
}

var relaxedEscapes = map[byte]string{'b': "\b", 'f': "\f", 'n': "\n", 'r': "\r", 't': "\t", 'v': "\v", '0': "\x00"}

func (p *relaxedParser) str() (string, error) {
	quote := p.s[p.i]
	p.i++
	var b strings.Builder
	var units []uint16 // pending \u escapes, so surrogate pairs join
	flush := func() {
		if len(units) > 0 {
			b.WriteString(string(utf16.Decode(units)))
			units = nil
		}
	}
	for p.i < len(p.s) {
		c := p.s[p.i]
		p.i++
		switch {
		case c == quote:
			flush()
			return b.String(), nil
		case c == '\n':
			return "", errRelaxed
		case c != '\\':
			flush()
			b.WriteByte(c)
			continue
		}
		if p.i >= len(p.s) {
			return "", errRelaxed
		}
		e := p.s[p.i]
		p.i++
		switch {
		case e == 'u':
			if p.i+4 > len(p.s) {
				return "", errRelaxed
			}
			v, err := strconv.ParseUint(p.s[p.i:p.i+4], 16, 16)
			if err != nil {
				return "", errRelaxed
			}
			units = append(units, uint16(v))
			p.i += 4
		case e == '\n':
			flush() // a backslash at the end of a line continues the string
		default:
			flush()
			if r, ok := relaxedEscapes[e]; ok {
				b.WriteString(r)
			} else {
				b.WriteByte(e)
			}
		}
	}
	return "", errRelaxed
}

var relaxedNumber = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?`)

func (p *relaxedParser) number() (*card.JSONNode, error) {
	m := relaxedNumber.FindString(p.s[p.i:])
	if m == "" {
		return nil, errRelaxed
	}
	p.i += len(m)
	f, err := strconv.ParseFloat(strings.TrimPrefix(m, "+"), 64)
	if err != nil {
		return nil, errRelaxed
	}
	return &card.JSONNode{Kind: card.JSONNumber, Num: card.FormatJSONNumber(f)}, nil
}

// identifier reads an unquoted key or true/false/null: letters, digits, _ and $,
// plus any non-ASCII character (Chinese keys).
func (p *relaxedParser) identifier() string {
	start := p.i
	for p.i < len(p.s) {
		r, size := utf8.DecodeRuneInString(p.s[p.i:])
		ok := r == '_' || r == '$' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= 0x80 && r != '\u00a0' && r != '\ufeff' || p.i > start && r >= '0' && r <= '9'
		if !ok {
			break
		}
		p.i += size
	}
	return p.s[start:p.i]
}
