package card

import (
	"encoding/json"
	"testing"
)

func TestSemanticRecallSurvivesFolderPayloadAndPull(t *testing.T) {
	f := sample(t)
	var book Lorebook
	if err := json.Unmarshal([]byte(`{"name":"Test","entries":[{"name":"Secret","content":"Reveal later","keywords":["reveal"],"semanticRecall":false}]}`), &book); err != nil {
		t.Fatal(err)
	}
	f.Lorebook = &book
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := loaded.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(payload.Worldbook)
	var wire struct {
		Entries []map[string]any `json:"entries"`
	}
	json.Unmarshal(raw, &wire)
	if wire.Entries[0]["semanticRecall"] != false {
		t.Fatalf("explicit off lost in push: %s", raw)
	}
	var remote RemoteEntry
	json.Unmarshal([]byte(`{"entryId":"remote","name":"Secret","content":"Reveal later","isEnabled":true,"semanticRecall":false}`), &remote)
	f = FromRemote(t.TempDir(), RemoteDetail{}, "book", "Test", []RemoteEntry{remote}, nil)
	raw, _ = json.Marshal(f.Lorebook)
	json.Unmarshal(raw, &wire)
	if wire.Entries[0]["semanticRecall"] != false {
		t.Fatalf("explicit off lost in pull: %s", raw)
	}
}
