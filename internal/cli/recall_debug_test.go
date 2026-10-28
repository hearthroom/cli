package cli

import (
	"encoding/json"
	"github.com/hearthroom/cli/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecallDebugRejectsMissingSpendBeforeStartingConversation(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"conversationId":"conv1"}`))
	}))
	defer srv.Close()
	_, _, err := runCLI(t, "play", "--role", "r1", "--message", "hi", "--debug-recall", "--new-session", "--api", srv.URL, "--config-dir", cfg)
	if err == nil || !strings.Contains(err.Error(), recallLabel("invalid_use")) || calls != 0 {
		t.Fatalf("invalid debug request reached conversation mutation: calls=%d err=%v", calls, err)
	}
}

func TestRecallDebugPresentationDistinguishesMissingPartialAndEmpty(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{{`{"status":"ready"}`, "These details contain no entries from your own Lorebooks that can be displayed."}, {`{"status":"not_recorded"}`, "Recall details were not enabled for this turn and cannot be collected later."}, {`{"status":"ready","partial":true}`, "Recall details are incomplete. Some items or content are not shown."}, {`{"status":"unsupported"}`, "The service did not return recall details for this turn. Normal chat billing still applies."}} {
		out := formatRecallDebug(json.RawMessage(tc.raw))
		if !strings.Contains(out, tc.want) {
			t.Fatalf("status message missing: %s", out)
		}
		if !strings.Contains(tc.raw, `"ready"`) && strings.Contains(out, "These details contain no entries") {
			t.Fatal("missing data portrayed as an empty set")
		}
	}
	out := formatRecallDebug(json.RawMessage(`{"status":"ready","selections":[{"scope":"role","entries":[{"name":"\u001b[2JOwned","decision":"excluded","lifecycle":"new","source":"none","reason":"secondary_veto","vectorScore":0}]}]}`))
	if strings.Contains(out, "\x1b") || strings.Contains(out, "secondary_veto") || !strings.Contains(out, "Secondary condition not met") || !strings.Contains(out, "Vector similarity: 0.0000") || strings.Contains(out, "Vector rank: 0") {
		t.Fatal("raw enum, missing zero semantics or terminal control escaped")
	}
}

func TestRecallDebugToolContentDisclosesTruncation(t *testing.T) {
	out := formatRecallDebug(json.RawMessage(`{"status":"ready","partial":true,"tools":[{"sequence":1,"tool":"read","result":"completed","hits":[{"entryId":"synthetic-entry","kind":"full","content":"Visible author excerpt","truncated":true}]}]}`))
	if !strings.Contains(out, recallLabel("content_truncated")) {
		t.Fatal("tool content omits its truncation notice")
	}
}
