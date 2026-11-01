package lorebook

import (
	"encoding/json"
	"testing"
)

func TestMarkdownSemanticRecall(t *testing.T) {
	for _, value := range []string{"false", "true", ""} {
		text := "---\nname: Secret\nkeywords: reveal\n"
		if value != "" {
			text += "semanticRecall: " + value + "\n"
		}
		s, err := ParseSource("secret.md", text+"---\nReveal later")
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(s.Entry)
		var entry map[string]any
		json.Unmarshal(raw, &entry)
		if value == "false" && entry["semanticRecall"] != false {
			t.Fatalf("Markdown off lost: %s", raw)
		}
		if value != "false" && entry["semanticRecall"] == false {
			t.Fatalf("default/on became off: %s", raw)
		}
	}
	if _, err := ParseSource("bad.md", "---\nsemanticRecall: maybe\n---\nContent"); err == nil {
		t.Fatal("invalid semanticRecall must be rejected")
	}
}
