package card

import (
	"encoding/json"
	"testing"
)

func TestCardToolsRoundTripAndExplicitClear(t *testing.T) {
	for _, raw := range []string{`[{"name":"report_turn","description":"Synthetic.","schema":{"type":"object"}}]`, `[]`} {
		var m Manifest
		if err := json.Unmarshal([]byte(`{"formatVersion":1,"name":"Synthetic","tools":`+raw+`}`), &m); err != nil {
			t.Fatal(err)
		}
		p, err := (&Folder{Manifest: m}).Build(nil)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := p.Card["tools"]
		if !ok {
			t.Fatal("card.json tools disappeared from the push payload")
		}
		encoded, _ := json.Marshal(got)
		var want, actual any
		_ = json.Unmarshal([]byte(raw), &want)
		_ = json.Unmarshal(encoded, &actual)
		a, _ := json.Marshal(actual)
		b, _ := json.Marshal(want)
		if string(a) != string(b) {
			t.Fatalf("tools changed: %s", encoded)
		}
		var remote RemoteDetail
		if err := json.Unmarshal([]byte(`{"roleName":"Synthetic","tools":`+raw+`}`), &remote); err != nil {
			t.Fatal(err)
		}
		back := FromRemote(t.TempDir(), remote, "", "", nil, nil)
		saved, err := json.Marshal(back.Manifest)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		_ = json.Unmarshal(saved, &fields)
		if _, ok := fields["tools"]; !ok {
			t.Fatal("pull lost tools declaration or explicit clear")
		}
	}
	p, err := (&Folder{Manifest: Manifest{Name: "Synthetic"}}).Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := p.Card["tools"]; exists {
		t.Fatal("omission must preserve server declaration")
	}
}
func TestCardToolsRejectInvalidDeclarationBeforeMapDecoding(t *testing.T) {
	for _, raw := range []string{
		`[{"name":"report_turn","description":"Synthetic.","schema":{"type":"object","type":"object"}}]`,
		`[{"name":"report_turn","description":"Synthetic.","schema":{"type":"object","$ref":"https://example.test/private"}}]`,
		`[{"name":"submit_reply","description":"Synthetic.","schema":{"type":"object"}}]`,
	} {
		var m Manifest
		err := json.Unmarshal([]byte(`{"formatVersion":1,"name":"Synthetic","tools":`+raw+`}`), &m)
		if err == nil {
			_, err = (&Folder{Manifest: m}).Build(nil)
		}
		if err == nil {
			t.Fatal("invalid tools declaration accepted")
		}
	}
}
