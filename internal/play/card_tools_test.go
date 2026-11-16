package play

import (
	"context"
	"encoding/json"
	"github.com/hearthroom/cli/internal/api"
	"testing"
)

func TestTurnCardToolPayloadAndTerminalResult(t *testing.T) {
	script := []string{
		"id: 1\nevent: operationStatus\ndata: {\"operationId\":\"synthetic-op\",\"state\":\"completed\",\"toolCall\":{\"name\":\"report_turn\",\"args\":{\"move\":\"a\"},\"valid\":true}}\n\n",
		"id: 2\nevent: done\ndata: [DONE]\n\n",
	}
	srv := fakeStream(t, script, "", "", "", "report_turn")
	c := api.New(srv.URL, "http://site", "t", func(context.Context) (string, error) { return "tok", nil })
	opts := TurnOptions{ConversationID: "conv1", Message: "hi", AllowSpend: true}
	_ = json.Unmarshal([]byte(`{"Tool":"report_turn"}`), &opts)
	result, err := Turn(context.Background(), c, opts)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out["toolCall"] == nil || result.Error != "" {
		t.Fatalf("lost terminal tool result: %s", raw)
	}
}
func TestTurnCardToolRejectsLegacyProviderBeforeSending(t *testing.T) {
	srv := fakeStream(t, []string{"event: done\ndata: [DONE]\n\n"}, "")
	c := api.New(srv.URL, "http://site", "t", func(context.Context) (string, error) { return "tok", nil })
	opts := TurnOptions{ConversationID: "conv1", Message: "hi", AllowSpend: true}
	_ = json.Unmarshal([]byte(`{"Tool":"report_turn"}`), &opts)
	if _, err := Turn(context.Background(), c, opts); err == nil {
		t.Fatal("legacy Provider silently accepted selected tool as plain text")
	}
}
