package cli

import (
	"encoding/json"
	"fmt"
	"github.com/coder/websocket"
	"github.com/hearthroom/cli/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlayContinueUsesCheckpoint(t *testing.T) {
	for _, state := range []string{"failed_retryable", "stopped"} {
		t.Run(state, func(t *testing.T) {
			cfg := authTestEnv(t)
			t.Setenv(auth.EnvToken, "tok")
			var frame map[string]any
			mux := http.NewServeMux()
			mux.HandleFunc("/open/v1/conversation/start", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{"conversationId":"conv1","historyConversation":true}`)
			})
			mux.HandleFunc("/open/v1/conversation/messages", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"operationStatusAvailable":true,"operations":[{"operationId":"parent","conversationId":"conv1","clientOperationId":"old-client","sourceChatId":"user1","state":%q,"agentTurn":true,"allowedActions":["continue","switch_model"],"chargeCredits":117,"settled":true}],"chats":[{"chatId":"user1","chatRole":"USER","chatMessage":"original line"}]}`, state)
			})
			mux.HandleFunc("/open/v1/conversation/ws-ticket", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"ticket":"T1"}`) })
			mux.HandleFunc("/open/v1/conversation/ws", func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				_, _, _ = conn.Read(r.Context())
				_ = conn.Write(r.Context(), websocket.MessageText, []byte("event: ready\ndata: {}\n\n"))
				_, raw, err := conn.Read(r.Context())
				if err != nil {
					return
				}
				_ = json.Unmarshal(raw, &frame)
				for _, s := range []string{"event: answer\ndata: {\"choices\":[{\"delta\":{\"content\":\"Recovered\"},\"finish_reason\":\"stop\"}]}\n\n", "event: operationStatus\ndata: {\"operationId\":\"child\",\"state\":\"completed\",\"chargeCredits\":33,\"settled\":true,\"allowedActions\":[]}\n\n", "event: done\ndata: [DONE]\n\n"} {
					_ = conn.Write(r.Context(), websocket.MessageText, []byte(s))
				}
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()
			out, errOut, err := runCLI(t, "play", "--role", "r1", "--continue", "--model", "repaired", "--allow-spend", "--json", "--api", srv.URL, "--config-dir", cfg)
			if err != nil {
				t.Fatalf("continue failed: %v %s %s", err, out, errOut)
			}
			if frame["message"] != "original line" || frame["resumeFromOperationId"] != "parent" || frame["operationKind"] != "retry_generation" || frame["model"] != "repaired" || frame["clientOperationId"] == "old-client" || frame["clientOperationId"] == nil || frame["rewrite"] == true {
				t.Fatalf("bad continuation frame: %v", frame)
			}
			if !strings.Contains(out, `"answer": "Recovered"`) || !strings.Contains(out, `"chargeCredits":33`) {
				t.Fatalf("missing result/status: %s", out)
			}
		})
	}
}

func TestPlayContinueRefusesUnsafeRequests(t *testing.T) {
	cases := []struct {
		name, history string
		flags         []string
		wantReads     bool
	}{
		{"spend", `{"operationStatusAvailable":true,"operations":[{"operationId":"parent","conversationId":"conv1","sourceChatId":"u1","state":"failed_retryable","agentTurn":true,"allowedActions":["continue"]}],"chats":[{"chatId":"u1","chatRole":"USER","chatMessage":"hi"}]}`, nil, true},
		{"unavailable", `{"operations":[],"chats":[]}`, []string{"--allow-spend"}, true},
		{"latest_not_older", `{"operationStatusAvailable":true,"operations":[{"operationId":"old","conversationId":"conv1","state":"failed_retryable","agentTurn":true,"allowedActions":["continue"]},{"operationId":"latest","conversationId":"conv1","state":"completed","allowedActions":[]}],"chats":[]}`, []string{"--allow-spend"}, true},
		{"missing_user", `{"operationStatusAvailable":true,"operations":[{"operationId":"parent","conversationId":"conv1","sourceChatId":"missing","state":"failed_retryable","agentTurn":true,"allowedActions":["continue"]}],"chats":[]}`, []string{"--allow-spend"}, true},
	}
	for _, flags := range [][]string{{"-m", "new line"}, {"--new-session"}, {"--history"}, {"--stop"}, {"--agent", "on"}, {"--greeting", "1"}} {
		cases = append(cases, struct {
			name, history string
			flags         []string
			wantReads     bool
		}{"conflict_" + flags[0], "", flags, false})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := authTestEnv(t)
			t.Setenv(auth.EnvToken, "tok")
			reads, paid := 0, 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/open/v1/conversation/start":
					reads++
					fmt.Fprint(w, `{"conversationId":"conv1","historyConversation":true}`)
				case "/open/v1/conversation/messages":
					reads++
					fmt.Fprint(w, tc.history)
				default:
					paid++
					http.Error(w, "unexpected request", 500)
				}
			}))
			defer srv.Close()
			args := append([]string{"play", "--role", "r1", "--continue", "--api", srv.URL, "--config-dir", cfg}, tc.flags...)
			_, _, err := runCLI(t, args...)
			if err == nil {
				t.Fatal("unsafe continue accepted")
			}
			if paid != 0 {
				t.Fatalf("paid requests=%d", paid)
			}
			if tc.wantReads && reads == 0 {
				t.Fatalf("did not inspect operation metadata: %v", err)
			}
			if !tc.wantReads && reads != 0 {
				t.Fatalf("conflict performed %d requests", reads)
			}
		})
	}
}

func TestPlayHistoryShowsContinuationAndCharge(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open/v1/conversation/start":
			fmt.Fprint(w, `{"conversationId":"conv1","historyConversation":true}`)
		case "/open/v1/conversation/messages":
			fmt.Fprint(w, `{"operationStatusAvailable":true,"operations":[{"operationId":"parent","conversationId":"conv1","sourceChatId":"u1","state":"failed_retryable","agentTurn":true,"allowedActions":["continue","backward","switch_model"],"chargeCredits":117,"settled":false}],"chats":[]}`)
		default:
			t.Errorf("unexpected paid request: %s", r.URL.Path)
			http.Error(w, "unexpected", 500)
		}
	}))
	defer srv.Close()
	args := []string{"play", "--role", "r1", "--history", "--api", srv.URL, "--config-dir", cfg}
	_, notes, err := runCLI(t, args...)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"failed_retryable", "117", "continue", "backward", "switch_model", "--continue --allow-spend", "--model"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("missing %q: %s", want, notes)
		}
	}
	out, notes, err := runCLI(t, append(args, "--json")...)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		History struct {
			Operations []struct {
				Settled       bool
				ChargeCredits int64 `json:"chargeCredits"`
			}
		}
	}
	if json.Unmarshal([]byte(out), &result) != nil || len(result.History.Operations) != 1 || result.History.Operations[0].ChargeCredits != 117 || result.History.Operations[0].Settled || notes != "" {
		t.Fatalf("bad machine status: %s notes=%s", out, notes)
	}
}

func TestPlayContinuationHintUsesResolvedRole(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open/v1/conversation/start":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["roleId"] != "selected-role" {
				t.Errorf("wrong role: %v", body)
			}
			fmt.Fprint(w, `{"conversationId":"conv1","historyConversation":true}`)
		case "/open/v1/conversation/messages":
			fmt.Fprint(w, `{"operationStatusAvailable":true,"operations":[{"operationId":"parent","conversationId":"conv1","sourceChatId":"u1","state":"failed_retryable","agentTurn":true,"allowedActions":["continue"],"settled":true}],"chats":[]}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	_, notes, err := runCLI(t, "play", "ignored-folder", "--role", "selected-role", "--history", "--api", srv.URL, "--config-dir", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(notes, "--role 'selected-role' --continue --allow-spend") || strings.Contains(notes, "ignored-folder") {
		t.Fatalf("hint changes the actual target: %s", notes)
	}
}
