package cardtools_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/cardtools"
)

const schema = `{"type":"object","$defs":{"id":{"type":"string","enum":["a","b"]}},"properties":{"action":{"$ref":"#/$defs/id"},"text":{"type":"string","maxLength":2}},"required":["action","text"],"additionalProperties":false}`

func TestTerminalArgumentsAreSchemaValidatedWithoutRepair(t *testing.T) {
	set, err := cardtools.Compile([]cardtools.Definition{{Name: "report_turn", Description: "Return a decision.", Schema: json.RawMessage(schema)}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args  string
		valid bool
	}{
		{`{"action":"a","text":"海港"}`, true},
		{`{"action":"b","text":"😀海"}`, true},
		{`{"action":"c","text":"港"}`, false},
		{`{"action":"a","text":"abc"}`, false},
		{`{"action":"a","text":"海","extra":1}`, false},
		{`{"action":"a","text":`, false},
		{`{"action":"a","action":"b","text":"海"}`, false},
	} {
		got := set.ValidateCall("report_turn", tc.args)
		if got.Valid != tc.valid {
			t.Errorf("valid=%v want=%v code=%s", got.Valid, tc.valid, got.Error)
		}
		if !got.Valid && got.Args != tc.args {
			t.Fatal("invalid arguments must remain an inert original string")
		}
		if got.Valid && got.Error != "" {
			t.Fatal("valid result has an error")
		}
	}
}

func TestReferenceExpansionHasAWorkBound(t *testing.T) {
	defs := map[string]any{"leaf": map[string]any{"type": "string"}}
	previous := "leaf"
	for i := 0; i < 14; i++ {
		name := fmt.Sprintf("node%d", i)
		ref := map[string]any{"$ref": "#/$defs/" + previous}
		defs[name] = map[string]any{"allOf": []any{ref, ref}}
		previous = name
	}
	raw, _ := json.Marshal(map[string]any{"type": "object", "$defs": defs, "properties": map[string]any{"x": map[string]any{"$ref": "#/$defs/" + previous}}})
	if _, err := cardtools.Compile([]cardtools.Definition{{Name: "bounded", Schema: raw}}); err == nil {
		t.Fatal("exponentially expanded schema accepted")
	}
}

func TestUnselectedToolCannotBypassArgumentLimit(t *testing.T) {
	set, err := cardtools.Compile([]cardtools.Definition{{Name: "report", Schema: json.RawMessage(`{"type":"object"}`)}})
	if err != nil {
		t.Fatal(err)
	}
	result := set.ValidateCall("unknown", strings.Repeat("x", cardtools.MaxArgumentsBytes+1))
	if result.Valid || result.Args != "" || result.Error != "arguments_limit" {
		t.Fatal("unknown tool bypassed the argument size guard")
	}
}

func TestDeclarationRejectsUnboundedOrAmbiguousSchemas(t *testing.T) {
	for _, raw := range []string{
		`{"type":"object","$ref":"https://example.invalid/private"}`,
		`{"type":"object","$ref":"#"}`,
		`{"type":"object","$defs":{"a":{"$ref":"#/$defs/b"},"b":{"$ref":"#/$defs/a"}},"properties":{"a":{"$ref":"#/$defs/a"}}}`,
		`{"type":"object","properties":{"x":{"mysteryKeyword":true}}}`,
		`{"type":"object","type":"string"}`,
		`{"type":"object","required":"wrong"}`,
		`{"type":"array"}`,
	} {
		if _, err := cardtools.Compile([]cardtools.Definition{{Name: "report", Schema: json.RawMessage(raw)}}); err == nil {
			t.Errorf("accepted invalid schema: %s", raw)
		}
	}
	for _, name := range []string{"submit_reply", "write_draft", "finish_preparation", "record_format_notes", "bad-name", strings.Repeat("a", 65)} {
		if _, err := cardtools.Compile([]cardtools.Definition{{Name: name, Schema: json.RawMessage(`{"type":"object"}`)}}); err == nil {
			t.Errorf("accepted invalid/reserved tool name: %s", name)
		}
	}
	def := cardtools.Definition{Name: "report", Schema: json.RawMessage(`{"type":"object"}`)}
	if _, err := cardtools.Compile([]cardtools.Definition{def, def}); err == nil {
		t.Fatal("duplicate names accepted")
	}
	defs := make([]cardtools.Definition, 17)
	if _, err := cardtools.Compile(defs); err == nil {
		t.Fatal("tool count limit not enforced")
	}
	def.Description = strings.Repeat("x", 2049)
	if _, err := cardtools.Compile([]cardtools.Definition{def}); err == nil {
		t.Fatal("description limit not enforced")
	}
}

func TestSchemaCombinationsAndPropertyNames(t *testing.T) {
	set, err := cardtools.Compile([]cardtools.Definition{{Name: "command", Schema: json.RawMessage(`{"type":"object","properties":{"choices":{"type":"object","propertyNames":{"enum":["a","b","c"]},"additionalProperties":{"type":"string"}},"command":{"anyOf":[{"type":"string","pattern":"^move_[a-z]+$"},{"type":"null"}]}},"required":["choices","command"]}`)}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args  string
		valid bool
	}{
		{`{"choices":{"a":"go"},"command":"move_east"}`, true},
		{`{"choices":{"a":"go"},"command":null}`, true},
		{`{"choices":{"d":"bad"},"command":"move_east"}`, false},
		{`{"choices":{"a":"go"},"command":"explain move_east"}`, false},
	} {
		if got := set.ValidateCall("command", tc.args); got.Valid != tc.valid {
			t.Errorf("valid=%v want=%v", got.Valid, tc.valid)
		}
	}
}
