package tgscreen_test

import (
	"testing"

	"github.com/Nakhodkin-Project/sdk/pkg/tgscreen"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestRouterCommand(t *testing.T) {
	bot, _ := newTestBot()

	var got bool
	router := tgscreen.NewRouter().Command("/start", func(ctx *tgscreen.Context) error {
		got = true
		return nil
	})

	update := tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 1,
		Chat:      &tgbotapi.Chat{ID: 1},
		Text:      "/start",
	}}
	if err := router.Dispatch(bot, update); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !got {
		t.Fatal("command handler was not called")
	}
}

func TestRouterCallbackAnswersAndDispatches(t *testing.T) {
	bot, fake := newTestBot()

	var got bool
	router := tgscreen.NewRouter().Callback("ping", func(ctx *tgscreen.Context) error {
		got = true
		if ctx.ChatID != 1 {
			t.Fatalf("ctx.ChatID = %d, want 1", ctx.ChatID)
		}
		return nil
	})

	update := tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{
		ID:   "cb1",
		Data: "ping",
		Message: &tgbotapi.Message{
			MessageID: 1,
			Chat:      &tgbotapi.Chat{ID: 1},
		},
	}}
	if err := router.Dispatch(bot, update); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !got {
		t.Fatal("callback handler was not called")
	}
	if n := fake.callsTo("answerCallbackQuery"); n != 1 {
		t.Fatalf("answerCallbackQuery calls = %d, want 1", n)
	}
}

func TestRouterFallback(t *testing.T) {
	bot, _ := newTestBot()

	var got bool
	router := tgscreen.NewRouter().Fallback(func(ctx *tgscreen.Context) error {
		got = true
		return nil
	})

	update := tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 1,
		Chat:      &tgbotapi.Chat{ID: 1},
		Text:      "/unknown",
	}}
	if err := router.Dispatch(bot, update); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !got {
		t.Fatal("fallback handler was not called")
	}
}

func TestSingleWindowDeletesUserMessage(t *testing.T) {
	bot, fake := newTestBot()

	router := tgscreen.NewRouter().SingleWindow().
		Fallback(func(ctx *tgscreen.Context) error { return nil })

	update := tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 7,
		Chat:      &tgbotapi.Chat{ID: 1},
		Text:      "/start",
	}}
	if err := router.Dispatch(bot, update); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if n := fake.callsTo("deleteMessage"); n != 1 {
		t.Fatalf("deleteMessage calls = %d, want 1", n)
	}
}

func TestSingleWindowKeepsUserMessageWhenDisabled(t *testing.T) {
	bot, fake := newTestBot()

	router := tgscreen.NewRouter().
		Fallback(func(ctx *tgscreen.Context) error { return nil })

	update := tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 7,
		Chat:      &tgbotapi.Chat{ID: 1},
		Text:      "/start",
	}}
	if err := router.Dispatch(bot, update); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if n := fake.callsTo("deleteMessage"); n != 0 {
		t.Fatalf("deleteMessage calls = %d, want 0 without SingleWindow", n)
	}
}

func TestSingleWindowAdoptsCallbackAnchor(t *testing.T) {
	bot, _ := newTestBot()

	router := tgscreen.NewRouter().SingleWindow().
		Callback("ping", func(ctx *tgscreen.Context) error { return nil })

	update := tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{
		ID:   "cb1",
		Data: "ping",
		Message: &tgbotapi.Message{
			MessageID: 42,
			Chat:      &tgbotapi.Chat{ID: 1},
		},
	}}
	if err := router.Dispatch(bot, update); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got := bot.Sessions.Get(1).Anchor().MessageID; got != 42 {
		t.Fatalf("adopted anchor MessageID = %d, want 42", got)
	}
}

func TestSingleWindowDoesNotDeleteCallbackMessage(t *testing.T) {
	bot, fake := newTestBot()

	router := tgscreen.NewRouter().SingleWindow().
		Callback("ping", func(ctx *tgscreen.Context) error { return nil })

	update := tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{
		ID:   "cb1",
		Data: "ping",
		Message: &tgbotapi.Message{
			MessageID: 42,
			Chat:      &tgbotapi.Chat{ID: 1},
		},
	}}
	if err := router.Dispatch(bot, update); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if n := fake.callsTo("deleteMessage"); n != 0 {
		t.Fatalf("deleteMessage calls = %d, want 0 (anchor must survive)", n)
	}
}

func TestRouterFallbackOnUnknownCallback(t *testing.T) {
	bot, _ := newTestBot()

	var got bool
	router := tgscreen.NewRouter().
		Callback("known", func(ctx *tgscreen.Context) error { return nil }).
		Fallback(func(ctx *tgscreen.Context) error {
			got = true
			return nil
		})

	update := tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{
		ID:   "cb1",
		Data: "unknown",
		Message: &tgbotapi.Message{
			MessageID: 1,
			Chat:      &tgbotapi.Chat{ID: 1},
		},
	}}
	if err := router.Dispatch(bot, update); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !got {
		t.Fatal("fallback handler was not called for unmatched callback data")
	}
}
