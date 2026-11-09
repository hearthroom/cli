package card

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// JSONNode is a JSON value that keeps object key order. Card variables are
// shown to players and the model in the order the author wrote them, and the
// provider stores them that way; a Go map would sort the keys on every pull.
type JSONNode struct {
	Kind JSONKind
	Bool bool
	Num  string // the number as written (always valid JSON)
	Str  string
	Arr  []*JSONNode
	Keys []string
	Vals []*JSONNode
}

// JSONKind is the type of a JSONNode.
type JSONKind int

const (
	JSONNull JSONKind = iota
	JSONBool
	JSONNumber
	JSONString
	JSONArray
	JSONObject
)

// NewJSONObject returns an empty object node.
func NewJSONObject() *JSONNode { return &JSONNode{Kind: JSONObject} }

// NewJSONString returns a string node.
func NewJSONString(s string) *JSONNode { return &JSONNode{Kind: JSONString, Str: s} }

// Get returns the value under key in an object.
func (n *JSONNode) Get(key string) (*JSONNode, bool) {
	if n == nil || n.Kind != JSONObject {
		return nil, false
	}
	for i, k := range n.Keys {
		if k == key {
			return n.Vals[i], true
		}
	}
	return nil, false
}

// Set replaces the value under key, or appends the key.
func (n *JSONNode) Set(key string, v *JSONNode) {
	for i, k := range n.Keys {
		if k == key {
			n.Vals[i] = v
			return
		}
	}
	n.Keys = append(n.Keys, key)
	n.Vals = append(n.Vals, v)
}

// Clone copies the tree.
func (n *JSONNode) Clone() *JSONNode {
	if n == nil {
		return nil
	}
	c := *n
	if n.Arr != nil {
		c.Arr = make([]*JSONNode, len(n.Arr))
		for i, v := range n.Arr {
			c.Arr[i] = v.Clone()
		}
	}
	if n.Keys != nil {
		c.Keys = append([]string(nil), n.Keys...)
		c.Vals = make([]*JSONNode, len(n.Vals))
		for i, v := range n.Vals {
			c.Vals[i] = v.Clone()
		}
	}
	return &c
}

// ParseJSON reads one strict JSON value keeping key order. A repeated key keeps
// its first position and its last value, as JavaScript's JSON.parse does.
func ParseJSON(data []byte) (*JSONNode, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	n, err := decodeJSONValue(dec, 0)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("unexpected data after the JSON value")
	}
	return n, nil
}

func decodeJSONValue(dec *json.Decoder, depth int) (*JSONNode, error) {
	if depth > 64 {
		return nil, errors.New("JSON nested too deeply")
	}
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case nil:
		return &JSONNode{Kind: JSONNull}, nil
	case bool:
		return &JSONNode{Kind: JSONBool, Bool: t}, nil
	case json.Number:
		return &JSONNode{Kind: JSONNumber, Num: string(t)}, nil
	case string:
		return NewJSONString(t), nil
	case json.Delim:
		switch t {
		case '[':
			arr := &JSONNode{Kind: JSONArray, Arr: []*JSONNode{}}
			for dec.More() {
				v, err := decodeJSONValue(dec, depth+1)
				if err != nil {
					return nil, err
				}
				arr.Arr = append(arr.Arr, v)
			}
			_, err := dec.Token()
			return arr, err
		case '{':
			obj := NewJSONObject()
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, _ := kt.(string)
				v, err := decodeJSONValue(dec, depth+1)
				if err != nil {
					return nil, err
				}
				obj.Set(key, v)
			}
			_, err := dec.Token()
			return obj, err
		}
	}
	return nil, fmt.Errorf("unexpected token %v", tok)
}

// MarshalJSON writes compact JSON in key order.
func (n *JSONNode) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	n.write(&b, false)
	return b.Bytes(), nil
}

// Compact is MarshalJSON without the error.
func (n *JSONNode) Compact() string {
	var b bytes.Buffer
	n.write(&b, false)
	return b.String()
}

// write emits compact JSON. canonical spells numbers the way the provider
// stores them (integers without a decimal point), for size checks.
func (n *JSONNode) write(b *bytes.Buffer, canonical bool) {
	if n == nil {
		b.WriteString("null")
		return
	}
	switch n.Kind {
	case JSONNull:
		b.WriteString("null")
	case JSONBool:
		b.WriteString(strconv.FormatBool(n.Bool))
	case JSONNumber:
		if canonical {
			b.WriteString(canonicalNumber(n.Num))
		} else {
			b.WriteString(n.Num)
		}
	case JSONString:
		writeJSONString(b, n.Str)
	case JSONArray:
		b.WriteByte('[')
		for i, v := range n.Arr {
			if i > 0 {
				b.WriteByte(',')
			}
			v.write(b, canonical)
		}
		b.WriteByte(']')
	default:
		b.WriteByte('{')
		for i, k := range n.Keys {
			if i > 0 {
				b.WriteByte(',')
			}
			writeJSONString(b, k)
			b.WriteByte(':')
			n.Vals[i].write(b, canonical)
		}
		b.WriteByte('}')
	}
}

func writeJSONString(b *bytes.Buffer, s string) {
	enc := json.NewEncoder(b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	b.Truncate(b.Len() - 1) // Encode appends a newline
}

func jsonString(s string) string {
	var b bytes.Buffer
	writeJSONString(&b, s)
	return b.String()
}

// canonicalNumber matches the provider's spelling of a stored number.
func canonicalNumber(lit string) string {
	f, err := strconv.ParseFloat(lit, 64)
	if err != nil {
		return lit
	}
	return FormatJSONNumber(f)
}

// FormatJSONNumber spells a float as the provider does: integers without a
// decimal point, others with at most twelve significant digits.
func FormatJSONNumber(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	s := strconv.FormatFloat(f, 'g', 12, 64)
	r, _ := strconv.ParseFloat(s, 64)
	return strconv.FormatFloat(r, 'f', -1, 64)
}

// FormatJSON indents by two spaces and keeps short lists of plain values on one
// line, so an MVU [value, "rule"] pair reads as one line.
func FormatJSON(n *JSONNode, indent string) string {
	if n == nil {
		return "null"
	}
	switch n.Kind {
	case JSONArray:
		if len(n.Arr) == 0 {
			return "[]"
		}
		flat := true
		for _, v := range n.Arr {
			if v.Kind == JSONArray || v.Kind == JSONObject {
				flat = false
				break
			}
		}
		if flat {
			parts := make([]string, len(n.Arr))
			for i, v := range n.Arr {
				parts[i] = v.Compact()
			}
			if line := "[" + strings.Join(parts, ", ") + "]"; utf8.RuneCountInString(line) <= 100 {
				return line
			}
		}
		inner := indent + "  "
		rows := make([]string, len(n.Arr))
		for i, v := range n.Arr {
			rows[i] = inner + FormatJSON(v, inner)
		}
		return "[\n" + strings.Join(rows, ",\n") + "\n" + indent + "]"
	case JSONObject:
		if len(n.Keys) == 0 {
			return "{}"
		}
		inner := indent + "  "
		rows := make([]string, len(n.Keys))
		for i, k := range n.Keys {
			rows[i] = inner + jsonString(k) + ": " + FormatJSON(n.Vals[i], inner)
		}
		return "{\n" + strings.Join(rows, ",\n") + "\n" + indent + "}"
	}
	return n.Compact()
}

// MergeJSON merges objects key by key; lists and plain values in next replace
// those in base. New keys follow the existing ones.
func MergeJSON(base, next *JSONNode) *JSONNode {
	if base == nil || base.Kind != JSONObject || next == nil || next.Kind != JSONObject {
		return next.Clone()
	}
	out := base.Clone()
	for i, k := range next.Keys {
		if cur, ok := out.Get(k); ok {
			out.Set(k, MergeJSON(cur, next.Vals[i]))
		} else {
			out.Set(k, next.Vals[i].Clone())
		}
	}
	return out
}

// JSONFromAny converts a decoded value (encoding/json with UseNumber) into a
// node. Object keys are sorted: a Go map has no order left to keep.
func JSONFromAny(v any) *JSONNode {
	switch t := v.(type) {
	case nil:
		return &JSONNode{Kind: JSONNull}
	case bool:
		return &JSONNode{Kind: JSONBool, Bool: t}
	case json.Number:
		return &JSONNode{Kind: JSONNumber, Num: string(t)}
	case float64:
		return &JSONNode{Kind: JSONNumber, Num: FormatJSONNumber(t)}
	case string:
		return NewJSONString(t)
	case []any:
		arr := &JSONNode{Kind: JSONArray, Arr: []*JSONNode{}}
		for _, item := range t {
			arr.Arr = append(arr.Arr, JSONFromAny(item))
		}
		return arr
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		obj := NewJSONObject()
		for _, k := range keys {
			obj.Set(k, JSONFromAny(t[k]))
		}
		return obj
	}
	return &JSONNode{Kind: JSONNull}
}
