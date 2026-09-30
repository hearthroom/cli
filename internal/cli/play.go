package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/output"
	"github.com/hearthroom/cli/internal/play"
)

func init() {
	extraCommands = append(extraCommands, func(a *App) []*cobra.Command { return []*cobra.Command{a.playCommand()} })
}

func (a *App) playCommand() *cobra.Command {
	var (
		roleID, message, model, thinking, language  string
		greeting, historyN                          int
		allowSpend, history, stop, show, newSession bool
		agent                                       string
	)
	c := &cobra.Command{
		Use:   "play [dir]",
		Short: "Talk to a card from the terminal (experimental)",
		Long: `Opens or resumes the current conversation with the card in a folder (its
pushed trial or owned card) or with --role <id>, and sends one message.

Starting a conversation and reading history are free. Sending a message
generates a reply and spends credits on the provider, so it runs only with
--allow-spend. Press Ctrl-C to stop a reply; what was generated is still
charged and shown.

Turns carry the card's language from card.json unless --language says otherwise;
without one the provider replies in English. --new-session archives the current
conversation with the card and starts a fresh one, so two folders can test the
same card without sharing a thread.

With --json, every server event is printed as one JSON object per line.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.RequireAuth(); err != nil {
				return err
			}
			var f *card.Folder
			if roleID == "" {
				var err error
				f, err = loadFolder(args)
				if err != nil {
					return err
				}
				if f.State.RoleID == "" {
					return output.Exitf(2, "this folder has not been pushed yet; run `hearthroom card push` first or pass --role")
				}
				roleID = f.State.RoleID
			}
			ctx := cmd.Context()
			pageSize := 0
			if history {
				pageSize = historyN
			}
			var start *play.StartResponse
			var err error
			if newSession {
				start, err = play.StartNew(ctx, a.Client(), roleID, greeting, pageSize)
			} else {
				start, err = play.Start(ctx, a.Client(), roleID, greeting, pageSize)
			}
			if err != nil {
				return err
			}
			if f != nil && f.State.ConversationID != start.ConversationID {
				f.State.ConversationID = start.ConversationID
				_ = f.SaveState()
			}
			if stop {
				if err := play.Stop(ctx, a.Client(), start.ConversationID, ""); err != nil {
					return err
				}
				if a.Out.JSON {
					return a.Out.JSONValue(map[string]any{"conversationId": start.ConversationID, "stopped": true})
				}
				a.Out.Line("Stop requested for conversation %s", start.ConversationID)
				return nil
			}
			if history {
				page := start.FirstPage
				if page == nil {
					page, err = play.History(ctx, a.Client(), start.ConversationID, 1, historyN)
					if err != nil {
						return err
					}
				}
				if a.Out.JSON {
					return a.Out.JSONValue(map[string]any{"conversationId": start.ConversationID, "history": page})
				}
				a.Out.Line("Conversation %s with %s (%d messages)", start.ConversationID, start.RoleInfo.RoleName, page.Total)
				for i := len(page.Chats) - 1; i >= 0; i-- {
					m := page.Chats[i]
					tag := m.Role
					if m.IsSummary {
						tag = "summary"
					}
					a.Out.Line("")
					a.Out.Line("[%s]%s", tag, incompleteMark(m))
					a.Out.Line("%s", m.Text)
				}
				return nil
			}
			if message == "" {
				if a.Out.JSON {
					return a.Out.JSONValue(map[string]any{"conversationId": start.ConversationID, "new": !start.HistoryConversation, "opening": start.DefaultRelay, "role": start.RoleInfo.RoleName})
				}
				if start.HistoryConversation {
					a.Out.Line("Resumed conversation %s with %s.", start.ConversationID, start.RoleInfo.RoleName)
				} else {
					a.Out.Line("Started conversation %s with %s.", start.ConversationID, start.RoleInfo.RoleName)
					a.Out.Line("")
					a.Out.Line("%s", start.DefaultRelay)
				}
				a.Out.Line("")
				a.Out.Line("Send a message with: hearthroom play %s -m \"…\" --allow-spend", strings.Join(args, " "))
				return nil
			}
			if !allowSpend {
				return output.Exit(3, play.ErrSpendNotAllowed)
			}
			opts := play.TurnOptions{
				ConversationID: start.ConversationID, Message: message, Model: model, AllowSpend: true,
				ThinkingDepth: thinking, Language: turnLanguage(language, f),
			}
			switch agent {
			case "on":
				t := true
				opts.AgentMode = &t
			case "off":
				fl := false
				opts.AgentMode = &fl
			}
			if a.Out.JSON {
				enc := json.NewEncoder(a.Out.Out)
				enc.SetEscapeHTML(false)
				opts.OnEvent = func(ev play.Event) { _ = enc.Encode(ev) }
			} else {
				opts.OnEvent = func(ev play.Event) {
					switch ev.Type {
					case "answer":
						fmt.Fprint(a.Out.Out, ev.Text)
					case "thinking":
						if show {
							fmt.Fprint(a.Out.Err, ev.Text)
						}
					case "prepStep":
						var p struct {
							Stage    string `json:"stage"`
							Resource string `json:"resource"`
						}
						_ = json.Unmarshal(ev.Data, &p)
						a.Out.Note("  … %s %s", p.Stage, p.Resource)
					case "error":
						a.Out.Note("error: %s", string(ev.Data))
					}
				}
			}
			res, err := play.Turn(ctx, a.Client(), opts)
			if !a.Out.JSON {
				fmt.Fprintln(a.Out.Out)
			}
			if err != nil && !errors.Is(err, ctx.Err()) {
				return err
			}
			if a.Out.JSON {
				_ = a.Out.JSONValue(map[string]any{"result": res})
			} else {
				if res.FinishReason != "" {
					a.Out.Note("[finished: %s]", res.FinishReason)
				}
				if res.Error != "" {
					a.Out.Note("[%s]", res.Error)
				}
			}
			if res.Error != "" && res.Error != "interrupted" {
				return output.Exitf(1, "the turn did not complete: %s", res.Error)
			}
			return nil
		},
	}
	c.Flags().StringVar(&roleID, "role", "", "card id instead of a folder")
	c.Flags().StringVar(&language, "language", "", "reply language, e.g. zh-Hant (default: the folder's card.json language; the provider assumes en when none is sent)")
	c.Flags().BoolVar(&newSession, "new-session", false, "archive the current conversation with this card and start a fresh one")
	c.Flags().StringVarP(&message, "message", "m", "", "message to send (spends credits; requires --allow-spend)")
	c.Flags().BoolVar(&allowSpend, "allow-spend", false, "confirm that this turn may spend credits")
	c.Flags().StringVar(&model, "model", "", "model value from `hearthroom models` (default: provider default)")
	c.Flags().StringVar(&thinking, "thinking", "", "thinking depth override for models that support it")
	c.Flags().StringVar(&agent, "agent", "", "agent mode for this turn: on or off (default: saved preference)")
	c.Flags().IntVar(&greeting, "greeting", 0, "opening to use when a new conversation is created (0 = main, 1.. = alternates)")
	c.Flags().BoolVar(&history, "history", false, "print recent messages instead of sending")
	c.Flags().IntVar(&historyN, "limit", 20, "messages to show with --history")
	c.Flags().BoolVar(&stop, "stop", false, "stop the reply currently being generated")
	c.Flags().BoolVar(&show, "show-thinking", false, "print reasoning deltas to stderr")
	_ = os.Stdout
	return c
}

func incompleteMark(m play.Message) string {
	if !m.IsComplete && m.Role != "user" && !m.IsFirst {
		return " (incomplete)"
	}
	return ""
}

// turnLanguage is the language sent with a turn: the flag, else the card's
// language from card.json. Empty means the provider's default, which is English.
func turnLanguage(flag string, f *card.Folder) string {
	if flag != "" {
		return flag
	}
	if f != nil {
		return f.Manifest.Language
	}
	return ""
}
