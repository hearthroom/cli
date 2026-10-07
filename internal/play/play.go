// Package play drives a conversation with a card: start/resume, history,
// one streamed turn over the provider's WebSocket, and stop. Generation
// spends credits, so the caller must opt in explicitly.
package play

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/hearthroom/cli/internal/api"
)

// ErrSpendNotAllowed is returned when a paid action was requested without
// the explicit flag.
var ErrSpendNotAllowed = errors.New("sending a message spends credits; re-run with --allow-spend to confirm")

// StartResponse is POST /open/v1/conversation/start.
type StartResponse struct {
	ConversationID      string `json:"conversationId"`
	DefaultRelay        string `json:"defaultRelay"`
	HistoryConversation bool   `json:"historyConversation"`
	RoleInfo            struct {
		RoleName string `json:"roleName"`
	} `json:"roleInfo"`
	FirstPage *MessagesResponse `json:"firstPage"`
}

// Start opens or resumes the current conversation of a role. It does not
// generate and does not spend.
func Start(ctx context.Context, c *api.Client, roleID string, greetingIndex, firstPageSize int) (*StartResponse, error) {
	body := map[string]any{"roleId": roleID, "greetingIndex": greetingIndex}
	if firstPageSize > 0 {
		body["firstPageSize"] = firstPageSize
	}
	var out StartResponse
	if err := c.OpenPost(ctx, "/conversation/start", body, &out); err != nil {
		return nil, fmt.Errorf("start conversation: %w", err)
	}
	if out.ConversationID == "" {
		return nil, errors.New("start conversation: no conversationId in response")
	}
	return &out, nil
}

// StartNew archives the current conversation with the card (the provider
// keeps it as history when it has a user message, otherwise discards it) and
// opens a fresh one. Two folders that share a card no longer step on each
// other's conversation.
//
// save-and-start-new opens the next conversation itself. A server that knows
// greetingIndex opens it with the requested greeting and echoes the index back.
// An older server always uses the main opening, so a later start would only
// resume it: then the fresh conversation (no user message yet) is deleted and
// started again with the requested greeting.
func StartNew(ctx context.Context, c *api.Client, roleID string, greetingIndex, firstPageSize int) (*StartResponse, error) {
	current, err := Start(ctx, c, roleID, 0, 0)
	if err != nil {
		return nil, err
	}
	var fresh struct {
		ConversationID string `json:"conversationId"`
		GreetingIndex  *int   `json:"greetingIndex"`
	}
	body := map[string]any{"conversationId": current.ConversationID, "save": true}
	if greetingIndex != 0 {
		body["greetingIndex"] = greetingIndex
	}
	if err := c.OpenPost(ctx, "/conversation/save-and-start-new", body, &fresh); err != nil {
		return nil, fmt.Errorf("start new conversation: %w", err)
	}
	applied := fresh.GreetingIndex != nil && *fresh.GreetingIndex == greetingIndex
	if greetingIndex != 0 && !applied {
		if fresh.ConversationID == "" {
			again, err := Start(ctx, c, roleID, 0, 0)
			if err != nil {
				return nil, err
			}
			fresh.ConversationID = again.ConversationID
		}
		if err := c.OpenPost(ctx, "/conversation/delete", map[string]any{"conversationId": fresh.ConversationID}, nil); err != nil {
			return nil, fmt.Errorf("start new conversation with opening %d: %w", greetingIndex, err)
		}
	}
	out, err := Start(ctx, c, roleID, greetingIndex, firstPageSize)
	if err != nil {
		return nil, err
	}
	// The conversation was opened by this call even when the last start
	// resumed the one save-and-start-new had just made.
	out.HistoryConversation = false
	return out, nil
}

// Message is one row of a conversation.
type Message struct {
	ID           int64  `json:"id"`
	ChatID       string `json:"chatId"`
	Role         string `json:"chatRole"`
	Text         string `json:"chatMessage"`
	Thinking     string `json:"contentThinking,omitempty"`
	IsFirst      bool   `json:"isFirst"`
	IsComplete   bool   `json:"isComplete"`
	IsSummary    bool   `json:"isSummary"`
	Model        string `json:"model,omitempty"`
	FinishReason string `json:"finishReason,omitempty"`
	CreateTime   string `json:"createTime"`
}

// MessagesResponse is GET /open/v1/conversation/messages.
type MessagesResponse struct {
	Total       int64     `json:"total"`
	Size        int       `json:"size"`
	Pages       int       `json:"pages"`
	HasNextPage bool      `json:"hasNextPage"`
	Chats       []Message `json:"chats"`
}

// History reads one page of messages (newest first, as the API returns).
func History(ctx context.Context, c *api.Client, conversationID string, page, size int) (*MessagesResponse, error) {
	q := url.Values{"conversationId": {conversationID}, "pageNum": {strconv.Itoa(page)}, "pageSize": {strconv.Itoa(size)}}
	var out MessagesResponse
	if err := c.OpenGet(ctx, "/conversation/messages", q, &out); err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}
	return &out, nil
}

// Stop asks the provider to cancel the running generation.
func Stop(ctx context.Context, c *api.Client, conversationID, streamID string) error {
	body := map[string]any{"conversationId": conversationID}
	if streamID != "" {
		body["streamId"] = streamID
	}
	if err := c.OpenPost(ctx, "/conversation/stop", body, nil); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	return nil
}

// Event is one server frame, decoded enough for a CLI to act on.
type Event struct {
	ID    int64           `json:"id,omitempty"`
	Type  string          `json:"event"`
	Data  json.RawMessage `json:"data,omitempty"`
	Text  string          `json:"text,omitempty"` // answer/thinking delta text
	Raw   string          `json:"-"`
	Final *string         `json:"finishReason,omitempty"`
}

// ParseFrame decodes an SSE-formatted WebSocket text frame.
func ParseFrame(frame string) (Event, error) {
	ev := Event{Raw: frame}
	var dataLines []string
	sc := bufio.NewScanner(strings.NewReader(frame))
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "id:"):
			ev.ID, _ = strconv.ParseInt(strings.TrimSpace(line[3:]), 10, 64)
		case strings.HasPrefix(line, "event:"):
			ev.Type = strings.TrimSpace(line[6:])
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	data := strings.Join(dataLines, "\n")
	if ev.Type == "" {
		// Raw JSON frames (e.g. {"error": "invalid request body"}).
		trimmed := strings.TrimSpace(frame)
		if strings.HasPrefix(trimmed, "{") {
			ev.Type = "error"
			ev.Data = json.RawMessage(trimmed)
			return ev, nil
		}
		return ev, fmt.Errorf("unrecognised frame: %q", truncate(frame, 120))
	}
	if data == "[DONE]" {
		ev.Data = json.RawMessage(`"[DONE]"`)
		return ev, nil
	}
	if json.Valid([]byte(data)) {
		ev.Data = json.RawMessage(data)
	} else {
		ev.Data, _ = json.Marshal(data)
	}
	if ev.Type == "answer" || ev.Type == "thinking" {
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					Reasoning string `json:"reasoning_content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err == nil && len(chunk.Choices) > 0 {
			if ev.Type == "answer" {
				ev.Text = chunk.Choices[0].Delta.Content
			} else {
				ev.Text = chunk.Choices[0].Delta.Reasoning
			}
			ev.Final = chunk.Choices[0].FinishReason
		}
	}
	return ev, nil
}

// TurnOptions describe one streamed turn.
type TurnOptions struct {
	ConversationID    string
	Message           string
	Model             string
	ClientOperationID string
	OperationKind     string // send | rewrite_response
	Language          string
	AgentMode         *bool
	ThinkingDepth     string
	// AllowSpend must be true; the function refuses otherwise.
	AllowSpend bool
	// OnEvent receives every server event in order.
	OnEvent func(Event)
}

// TurnResult summarises a finished turn.
type TurnResult struct {
	StreamID     string          `json:"streamId,omitempty"`
	ChatID       string          `json:"chatId,omitempty"`
	Answer       string          `json:"answer"`
	Thinking     string          `json:"thinking,omitempty"`
	FinishReason string          `json:"finishReason,omitempty"`
	Status       json.RawMessage `json:"operationStatus,omitempty"`
	Error        string          `json:"error,omitempty"`
	ErrorType    string          `json:"errorType,omitempty"`
	Events       int             `json:"events"`
}

// NewClientOperationID returns a random idempotency key.
func NewClientOperationID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "cli-" + hex.EncodeToString(b[:])
}

// Turn sends one message and streams the reply. It obtains a ticket with
// the client's bearer, connects with protocolVersion 2 (the version providers implement today), authenticates, sends
// the chat frame and reads until done, error or context cancellation. On
// cancellation it sends a stop control frame before closing.
func Turn(ctx context.Context, c *api.Client, opts TurnOptions) (*TurnResult, error) {
	if !opts.AllowSpend {
		return nil, ErrSpendNotAllowed
	}
	if opts.ConversationID == "" {
		return nil, errors.New("conversationId is required")
	}
	if opts.ClientOperationID == "" {
		opts.ClientOperationID = NewClientOperationID()
	}
	if opts.OperationKind == "" {
		opts.OperationKind = "send"
	}
	var ticket struct {
		Ticket    string `json:"ticket"`
		ExpiresIn int    `json:"expiresIn"`
	}
	if err := c.OpenPost(ctx, "/conversation/ws-ticket", nil, &ticket); err != nil {
		return nil, fmt.Errorf("get stream ticket: %w", err)
	}
	if ticket.Ticket == "" {
		return nil, errors.New("get stream ticket: empty ticket")
	}
	wsURL := strings.Replace(strings.Replace(c.API, "https://", "wss://", 1), "http://", "ws://", 1) + "/open/v1/conversation/ws?protocolVersion=2"
	dialCtx, cancelDial := context.WithTimeout(ctx, 20*time.Second)
	defer cancelDial()
	conn, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
		HTTPClient: c.HTTP,
		HTTPHeader: http.Header{"User-Agent": {c.UserAgent}, "X-Client-Platform": {"web"}},
	})
	if err != nil {
		return nil, fmt.Errorf("connect stream: %w", err)
	}
	conn.SetReadLimit(64 << 20)
	defer conn.CloseNow()

	if err := writeJSON(ctx, conn, map[string]string{"type": "auth", "ticket": ticket.Ticket}); err != nil {
		return nil, err
	}
	ev, err := readEvent(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("stream handshake: %w", err)
	}
	if ev.Type != "ready" {
		return nil, fmt.Errorf("stream handshake rejected: %s %s", ev.Type, truncate(string(ev.Data), 200))
	}
	if opts.OnEvent != nil {
		opts.OnEvent(ev)
	}

	frame := map[string]any{
		"conversationId":    opts.ConversationID,
		"message":           opts.Message,
		"clientOperationId": opts.ClientOperationID,
		"operationKind":     opts.OperationKind,
		"from":              "hearthroom-cli",
	}
	if opts.Model != "" {
		frame["model"] = opts.Model
	}
	if opts.Language != "" {
		frame["language"] = opts.Language
	}
	if opts.AgentMode != nil {
		frame["agentMode"] = *opts.AgentMode
	}
	if opts.ThinkingDepth != "" {
		frame["thinkingDepth"] = opts.ThinkingDepth
	}
	if opts.OperationKind == "rewrite_response" {
		frame["rewrite"] = true
	}
	if err := writeJSON(ctx, conn, frame); err != nil {
		return nil, err
	}

	res := &TurnResult{}
	var answer, thinking strings.Builder
	for {
		ev, err := readEvent(ctx, conn)
		if err != nil {
			if ctx.Err() != nil {
				// Interrupted: ask the server to stop, then report what we have.
				stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_ = writeJSON(stopCtx, conn, map[string]string{"action": "stop"})
				cancel()
				res.Answer, res.Thinking = answer.String(), thinking.String()
				res.Error = "interrupted"
				return res, ctx.Err()
			}
			if errors.Is(err, io.EOF) || websocket.CloseStatus(err) != -1 {
				res.Answer, res.Thinking = answer.String(), thinking.String()
				if res.Error == "" && res.FinishReason == "" {
					res.Error = "stream closed before done"
				}
				return res, nil
			}
			return res, err
		}
		res.Events++
		if opts.OnEvent != nil {
			opts.OnEvent(ev)
		}
		switch ev.Type {
		case "streamMeta":
			var m struct {
				StreamID string `json:"streamId"`
			}
			_ = json.Unmarshal(ev.Data, &m)
			res.StreamID = m.StreamID
		case "accepted":
			var m struct {
				ChatID string `json:"chatId"`
			}
			_ = json.Unmarshal(ev.Data, &m)
			res.ChatID = m.ChatID
		case "answer":
			answer.WriteString(ev.Text)
			if ev.Final != nil && *ev.Final != "" {
				res.FinishReason = *ev.Final
			}
		case "thinking":
			thinking.WriteString(ev.Text)
		case "operationStatus":
			res.Status = ev.Data
		case "error":
			var e struct {
				Error     string `json:"error"`
				ErrorType string `json:"error_type"`
				Code      string `json:"code"`
			}
			_ = json.Unmarshal(ev.Data, &e)
			res.Error = firstNonEmpty(e.Error, e.Code, "error")
			res.ErrorType = e.ErrorType
			res.Answer, res.Thinking = answer.String(), thinking.String()
			return res, nil
		case "done", "sessionExpired", "turn_already_consumed":
			res.Answer, res.Thinking = answer.String(), thinking.String()
			if ev.Type != "done" {
				res.Error = ev.Type
			}
			return res, nil
		}
	}
}

func writeJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		return fmt.Errorf("stream write: %w", err)
	}
	return nil
}

func readEvent(ctx context.Context, conn *websocket.Conn) (Event, error) {
	_, raw, err := conn.Read(ctx)
	if err != nil {
		return Event{}, err
	}
	return ParseFrame(string(raw))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
