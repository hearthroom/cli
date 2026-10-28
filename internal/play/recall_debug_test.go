package play

import (
	"context"
	"encoding/json"
	"github.com/hearthroom/cli/internal/api"
	"testing"
)

func TestRecallDebugTurnPreservesSeparatePayload(t *testing.T) {
	script := []string{
		"event: streamMeta\ndata: {\"streamId\":\"op1\"}\n\n",
		"event: recallDebug\ndata: {\"operationId\":\"op1\",\"recallDebug\":{\"schemaVersion\":\"recall_debug_v1\",\"status\":\"ready\",\"tools\":[]}}\n\n",
		"event: done\ndata: {}\n\n",
	}
	srv := fakeStream(t, script, "", "", "recall")
	res, err := Turn(context.Background(), api.New(srv.URL, "http://site", "t", func(context.Context) (string, error) { return "tok", nil }), TurnOptions{ConversationID: "conv1", Message: "hi", AllowSpend: true, DebugRecall: true})
	if err != nil {
		t.Fatal(err)
	}
	var debug map[string]any
	if json.Unmarshal(res.RecallDebug, &debug) != nil || debug["status"] != "ready" || res.Answer != "" {
		t.Fatalf("private debug mixed with answer: %+v", res)
	}
}
