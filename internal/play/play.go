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

// ErrGreetingNotApplied is returned when a fresh conversation could not be
// opened with the requested greeting because the card already had a current one.
var ErrGreetingNotApplied = errors.New("the card already had a current conversation, so --greeting was not applied; run the command again")

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
		ConversationID      string `json:"conversationId"`
		GreetingIndex       *int   `json:"greetingIndex"`
		HistoryConversation bool   `json:"historyConversation"`
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
		if fresh.HistoryConversation {
			// The server handed back a conversation that already existed (a
			// concurrent start, or a newer revision of the card). It may hold the
			// player's messages, so it is never deleted here.
			return nil, ErrGreetingNotApplied
		}
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
	Total                    int64             `json:"total"`
	Size                     int               `json:"size"`
	Pages                    int               `json:"pages"`
	HasNextPage              bool              `json:"hasNextPage"`
	Chats                    []Message         `json:"chats"`
	Operations               []OperationStatus `json:"operations,omitempty"`
	OperationStatusAvailable bool              `json:"operationStatusAvailable"`
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
	DebugRecall           bool
	ConversationID        string
	Message               string
	Model                 string
	ClientOperationID     string
	OperationKind         string // send | rewrite_response | retry_generation
	ResumeFromOperationID string
	Language              string
	AgentMode             *bool
	ThinkingDepth         string
	// AllowSpend must be true; the function refuses otherwise.
	AllowSpend bool
	// OnEvent receives every server event in order.
	OnEvent func(Event)
}

// TurnResult summarises a finished turn.
type TurnResult struct {
	RecallDebug  json.RawMessage `json:"recallDebug,omitempty"`
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
//
// The provider keeps generating when the socket drops, so a stream that closes
// before done is reattached to the same reply from the last event received
// (resumeStreamId/lastEventId). Without a stream id yet, the operation is found
// in the free history by this turn's client operation id. Resuming never sends
// the message again.
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
	if opts.ResumeFromOperationID != "" {
		opts.OperationKind = "retry_generation"
	}
	if opts.OperationKind == "" {
		opts.OperationKind = "send"
	}
	conn, err := openStream(ctx, c, nil, opts.OnEvent)
	if err != nil {
		return nil, err
	}
	defer func() { conn.CloseNow() }()

	frame := map[string]any{
		"conversationId":    opts.ConversationID,
		"message":           opts.Message,
		"clientOperationId": opts.ClientOperationID,
		"operationKind":     opts.OperationKind,
		"from":              "hearthroom-cli",
	}
	if opts.DebugRecall {
		frame["debug"] = "recall"
	}
	if opts.ResumeFromOperationID != "" {
		frame["resumeFromOperationId"] = opts.ResumeFromOperationID
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
	defer func() {
		if !opts.DebugRecall || len(res.RecallDebug) > 0 {
			return
		}
		res.RecallDebug = []byte(`{"schemaVersion":"recall_debug_v1","status":"unsupported"}`)
		if res.StreamID == "" {
			return
		}
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if debug, readErr := ReadRecallDebug(readCtx, c, res.StreamID); readErr == nil {
			res.RecallDebug = debug
		} else {
			var response *api.Error
			if !errors.As(readErr, &response) || response.Status != http.StatusNotFound {
				res.RecallDebug = []byte(`{"schemaVersion":"recall_debug_v1","status":"unavailable"}`)
			}
		}
	}()
	s := &turnStream{res: res, onEvent: opts.OnEvent}
	dropped, err := s.read(ctx, conn)
	for attempt := 1; dropped && attempt <= resumeAttempts; attempt++ {
		opID := res.StreamID
		if opID == "" {
			opID = findOperation(ctx, c, opts.ConversationID, opts.ClientOperationID)
		}
		if opID == "" {
			break
		}
		if opts.OnEvent != nil {
			opts.OnEvent(Event{Type: "reconnecting", Data: json.RawMessage(fmt.Sprintf(`{"attempt":%d,"operationId":%q}`, attempt, opID))})
		}
		select {
		case <-ctx.Done():
			s.finish()
			res.Error = "interrupted"
			return res, ctx.Err()
		case <-time.After(resumeBackoff * time.Duration(attempt)):
		}
		next, openErr := openStream(ctx, c, url.Values{"resumeStreamId": {opID}, "lastEventId": {strconv.FormatInt(s.lastSeq, 10)}}, opts.OnEvent)
		if openErr != nil {
			if ctx.Err() != nil {
				s.finish()
				res.Error = "interrupted"
				return res, ctx.Err()
			}
			continue
		}
		conn.CloseNow()
		conn = next
		dropped, err = s.read(ctx, conn)
	}
	if dropped {
		s.finish()
		res.Error = StreamClosedBeforeDone
	}
	return res, err
}

// StreamClosedBeforeDone is TurnResult.Error when the socket dropped and could
// not be reattached. The provider may still finish and save the reply.
const StreamClosedBeforeDone = "stream closed before done"

// resumeAttempts bounds how often a dropped stream is reattached.
const resumeAttempts = 3

// pingInterval matches the provider's control protocol: clients ping every ten
// seconds. A turn that is preparing (summarising memory, an Agent step) sends
// no reply text for a while, and a proxy on the way may drop a socket whose
// client side stays silent.
var (
	pingInterval  = 10 * time.Second
	resumeBackoff = time.Second
)

// openStream gets a ticket, connects and completes the handshake. extra adds
// query parameters (a resume names the stream and the last event received).
func openStream(ctx context.Context, c *api.Client, extra url.Values, onEvent func(Event)) (*websocket.Conn, error) {
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
	q := url.Values{"protocolVersion": {"2"}}
	for k, v := range extra {
		q[k] = v
	}
	wsURL := strings.Replace(strings.Replace(c.API, "https://", "wss://", 1), "http://", "ws://", 1) + "/open/v1/conversation/ws?" + q.Encode()
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
	if err := writeJSON(ctx, conn, map[string]string{"type": "auth", "ticket": ticket.Ticket}); err != nil {
		conn.CloseNow()
		return nil, err
	}
	ev, err := readEvent(ctx, conn)
	if err != nil {
		conn.CloseNow()
		return nil, fmt.Errorf("stream handshake: %w", err)
	}
	if ev.Type != "ready" {
		conn.CloseNow()
		return nil, fmt.Errorf("stream handshake rejected: %s %s", ev.Type, truncate(string(ev.Data), 200))
	}
	if onEvent != nil {
		onEvent(ev)
	}
	return conn, nil
}

// findOperation looks up this turn's operation in the free history by its
// client operation id. Empty means not found (refused before admission, or the
// history could not be read).
func findOperation(ctx context.Context, c *api.Client, conversationID, clientOperationID string) string {
	page, err := History(ctx, c, conversationID, 1, 20)
	if err != nil {
		return ""
	}
	for _, op := range page.Operations {
		if op.ClientOperationID == clientOperationID {
			return op.OperationID
		}
	}
	return ""
}

// turnStream accumulates one turn across the connections that carry it.
type turnStream struct {
	res              *TurnResult
	onEvent          func(Event)
	answer, thinking strings.Builder
	lastSeq          int64
}

func (s *turnStream) finish() {
	s.res.Answer, s.res.Thinking = s.answer.String(), s.thinking.String()
}

// read consumes one connection until done, an error frame or cancellation.
// dropped reports a socket that closed before the turn ended.
func (s *turnStream) read(ctx context.Context, conn *websocket.Conn) (dropped bool, err error) {
	pingCtx, stopPing := context.WithCancel(ctx)
	defer stopPing()
	go func() {
		t := time.NewTicker(pingInterval)
		defer t.Stop()
		for {
			select {
			case <-pingCtx.Done():
				return
			case <-t.C:
				if writeJSON(pingCtx, conn, map[string]string{"type": "ping"}) != nil {
					return
				}
			}
		}
	}()
	res := s.res
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				// Interrupted: ask the server to stop, then report what we have.
				stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_ = writeJSON(stopCtx, conn, map[string]string{"action": "stop"})
				cancel()
				s.finish()
				res.Error = "interrupted"
				return false, ctx.Err()
			}
			// A close, EOF or reset all mean the socket is gone while the
			// provider may still be generating.
			s.finish()
			return res.Error == "" && res.FinishReason == "", nil
		}
		ev, err := ParseFrame(string(raw))
		if err != nil {
			s.finish()
			return false, err
		}
		res.Events++
		if ev.ID > s.lastSeq {
			s.lastSeq = ev.ID
		}
		if s.onEvent != nil {
			s.onEvent(ev)
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
			s.answer.WriteString(ev.Text)
			if ev.Final != nil && *ev.Final != "" {
				res.FinishReason = *ev.Final
			}
		case "thinking":
			s.thinking.WriteString(ev.Text)
		case "recallDebug":
			var payload struct {
				Debug json.RawMessage `json:"recallDebug"`
			}
			if json.Unmarshal(ev.Data, &payload) == nil {
				res.RecallDebug = payload.Debug
			}
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
			s.finish()
			return false, nil
		case "done", "sessionExpired", "turn_already_consumed":
			s.finish()
			if ev.Type != "done" {
				res.Error = ev.Type
			}
			return false, nil
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

// ReadRecallDebug reads only the capture attached to an existing paid operation.
// It never submits a turn or retries a generation.
func ReadRecallDebug(ctx context.Context, c *api.Client, operation string) (json.RawMessage, error) {
	var out struct {
		Debug json.RawMessage `json:"recallDebug"`
	}
	if err := c.OpenGet(ctx, "/conversation/operations/"+url.PathEscape(operation), url.Values{"include": {"recallDebug"}}, &out); err != nil {
		return nil, err
	}
	if len(out.Debug) == 0 {
		return json.RawMessage(`{"schemaVersion":"recall_debug_v1","status":"unsupported"}`), nil
	}
	return out.Debug, nil
}
