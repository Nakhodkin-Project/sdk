package tgscreen_test

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/matvievsky/tg-bot-sdk/pkg/tgscreen"
)

// newConvCtx builds a Context for chatID, after Show-ing an initial anchor
// so subsequent screens are edited in place.
func newConvCtx(t *testing.T, bot *tgscreen.Bot, chatID int64) *tgscreen.Context {
	t.Helper()
	if err := bot.Show(chatID, tgscreen.Screen{Text: "menu"}); err != nil {
		t.Fatalf("Show: %v", err)
	}
	return &tgscreen.Context{Bot: bot, ChatID: chatID, Session: bot.Sessions.Get(chatID)}
}

func TestConversationFullFlow(t *testing.T) {
	bot, _ := newTestBot()
	const chatID = 5

	var done bool
	conv := &tgscreen.Conversation{
		Steps: []tgscreen.Step{
			{
				Prompt:  tgscreen.Screen{Text: "name?"},
				SaveAs:  "name",
				Confirm: tgscreen.Screen{Text: "confirm name"},
			},
			{
				Prompt:  tgscreen.Screen{Text: "age?"},
				SaveAs:  "age",
				Confirm: tgscreen.Screen{Text: "confirm age"},
			},
		},
		OnDone: func(ctx *tgscreen.Context) error {
			done = true
			return nil
		},
	}

	router := tgscreen.NewRouter().Conversation("survey", conv)
	router.ConfirmYes("yes")
	router.ConfirmNo("no")

	ctx := newConvCtx(t, bot, chatID)
	if err := conv.Start(ctx, "survey"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := bot.Sessions.Get(chatID).Conv(); got == nil || got.Name != "survey" || got.Step != 0 {
		t.Fatalf("Conv after Start = %+v", got)
	}

	answerName := tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 100,
		Chat:      &tgbotapi.Chat{ID: chatID},
		Text:      "Alice",
	}}
	if err := router.Dispatch(bot, answerName); err != nil {
		t.Fatalf("Dispatch (name): %v", err)
	}
	if got := bot.Sessions.Get(chatID).Conv(); got == nil || !got.Blocked {
		t.Fatalf("expected conversation blocked awaiting confirmation, got %+v", got)
	}
	if name, ok := tgscreen.Get[tgbotapi.Message](bot.Sessions.Get(chatID), "name"); !ok || name.Text != "Alice" {
		t.Fatalf("name not stored correctly: %+v, ok=%v", name, ok)
	}

	reject := tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{
		ID: "cb1", Data: "no",
		Message: &tgbotapi.Message{MessageID: 1, Chat: &tgbotapi.Chat{ID: chatID}},
	}}
	if err := router.Dispatch(bot, reject); err != nil {
		t.Fatalf("Dispatch (reject): %v", err)
	}
	if got := bot.Sessions.Get(chatID).Conv(); got == nil || got.Step != 0 || got.Blocked {
		t.Fatalf("expected unblocked step 0 after reject, got %+v", got)
	}
	if got := bot.Sessions.Get(chatID).Page(); len(got) != 0 {
		t.Fatalf("expected the rejected step's tracked message to be undone, got %v", got)
	}

	if err := router.Dispatch(bot, answerName); err != nil {
		t.Fatalf("Dispatch (name again): %v", err)
	}
	accept := tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{
		ID: "cb2", Data: "yes",
		Message: &tgbotapi.Message{MessageID: 1, Chat: &tgbotapi.Chat{ID: chatID}},
	}}
	if err := router.Dispatch(bot, accept); err != nil {
		t.Fatalf("Dispatch (accept): %v", err)
	}
	if got := bot.Sessions.Get(chatID).Conv(); got == nil || got.Step != 1 || got.Blocked {
		t.Fatalf("expected unblocked step 1, got %+v", got)
	}

	answerAge := tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 101,
		Chat:      &tgbotapi.Chat{ID: chatID},
		Text:      "30",
	}}
	if err := router.Dispatch(bot, answerAge); err != nil {
		t.Fatalf("Dispatch (age): %v", err)
	}
	if err := router.Dispatch(bot, accept); err != nil {
		t.Fatalf("Dispatch (accept 2): %v", err)
	}

	if !done {
		t.Fatal("OnDone was not called")
	}
	if got := bot.Sessions.Get(chatID).Conv(); got != nil {
		t.Fatalf("expected Conv to be cleared after OnDone, got %+v", got)
	}
}

func TestConfirmNoUndoesOnlyCurrentStepMessages(t *testing.T) {
	bot, fake := newTestBot()
	const chatID = 7

	conv := &tgscreen.Conversation{
		Steps: []tgscreen.Step{
			{
				Prompt: tgscreen.Screen{Text: "first?"},
				// Tracks an extra message besides the user's reply (which is
				// deleted once captured), to exercise Undo across steps.
				OnInput: func(ctx *tgscreen.Context, msg *tgbotapi.Message) (tgscreen.Screen, bool, error) {
					extra, err := ctx.Send(tgbotapi.NewMessage(ctx.ChatID, "extra1"))
					if err != nil {
						return tgscreen.Screen{}, false, err
					}
					ctx.Track(ctx.ChatID, extra)
					return tgscreen.Screen{Text: "confirm first"}, true, nil
				},
			},
			{
				Prompt: tgscreen.Screen{Text: "second?"},
				// Tracks an extra message besides the user's reply, to
				// exercise Undo across more than one tracked message.
				OnInput: func(ctx *tgscreen.Context, msg *tgbotapi.Message) (tgscreen.Screen, bool, error) {
					extra, err := ctx.Send(tgbotapi.NewMessage(ctx.ChatID, "extra2"))
					if err != nil {
						return tgscreen.Screen{}, false, err
					}
					ctx.Track(ctx.ChatID, extra)
					return tgscreen.Screen{Text: "confirm second"}, true, nil
				},
			},
		},
	}

	router := tgscreen.NewRouter().Conversation("flow", conv)
	router.ConfirmYes("yes")
	router.ConfirmNo("no")

	ctx := newConvCtx(t, bot, chatID)
	if err := conv.Start(ctx, "flow"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	first := tgbotapi.Update{Message: &tgbotapi.Message{MessageID: 100, Chat: &tgbotapi.Chat{ID: chatID}, Text: "one"}}
	accept := tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{ID: "cb1", Data: "yes", Message: &tgbotapi.Message{MessageID: 1, Chat: &tgbotapi.Chat{ID: chatID}}}}
	reject := tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{ID: "cb2", Data: "no", Message: &tgbotapi.Message{MessageID: 1, Chat: &tgbotapi.Chat{ID: chatID}}}}

	if err := router.Dispatch(bot, first); err != nil {
		t.Fatalf("Dispatch (first): %v", err)
	}
	if err := router.Dispatch(bot, accept); err != nil {
		t.Fatalf("Dispatch (accept first): %v", err)
	}
	if got := bot.Sessions.Get(chatID).Page(); len(got) != 1 {
		t.Fatalf("Page() after step 1 = %v, want 1 tracked message", got)
	}

	second := tgbotapi.Update{Message: &tgbotapi.Message{MessageID: 101, Chat: &tgbotapi.Chat{ID: chatID}, Text: "two"}}
	if err := router.Dispatch(bot, second); err != nil {
		t.Fatalf("Dispatch (second): %v", err)
	}
	if got := bot.Sessions.Get(chatID).Page(); len(got) != 2 {
		t.Fatalf("Page() after step 2 input = %v, want 2 tracked messages (step 1's extra + this step's extra)", got)
	}

	deletesBefore := fake.callsTo("deleteMessage")
	if err := router.Dispatch(bot, reject); err != nil {
		t.Fatalf("Dispatch (reject second): %v", err)
	}
	if got := fake.callsTo("deleteMessage") - deletesBefore; got != 1 {
		t.Fatalf("deleteMessage calls after reject = %d, want 1", got)
	}
	if got := bot.Sessions.Get(chatID).Page(); len(got) != 1 {
		t.Fatalf("Page() after reject = %v, want step 1's message to remain", got)
	}
}

func TestStepOnInputCanKeepWaiting(t *testing.T) {
	bot, _ := newTestBot()
	const chatID = 6

	attempts := 0
	conv := &tgscreen.Conversation{
		Steps: []tgscreen.Step{
			{
				Prompt: tgscreen.Screen{Text: "a number, please"},
				OnInput: func(ctx *tgscreen.Context, msg *tgbotapi.Message) (tgscreen.Screen, bool, error) {
					attempts++
					if msg.Text != "42" {
						return tgscreen.Screen{}, false, nil
					}
					return tgscreen.Screen{Text: "confirm"}, true, nil
				},
			},
		},
	}

	ctx := newConvCtx(t, bot, chatID)
	if err := conv.Start(ctx, "numbers"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	bad := &tgbotapi.Message{MessageID: 1, Chat: &tgbotapi.Chat{ID: chatID}, Text: "oops"}
	if err := conv.Advance(ctx, bad); err != nil {
		t.Fatalf("Advance (bad): %v", err)
	}
	if got := bot.Sessions.Get(chatID).Conv(); got == nil || got.Blocked {
		t.Fatalf("expected to remain unblocked on step 0 after invalid input, got %+v", got)
	}

	good := &tgbotapi.Message{MessageID: 2, Chat: &tgbotapi.Chat{ID: chatID}, Text: "42"}
	if err := conv.Advance(ctx, good); err != nil {
		t.Fatalf("Advance (good): %v", err)
	}
	if got := bot.Sessions.Get(chatID).Conv(); got == nil || !got.Blocked {
		t.Fatalf("expected blocked after valid input, got %+v", got)
	}
	if attempts != 2 {
		t.Fatalf("OnInput called %d times, want 2", attempts)
	}
}
