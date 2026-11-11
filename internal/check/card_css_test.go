package check

import (
	"strings"
	"testing"
)

func TestCardCSSCheckBounds(t *testing.T) {
	for _, tc := range []struct {
		css, dialect string
		valid        bool
	}{
		{strings.Repeat("x", 524288), "", true},
		{strings.Repeat("界", 174763), "", false},
		{"p{color:red}", "fengyue", true},
		{"\r\n#ai-chat-input{border-width:1px}\r\n", "fengyue-v2", true},
		{"p{color:red}", "unknown", false},
	} {
		dir := write(t, t.TempDir(), map[string]string{"card.json": `{"formatVersion":1,"name":"Synthetic","language":"en"}`, "card.css": tc.css, "rules.json": `{"rules":[],"cardCssDialect":"` + tc.dialect + `"}`})
		r, err := Card(dir)
		if err != nil {
			t.Fatal(err)
		}
		if (r.Errors() == 0) != tc.valid {
			t.Errorf("valid=%v findings=%v", tc.valid, r.Findings)
		}
	}
}
