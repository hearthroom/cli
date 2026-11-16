// Package cardtools validates author declarations and inert model output.
// It never executes tools or loads external schema resources.
package cardtools

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var ErrDeclaration = errors.New("invalid_card_tools")

type Definition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

type Result struct {
	Name  string `json:"name"`
	Args  any    `json:"args"`
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

const (
	MaxTools          = 16
	MaxSchemaBytes    = 32 * 1024
	MaxArgumentsBytes = 64 * 1024
)

var toolName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
var reserved = map[string]bool{"submit_reply": true, "write_draft": true, "finish_preparation": true, "record_format_notes": true}

type Set struct {
	definitions map[string]Definition
	schemas     map[string]*jsonschema.Schema
}

// Parse validates the wire declaration before a caller writes any card fields.
func Parse(raw json.RawMessage) ([]Definition, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > 132*1024 {
		return nil, ErrDeclaration
	}
	if _, err := decode(raw); err != nil {
		return nil, ErrDeclaration
	}
	var defs []Definition
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&defs); err != nil {
		return nil, ErrDeclaration
	}
	if _, err := Compile(defs); err != nil {
		return nil, ErrDeclaration
	}
	return defs, nil
}

// Compile validates a complete replacement declaration. An empty slice clears it.
// All errors are intentionally content-free; callers must not expose schema excerpts.
func Compile(defs []Definition) (*Set, error) {
	if len(defs) > MaxTools {
		return nil, ErrDeclaration
	}
	out := &Set{definitions: map[string]Definition{}, schemas: map[string]*jsonschema.Schema{}}
	total := 0
	for _, def := range defs {
		if !toolName.MatchString(def.Name) || reserved[def.Name] || len(def.Description) > 2048 || !utf8.ValidString(def.Description) || len(def.Schema) > MaxSchemaBytes {
			return nil, ErrDeclaration
		}
		if _, exists := out.definitions[def.Name]; exists {
			return nil, ErrDeclaration
		}
		total += len(def.Name) + len(def.Description) + len(def.Schema)
		if total > 128*1024 {
			return nil, ErrDeclaration
		}
		doc, err := decode(def.Schema)
		if err != nil {
			return nil, ErrDeclaration
		}
		root, ok := doc.(map[string]any)
		if !ok || root["type"] != "object" {
			return nil, ErrDeclaration
		}
		if err = boundedSchema(root); err != nil {
			return nil, ErrDeclaration
		}
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		compiler.UseLoader(noResources{})
		const resource = "https://card-schema.invalid/schema.json"
		if err = compiler.AddResource(resource, doc); err != nil {
			return nil, ErrDeclaration
		}
		sch, err := compiler.Compile(resource)
		if err != nil {
			return nil, ErrDeclaration
		}
		def.Schema = bytes.Clone(def.Schema)
		out.definitions[def.Name] = def
		out.schemas[def.Name] = sch
	}
	return out, nil
}

type noResources struct{}

func (noResources) Load(string) (any, error) { return nil, ErrDeclaration }

func (s *Set) Definition(name string) (Definition, bool) {
	if s == nil {
		return Definition{}, false
	}
	d, ok := s.definitions[name]
	d.Schema = bytes.Clone(d.Schema)
	return d, ok
}

func (s *Set) ValidateCall(name, args string) Result {
	out := Result{Name: name, Args: args, Error: "invalid_schema"}
	if len(args) > MaxArgumentsBytes {
		out.Args = ""
		out.Error = "arguments_limit"
		return out
	}
	if s == nil || s.schemas[name] == nil {
		out.Error = "unknown_tool"
		return out
	}
	doc, err := decode([]byte(args))
	if err != nil {
		out.Error = "invalid_json"
		return out
	}
	if s.schemas[name].Validate(doc) != nil {
		return out
	}
	out.Args, out.Valid, out.Error = doc, true, ""
	return out
}

// decode rejects duplicate keys, trailing JSON, invalid UTF-8 and excessive
// nesting. A duplicate must not acquire different meanings in two consumers.
func decode(raw []byte) (any, error) {
	if !utf8.Valid(raw) {
		return nil, ErrDeclaration
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var value func(int) (any, error)
	value = func(depth int) (any, error) {
		if depth > 64 {
			return nil, ErrDeclaration
		}
		t, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch t {
		case json.Delim('{'):
			m := map[string]any{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return nil, e
				}
				k, ok := key.(string)
				if !ok {
					return nil, ErrDeclaration
				}
				if _, exists := m[k]; exists {
					return nil, ErrDeclaration
				}
				v, e := value(depth + 1)
				if e != nil {
					return nil, e
				}
				m[k] = v
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return nil, ErrDeclaration
			}
			return m, nil
		case json.Delim('['):
			a := []any{}
			for d.More() {
				v, e := value(depth + 1)
				if e != nil {
					return nil, e
				}
				a = append(a, v)
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return nil, ErrDeclaration
			}
			return a, nil
		case json.Delim('}'), json.Delim(']'):
			return nil, ErrDeclaration
		default:
			return t, nil
		}
	}
	v, err := value(0)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, ErrDeclaration
	}
	return v, nil
}

// Only explicitly supported schema vocabulary can reach the compiler. Schema
// paths form a DAG: this bounds both author input and instance validation work.
func boundedSchema(root map[string]any) error {
	nodes := map[string]bool{}
	edges := map[string][]string{}
	refs := map[string]string{}
	escape := strings.NewReplacer("~", "~0", "/", "~1")
	var visit func(any, string, int) error
	visit = func(raw any, path string, depth int) error {
		if depth > 32 {
			return ErrDeclaration
		}
		nodes[path] = true
		if _, ok := raw.(bool); ok {
			return nil
		}
		m, ok := raw.(map[string]any)
		if !ok {
			return ErrDeclaration
		}
		child := func(v any, p string) error { edges[path] = append(edges[path], p); return visit(v, p, depth+1) }
		for key, v := range m {
			switch key {
			case "$schema":
				if v != "https://json-schema.org/draft/2020-12/schema" {
					return ErrDeclaration
				}
			case "$ref":
				r, ok := v.(string)
				if !ok || !strings.HasPrefix(r, "#/") {
					return ErrDeclaration
				}
				refs[path] = r[1:]
			case "$defs", "properties":
				items, ok := v.(map[string]any)
				if !ok {
					return ErrDeclaration
				}
				for k, c := range items {
					if err := child(c, path+"/"+key+"/"+escape.Replace(k)); err != nil {
						return err
					}
				}
			case "additionalProperties", "propertyNames", "items":
				if err := child(v, path+"/"+key); err != nil {
					return err
				}
			case "anyOf", "oneOf", "allOf", "prefixItems":
				items, ok := v.([]any)
				if !ok {
					return ErrDeclaration
				}
				for i, c := range items {
					p, _ := json.Marshal(i)
					if err := child(c, path+"/"+key+"/"+string(p)); err != nil {
						return err
					}
				}
			case "type", "required", "enum", "const", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf", "minItems", "maxItems", "uniqueItems", "minProperties", "maxProperties", "minLength", "maxLength", "title", "description", "$comment", "examples", "default":
			case "pattern":
				pattern, ok := v.(string)
				if !ok || len(pattern) > 2048 {
					return ErrDeclaration
				}
				if _, err := regexp.Compile(pattern); err != nil {
					return ErrDeclaration
				}
			default:
				return ErrDeclaration
			}
		}
		return nil
	}
	if err := visit(root, "", 0); err != nil {
		return err
	}
	for from, to := range refs {
		if !nodes[to] {
			return ErrDeclaration
		}
		edges[from] = append(edges[from], to)
	}
	active := map[string]bool{}
	type bound struct{ work, depth int }
	memo := map[string]bound{}
	var check func(string) (bound, error)
	check = func(path string) (bound, error) {
		if active[path] {
			return bound{}, ErrDeclaration
		}
		if b, ok := memo[path]; ok {
			return b, nil
		}
		active[path] = true
		b := bound{1, 1}
		for _, to := range edges[path] {
			child, err := check(to)
			if err != nil {
				return bound{}, err
			}
			b.work += child.work
			b.depth = max(b.depth, child.depth+1)
			if b.work > 4096 || b.depth > 32 {
				return bound{}, ErrDeclaration
			}
		}
		active[path] = false
		memo[path] = b
		return b, nil
	}
	_, err := check("")
	return err
}
