package tgscreen_test

import (
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/Nakhodkin-Project/sdk/pkg/tgscreen"
)

func TestShowSendsThenEditsInPlace(t *testing.T) {
	bot, fake := newTestBot()
	const chatID = 1

	if err := bot.Show(chatID, tgscreen.Screen{Text: "menu"}); err != nil {
		t.Fatalf("Show: %v", err)
	}
	if got := fake.callsTo("sendMessage"); got != 1 {
		t.Fatalf("sendMessage calls = %d, want 1", got)
	}

	anchor := bot.Sessions.Get(chatID).Anchor()
	if anchor.MessageID == 0 {
		t.Fatal("anchor not set after first Show")
	}

	if err := bot.Show(chatID, tgscreen.Screen{Text: "menu v2"}); err != nil {
		t.Fatalf("Show (edit): %v", err)
	}
	if got := fake.callsTo("editMessageText"); got != 1 {
		t.Fatalf("editMessageText calls = %d, want 1", got)
	}
	if got := fake.callsTo("sendMessage"); got != 1 {
		t.Fatalf("sendMessage calls after edit = %d, want still 1", got)
	}

	edited := bot.Sessions.Get(chatID).Anchor()
	if edited.MessageID != anchor.MessageID {
		t.Fatalf("anchor message id changed: %d -> %d", anchor.MessageID, edited.MessageID)
	}
}

func TestTrackAndClearPage(t *testing.T) {
	bot, fake := newTestBot()
	const chatID = 2

	for i := 0; i < 3; i++ {
		msg, err := bot.Send(tgbotapi.NewMessage(chatID, "extra"))
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		bot.Track(chatID, msg)
	}

	if got := bot.Sessions.Get(chatID).Page(); len(got) != 3 {
		t.Fatalf("Page() = %v, want 3 tracked messages", got)
	}

	if err := bot.ClearPage(chatID); err != nil {
		t.Fatalf("ClearPage: %v", err)
	}
	if got := fake.callsTo("deleteMessage"); got != 3 {
		t.Fatalf("deleteMessage calls = %d, want 3", got)
	}
	if got := bot.Sessions.Get(chatID).Page(); len(got) != 0 {
		t.Fatalf("Page() after ClearPage = %v, want empty", got)
	}
}

func TestReset(t *testing.T) {
	bot, fake := newTestBot()
	const chatID = 3

	if err := bot.Show(chatID, tgscreen.Screen{Text: "menu"}); err != nil {
		t.Fatalf("Show: %v", err)
	}
	extra, err := bot.Send(tgbotapi.NewMessage(chatID, "extra"))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	bot.Track(chatID, extra)

	if err := bot.Reset(chatID, tgscreen.Screen{Text: "menu again"}); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// The old anchor and the tracked extra message should both be deleted.
	if got := fake.callsTo("deleteMessage"); got != 2 {
		t.Fatalf("deleteMessage calls = %d, want 2", got)
	}
	// The initial Show, the tracked "extra" message, and Reset's fresh
	// anchor are all new messages, not edits.
	if got := fake.callsTo("sendMessage"); got != 3 {
		t.Fatalf("sendMessage calls = %d, want 3", got)
	}
	if got := fake.callsTo("editMessageText"); got != 0 {
		t.Fatalf("editMessageText calls = %d, want 0", got)
	}
	if got := bot.Sessions.Get(chatID).Page(); len(got) != 0 {
		t.Fatalf("Page() after Reset = %v, want empty", got)
	}
	if bot.Sessions.Get(chatID).Anchor().MessageID == 0 {
		t.Fatal("Reset should leave a fresh anchor")
	}
}

func TestFlashDeletesAfterTTL(t *testing.T) {
	bot, fake := newTestBot()
	const chatID = 4

	if err := bot.Flash(chatID, "oops", 10*time.Millisecond); err != nil {
		t.Fatalf("Flash: %v", err)
	}
	if got := fake.callsTo("sendMessage"); got != 1 {
		t.Fatalf("sendMessage calls = %d, want 1", got)
	}

	deadline := time.Now().Add(time.Second)
	for fake.callsTo("deleteMessage") == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := fake.callsTo("deleteMessage"); got != 1 {
		t.Fatalf("deleteMessage calls = %d, want 1", got)
	}
}
