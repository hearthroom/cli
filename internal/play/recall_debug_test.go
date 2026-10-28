package play

import (
	"context"
	"encoding/json"
	"github.com/hearthroom/cli/internal/api"
	"net/http"
	"strings"
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

func TestRecallDebugRecoversSavedDetailsOnErrorAndDisconnect(t *testing.T) {
	for _, terminal := range []string{"event: error\ndata: {\"code\":\"upstream_error\"}\n\n", ""} {
		t.Run(terminal, func(t *testing.T) {
			script := []string{"event: streamMeta\ndata: {\"streamId\":\"saved-op\"}\n\n"}
			if terminal != "" {
				script = append(script, terminal)
			}
			srv := fakeStream(t, script, "", "", "recall")
			handler := srv.Config.Handler
			reads := 0
			srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/open/v1/conversation/operations/saved-op" {
					reads++
					if r.URL.Query().Get("include") != "recallDebug" {
						t.Error("not reading saved details")
					}
					w.Write([]byte(`{"recallDebug":{"schemaVersion":"recall_debug_v1","status":"ready","partial":true}}`))
					return
				}
				handler.ServeHTTP(w, r)
			})
			res, err := Turn(context.Background(), api.New(srv.URL, "http://site", "t", func(context.Context) (string, error) { return "tok", nil }), TurnOptions{ConversationID: "conv1", Message: "hi", AllowSpend: true, DebugRecall: true})
			if err != nil || reads != 1 || !strings.Contains(string(res.RecallDebug), `"partial":true`) {
				t.Fatalf("lost saved diagnostics on failure: reads=%d res=%+v err=%v", reads, res, err)
			}
		})
	}
}
