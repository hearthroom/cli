package e2e

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/play"
	"github.com/hearthroom/cli/internal/sync"
)

// TestPlayStartHistoryAndSpendGate needs a provider with the conversation
// surface enabled. Generation is exercised only when HEARTHROOM_E2E_SPEND=1,
// because it charges the test account (a local provider with a fake relay
// is the intended setup).
func TestPlayStartHistoryAndSpendGate(t *testing.T) {
	e := setup(t)
	e.login(t)
	ctx := context.Background()
	f := fixtureFolder(t)
	pushed, err := sync.Push(ctx, e.client, f, sync.PushOptions{Evict: true})
	if err != nil {
		t.Fatal(err)
	}

	start, err := play.Start(ctx, e.client, pushed.RoleID, 0, 0)
	if err != nil {
		if api.IsStatus(err, 404) {
			t.Skip("provider has no conversation surface enabled")
		}
		t.Fatal(err)
	}
	if start.ConversationID == "" || start.HistoryConversation || !strings.Contains(start.DefaultRelay, "Hello from the end-to-end test") {
		t.Fatalf("start: %+v", start)
	}
	again, err := play.Start(ctx, e.client, pushed.RoleID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if again.ConversationID != start.ConversationID || !again.HistoryConversation || again.FirstPage == nil || len(again.FirstPage.Chats) != 1 {
		t.Fatalf("resume: %+v", again)
	}
	hist, err := play.History(ctx, e.client, start.ConversationID, 1, 10)
	if err != nil || len(hist.Chats) != 1 || !hist.Chats[0].IsFirst {
		t.Fatalf("history: %+v %v", hist, err)
	}

	// The spend gate never touches the network.
	if _, err := play.Turn(ctx, e.client, play.TurnOptions{ConversationID: start.ConversationID, Message: "hi"}); !errors.Is(err, play.ErrSpendNotAllowed) {
		t.Fatalf("spend gate: %v", err)
	}

	if os.Getenv("HEARTHROOM_E2E_SPEND") != "1" {
		t.Log("HEARTHROOM_E2E_SPEND != 1: skipping the paid turn")
		return
	}
	turnCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var events []string
	res, err := play.Turn(turnCtx, e.client, play.TurnOptions{
		ConversationID: start.ConversationID, Message: "hello", AllowSpend: true,
		OnEvent: func(ev play.Event) { events = append(events, ev.Type) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Error != "" || res.Answer == "" || res.StreamID == "" || res.ChatID == "" {
		t.Fatalf("turn: %+v events=%v", res, events)
	}
	if events[0] != "ready" || events[len(events)-1] != "done" {
		t.Fatalf("event order: %v", events)
	}
	hist, err = play.History(ctx, e.client, start.ConversationID, 1, 10)
	if err != nil || len(hist.Chats) != 3 || hist.Chats[0].Text != res.Answer {
		t.Fatalf("history after turn: %+v %v", hist, err)
	}
}
