package play

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/hearthroom/cli/internal/api"
)

// resumeServer serves one WebSocket connection per entry of conns. Each entry
// receives the connection's query and the conn after the handshake; the chat
// frame is read for the first connection only (a resume sends none).
type resumeServer struct {
	t       *testing.T
	mu      sync.Mutex
	queries []string
	conns   []func(ctx context.Context, conn *websocket.Conn)
	history func(w http.ResponseWriter, r *http.Request)
}

func (s *resumeServer) start() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/open/v1/conversation/ws-ticket", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ticket": "T1", "expiresIn": 30})
	})
	mux.HandleFunc("/open/v1/conversation/messages", func(w http.ResponseWriter, r *http.Request) {
		if s.history == nil {
			http.Error(w, `{"error":"not_found"}`, 404)
			return
		}
		s.history(w, r)
	})
	mux.HandleFunc("/open/v1/conversation/ws", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		n := len(s.queries)
		s.queries = append(s.queries, r.URL.RawQuery)
		s.mu.Unlock()
		if n >= len(s.conns) {
			http.Error(w, "unexpected connection", 500)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
		_ = conn.Write(ctx, websocket.MessageText, []byte("event: ready\ndata: {\"accountBound\":true}\n\n"))
		if n == 0 {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
		s.conns[n](ctx, conn)
	})
	srv := httptest.NewServer(mux)
	s.t.Cleanup(srv.Close)
	return srv
}

func write(ctx context.Context, conn *websocket.Conn, frames ...string) {
	for _, f := range frames {
		_ = conn.Write(ctx, websocket.MessageText, []byte(f))
	}
}

func fastResume(t *testing.T) {
	interval, backoff := pingInterval, resumeBackoff
	pingInterval, resumeBackoff = 20*time.Millisecond, time.Millisecond
	t.Cleanup(func() { pingInterval, resumeBackoff = interval, backoff })
}

// A turn that is still preparing (e.g. summarising memory) sends no reply text
// for a long while. The client must keep its side of the socket alive with the
// ping the provider expects, or a proxy on the way may drop the connection.
func TestTurnPingsWhileWaiting(t *testing.T) {
	fastResume(t)
	s := &resumeServer{t: t}
	s.conns = []func(context.Context, *websocket.Conn){func(ctx context.Context, conn *websocket.Conn) {
		write(ctx, conn, "event: streamMeta\ndata: {\"streamId\":\"s1\"}\n\n", "event: compacting\ndata: {\"reason\":\"memory\"}\n\n")
		readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_, raw, err := conn.Read(readCtx)
		if err != nil || string(raw) != `{"type":"ping"}` {
			t.Errorf("control frame = %q, %v; want a ping", raw, err)
			return
		}
		write(ctx, conn,
			"id: 4\nevent: answer\ndata: {\"choices\":[{\"delta\":{\"content\":\"Hi\"},\"finish_reason\":\"stop\"}]}\n\n",
			"event: done\ndata: [DONE]\n\n")
	}}
	srv := s.start()
	c := api.New(srv.URL, "http://site", "t", func(context.Context) (string, error) { return "tok", nil })
	res, err := Turn(context.Background(), c, TurnOptions{ConversationID: "conv1", Message: "hi", AllowSpend: true})
	if err != nil || res.Answer != "Hi" || res.Error != "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

// The provider keeps generating when the socket drops, so a dropped stream is
// reattached to the same reply from the last event received instead of being
// reported as a failure the player would pay to send again.
func TestTurnResumesTheSameReplyAfterADroppedStream(t *testing.T) {
	fastResume(t)
	s := &resumeServer{t: t}
	s.conns = []func(context.Context, *websocket.Conn){
		func(ctx context.Context, conn *websocket.Conn) {
			write(ctx, conn,
				"event: streamMeta\ndata: {\"streamId\":\"op1\"}\n\n",
				"event: accepted\ndata: {\"chatId\":\"c1\"}\n\n",
				"event: compacting\ndata: {\"reason\":\"memory\"}\n\n",
				"id: 7\nevent: answer\ndata: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n")
		},
		func(ctx context.Context, conn *websocket.Conn) {
			write(ctx, conn,
				"event: streamMeta\ndata: {\"streamId\":\"op1\"}\n\n",
				"id: 8\nevent: answer\ndata: {\"choices\":[{\"delta\":{\"content\":\", world\"},\"finish_reason\":null}]}\n\n",
				"event: answer\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n",
				"event: done\ndata: [DONE]\n\n")
		},
	}
	srv := s.start()
	c := api.New(srv.URL, "http://site", "t", func(context.Context) (string, error) { return "tok", nil })
	var seen []string
	res, err := Turn(context.Background(), c, TurnOptions{ConversationID: "conv1", Message: "hi", AllowSpend: true, OnEvent: func(ev Event) { seen = append(seen, ev.Type) }})
	if err != nil || res.Answer != "Hello, world" || res.FinishReason != "stop" || res.Error != "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(s.queries) != 2 || !strings.Contains(s.queries[1], "resumeStreamId=op1") || !strings.Contains(s.queries[1], "lastEventId=7") {
		t.Fatalf("queries = %v", s.queries)
	}
	if !strings.Contains(strings.Join(seen, ","), "reconnecting") {
		t.Fatalf("events = %v; want a reconnecting note", seen)
	}
}

// A drop before streamMeta leaves no stream id. The operation is found in the
// free history by this turn's client operation id, then resumed.
func TestTurnFindsTheOperationInHistoryWhenTheStreamDropsEarly(t *testing.T) {
	fastResume(t)
	var clientOp string
	s := &resumeServer{t: t}
	s.history = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"operationStatusAvailable": true, "operations": []map[string]any{
			{"operationId": "other", "clientOperationId": "cli-someone-else", "state": "completed"},
			{"operationId": "op9", "clientOperationId": clientOp, "state": "running"},
		}})
	}
	s.conns = []func(context.Context, *websocket.Conn){
		func(ctx context.Context, conn *websocket.Conn) {},
		func(ctx context.Context, conn *websocket.Conn) {
			write(ctx, conn,
				"event: streamMeta\ndata: {\"streamId\":\"op9\"}\n\n",
				"id: 1\nevent: answer\ndata: {\"choices\":[{\"delta\":{\"content\":\"Back\"},\"finish_reason\":\"stop\"}]}\n\n",
				"event: done\ndata: [DONE]\n\n")
		},
	}
	srv := s.start()
	c := api.New(srv.URL, "http://site", "t", func(context.Context) (string, error) { return "tok", nil })
	clientOp = "cli-fixed"
	res, err := Turn(context.Background(), c, TurnOptions{ConversationID: "conv1", Message: "hi", ClientOperationID: clientOp, AllowSpend: true})
	if err != nil || res.Answer != "Back" || res.Error != "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(s.queries) != 2 || !strings.Contains(s.queries[1], "resumeStreamId=op9") || !strings.Contains(s.queries[1], "lastEventId=0") {
		t.Fatalf("queries = %v", s.queries)
	}
}

// When the operation cannot be found or reattached, the turn still ends with
// the dropped-stream error rather than looping.
func TestTurnGivesUpWhenTheOperationCannotBeFound(t *testing.T) {
	fastResume(t)
	s := &resumeServer{t: t}
	s.history = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"operationStatusAvailable": true})
	}
	s.conns = []func(context.Context, *websocket.Conn){func(ctx context.Context, conn *websocket.Conn) {}}
	srv := s.start()
	c := api.New(srv.URL, "http://site", "t", func(context.Context) (string, error) { return "tok", nil })
	res, err := Turn(context.Background(), c, TurnOptions{ConversationID: "conv1", Message: "hi", AllowSpend: true})
	if err != nil || res.Error != "stream closed before done" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(s.queries) != 1 {
		t.Fatalf("queries = %v; want no resume without an operation", s.queries)
	}
}
