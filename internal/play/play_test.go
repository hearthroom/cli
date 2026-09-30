package play

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/hearthroom/cli/internal/api"
)

func TestParseFrame(t *testing.T) {
	ev, err := ParseFrame("id: 7\nevent: answer\ndata: {\"choices\":[{\"delta\":{\"content\":\"Hel\"},\"finish_reason\":null}]}\n\n")
	if err != nil || ev.ID != 7 || ev.Type != "answer" || ev.Text != "Hel" || ev.Final != nil {
		t.Fatalf("%+v %v", ev, err)
	}
	ev, _ = ParseFrame("event: answer\ndata: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"stop\"}]}\n\n")
	if ev.Final == nil || *ev.Final != "stop" {
		t.Fatalf("final: %+v", ev)
	}
	ev, _ = ParseFrame("event: done\ndata: [DONE]\n\n")
	if ev.Type != "done" || string(ev.Data) != `"[DONE]"` {
		t.Fatalf("done: %+v", ev)
	}
	ev, _ = ParseFrame(`{"error":"invalid request body"}`)
	if ev.Type != "error" || !strings.Contains(string(ev.Data), "invalid request body") {
		t.Fatalf("raw error: %+v", ev)
	}
	ev, _ = ParseFrame("event: thinking\ndata: {\"choices\":[{\"delta\":{\"reasoning_content\":\"hmm\"}}]}\n\n")
	if ev.Text != "hmm" {
		t.Fatalf("thinking: %+v", ev)
	}
	if _, err := ParseFrame("garbage"); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestTurnRefusesWithoutSpend(t *testing.T) {
	c := api.New("http://127.0.0.1:1", "http://site", "t", nil)
	if _, err := Turn(context.Background(), c, TurnOptions{ConversationID: "x", Message: "hi"}); err != ErrSpendNotAllowed {
		t.Fatalf("err = %v", err)
	}
}

// fakeStream serves ws-ticket and a WebSocket that replays a scripted turn.
func fakeStream(t *testing.T, script []string, wantModel string, wantLanguage ...string) *httptest.Server {
	lang := ""
	if len(wantLanguage) > 0 {
		lang = wantLanguage[0]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/open/v1/conversation/ws-ticket", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, `{"error":"unauthorized"}`, 401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ticket": "T1", "expiresIn": 30})
	})
	mux.HandleFunc("/open/v1/conversation/ws", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("protocolVersion") != "2" || r.Header.Get("Authorization") != "" {
			http.Error(w, "bad upgrade", 400)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var auth map[string]string
		_ = json.Unmarshal(raw, &auth)
		if auth["type"] != "auth" || auth["ticket"] != "T1" {
			_ = conn.Write(ctx, websocket.MessageText, []byte("event: error\ndata: {\"code\":\"unauthorized\"}\n\n"))
			return
		}
		_ = conn.Write(ctx, websocket.MessageText, []byte("event: ready\ndata: {\"accountBound\":true}\n\n"))
		_, raw, err = conn.Read(ctx)
		if err != nil {
			return
		}
		var frame map[string]any
		_ = json.Unmarshal(raw, &frame)
		if frame["conversationId"] != "conv1" || frame["message"] != "hi" || frame["operationKind"] != "send" || frame["clientOperationId"] == "" || (wantModel != "" && frame["model"] != wantModel) {
			t.Errorf("chat frame = %v", frame)
		}
		// The provider defaults a missing language to English, so a Traditional
		// Chinese card must carry its language on every turn; an empty one is omitted.
		if got, ok := frame["language"]; (lang != "" && got != lang) || (lang == "" && ok) {
			t.Errorf("chat frame language = %v (want %q)", got, lang)
		}
		for _, s := range script {
			if err := conn.Write(ctx, websocket.MessageText, []byte(s)); err != nil {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestTurnStreamsAnswer(t *testing.T) {
	script := []string{
		"id: 1\nevent: streamMeta\ndata: {\"streamId\":\"s1\"}\n\n",
		"id: 2\nevent: accepted\ndata: {\"chatId\":\"c1\"}\n\n",
		"id: 3\nevent: messageMeta\ndata: {\"isV3\":false}\n\n",
		"id: 4\nevent: answer\ndata: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n",
		"id: 5\nevent: answer\ndata: {\"choices\":[{\"delta\":{\"content\":\", world\"},\"finish_reason\":null}]}\n\n",
		"id: 6\nevent: answer\ndata: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"stop\"}]}\n\n",
		"id: 7\nevent: answer\ndata: [DONE]\n\n",
		"id: 8\nevent: operationStatus\ndata: {\"state\":\"completed\"}\n\n",
		"id: 9\nevent: done\ndata: [DONE]\n\n",
	}
	srv := fakeStream(t, script, "m1")
	c := api.New(srv.URL, "http://site", "t", func(context.Context) (string, error) { return "tok", nil })
	var seen []string
	res, err := Turn(context.Background(), c, TurnOptions{ConversationID: "conv1", Message: "hi", Model: "m1", AllowSpend: true, OnEvent: func(ev Event) { seen = append(seen, ev.Type) }})
	if err != nil {
		t.Fatal(err)
	}
	if res.Answer != "Hello, world" || res.FinishReason != "stop" || res.StreamID != "s1" || res.ChatID != "c1" || res.Error != "" {
		t.Fatalf("result: %+v", res)
	}
	if !strings.Contains(string(res.Status), "completed") || seen[0] != "ready" || seen[len(seen)-1] != "done" {
		t.Fatalf("status=%s seen=%v", res.Status, seen)
	}
}

func TestTurnReportsServerError(t *testing.T) {
	script := []string{
		"id: 1\nevent: streamMeta\ndata: {\"streamId\":\"s1\"}\n\n",
		"event: error\ndata: {\"event\":\"error\",\"error\":\"not enough credits\",\"error_type\":\"insufficient_credits\",\"retryable\":false}\n\n",
	}
	srv := fakeStream(t, script, "")
	c := api.New(srv.URL, "http://site", "t", func(context.Context) (string, error) { return "tok", nil })
	res, err := Turn(context.Background(), c, TurnOptions{ConversationID: "conv1", Message: "hi", AllowSpend: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Error != "not enough credits" || res.ErrorType != "insufficient_credits" {
		t.Fatalf("result: %+v", res)
	}
}

func TestTurnSendsTheCardLanguage(t *testing.T) {
	script := []string{
		"id: 1\nevent: answer\ndata: {\"choices\":[{\"delta\":{\"content\":\"你好\"},\"finish_reason\":\"stop\"}]}\n\n",
		"id: 2\nevent: done\ndata: [DONE]\n\n",
	}
	srv := fakeStream(t, script, "", "zh-Hant")
	c := api.New(srv.URL, "http://site", "t", func(context.Context) (string, error) { return "tok", nil })
	if _, err := Turn(context.Background(), c, TurnOptions{ConversationID: "conv1", Message: "hi", Language: "zh-Hant", AllowSpend: true}); err != nil {
		t.Fatal(err)
	}
}

// --new-session: archive the current conversation, then open the fresh one.
func TestStartNewArchivesThenOpens(t *testing.T) {
	var archived map[string]any
	starts := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/open/v1/conversation/save-and-start-new", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&archived)
		_ = json.NewEncoder(w).Encode(map[string]any{"conversationId": "conv2"})
	})
	mux.HandleFunc("/open/v1/conversation/start", func(w http.ResponseWriter, r *http.Request) {
		starts++
		id := "conv1"
		if archived != nil {
			id = "conv2"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"conversationId": id, "historyConversation": false, "defaultRelay": "hi", "roleInfo": map[string]any{"roleName": "Mira"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := api.New(srv.URL, "http://site", "t", func(context.Context) (string, error) { return "tok", nil })
	res, err := StartNew(context.Background(), c, "r1", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if archived["conversationId"] != "conv1" || archived["save"] != true || res.ConversationID != "conv2" || starts != 2 {
		t.Fatalf("archived=%v res=%+v starts=%d", archived, res, starts)
	}
}
