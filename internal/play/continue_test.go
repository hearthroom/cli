package play

import "testing"

func TestContinuationDenials(t *testing.T) {
	valid := OperationStatus{OperationID: "parent", ConversationID: "c1", SourceChatID: "u1", State: "failed_retryable", AgentTurn: true, AllowedActions: []string{"continue"}}
	for _, tc := range []struct {
		name   string
		change func(*OperationStatus, *Message)
	}{
		{"not_agent", func(o *OperationStatus, _ *Message) { o.AgentTurn = false }},
		{"not_allowed", func(o *OperationStatus, _ *Message) { o.AllowedActions = []string{"retry"} }},
		{"running", func(o *OperationStatus, _ *Message) { o.State = "generating" }},
		{"foreign_conversation", func(o *OperationStatus, _ *Message) { o.ConversationID = "other" }},
		{"missing_operation", func(o *OperationStatus, _ *Message) { o.OperationID = "" }},
		{"missing_source", func(o *OperationStatus, m *Message) { o.SourceChatID = ""; m.ChatID = "" }},
		{"not_user", func(_ *OperationStatus, m *Message) { m.Role = "AI" }},
		{"blank_message", func(_ *OperationStatus, m *Message) { m.Text = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := valid
			m := Message{ChatID: "u1", Role: "USER", Text: "original"}
			tc.change(&o, &m)
			page := &MessagesResponse{OperationStatusAvailable: true, Operations: []OperationStatus{o}, Chats: []Message{m}}
			if _, err := PrepareContinuation("c1", page); err == nil {
				t.Fatal("unsafe source accepted")
			}
		})
	}
}
