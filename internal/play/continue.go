package play

import (
	"fmt"
	"strings"
)

// OperationStatus is the public durable outcome shared by HTTP history and WS.
type OperationStatus struct {
	OperationID       string   `json:"operationId"`
	ConversationID    string   `json:"conversationId"`
	ClientOperationID string   `json:"clientOperationId"`
	SourceChatID      string   `json:"sourceChatId"`
	State             string   `json:"state"`
	AgentTurn         bool     `json:"agentTurn"`
	AllowedActions    []string `json:"allowedActions"`
	ChargeCredits     int64    `json:"chargeCredits"`
	Settled           bool     `json:"settled"`
}

func (s OperationStatus) CanContinue() bool {
	if !s.AgentTurn || (s.State != "failed_retryable" && s.State != "stopped") {
		return false
	}
	for _, action := range s.AllowedActions {
		if action == "continue" {
			return true
		}
	}
	return false
}

type Continuation struct {
	Operation OperationStatus
	Message   string
}

// PrepareContinuation uses only the latest authoritative outcome and its exact
// source USER. It never guesses a checkpoint or falls back to sending a new USER.
func PrepareContinuation(conversationID string, page *MessagesResponse) (*Continuation, error) {
	if page == nil || !page.OperationStatusAvailable {
		return nil, fmt.Errorf("cannot confirm the previous generation status; continuation has not started")
	}
	if len(page.Operations) == 0 {
		return nil, fmt.Errorf("this conversation has no generation to continue")
	}
	op := page.Operations[len(page.Operations)-1]
	if !op.CanContinue() || op.OperationID == "" || op.ConversationID != conversationID {
		return nil, fmt.Errorf("the latest generation cannot be continued")
	}
	if op.SourceChatID == "" {
		return nil, fmt.Errorf("cannot find the original message for this generation; saved preparation cannot be continued")
	}
	for _, m := range page.Chats {
		if m.ChatID == op.SourceChatID && strings.EqualFold(m.Role, "user") && m.Text != "" {
			return &Continuation{op, m.Text}, nil
		}
	}
	return nil, fmt.Errorf("cannot find the original message for this generation; saved preparation cannot be continued")
}
