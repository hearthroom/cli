package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

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
