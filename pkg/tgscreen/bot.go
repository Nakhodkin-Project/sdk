package tgscreen

import (
	"errors"
	"fmt"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

// Bot wraps tgbotapi.BotAPI with screen-oriented helpers that keep a chat's
// navigation pinned to a single anchor message.
type Bot struct {
	*tgbotapi.BotAPI
	Sessions SessionStore
}

// New creates a Bot. If store is nil, an in-memory SessionStore is used.
func New(api *tgbotapi.BotAPI, store SessionStore) *Bot {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Bot{BotAPI: api, Sessions: store}
}

// Show renders s as the chat's anchor message: if an anchor already exists
// it is edited in place (text and keyboard in one request), otherwise a new
// message is sent and becomes the anchor.
func (b *Bot) Show(chatID int64, s Screen) error {
	session := b.Sessions.Get(chatID)
	anchor := session.Anchor()

	markup := s.Markup
	if markup.InlineKeyboard == nil {
		markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{}
	}

	if anchor.MessageID == 0 {
		msg := tgbotapi.NewMessage(chatID, s.Text)
		msg.ReplyMarkup = markup
		msg.ParseMode = s.ParseMode
		sent, err := b.Send(msg)
		if err != nil {
			return fmt.Errorf("tgscreen: send screen: %w", err)
		}
		session.SetAnchor(sent)
		return nil
	}

	edit := tgbotapi.NewEditMessageText(chatID, anchor.MessageID, s.Text)
	edit.ReplyMarkup = &markup
	edit.ParseMode = s.ParseMode
	edited, err := b.Send(edit)
	if err != nil {
		return fmt.Errorf("tgscreen: edit screen: %w", err)
	}
	session.SetAnchor(edited)
	return nil
}

// Track adds msg to the messages tracked as part of the chat's current
// screen, so a later ClearPage (or Reset) removes it.
func (b *Bot) Track(chatID int64, msg tgbotapi.Message) {
	b.Sessions.Get(chatID).AppendPage(msg)
}

// ClearPage deletes every message tracked via Track for chatID. It attempts
// to delete all of them even if some deletions fail, returning a joined
// error for any that did.
func (b *Bot) ClearPage(chatID int64) error {
	page := b.Sessions.Get(chatID).TakePage()

	var errs []error
	for _, msg := range page {
		if _, err := b.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID)); err != nil {
			errs = append(errs, fmt.Errorf("tgscreen: delete message %d: %w", msg.MessageID, err))
		}
	}
	return errors.Join(errs...)
}

// Undo deletes the page messages tracked since mark (a value previously
// obtained from Session.PageLen) — used to let the user redo a step without
// disturbing messages tracked by earlier, already-confirmed steps.
func (b *Bot) Undo(chatID int64, mark int) error {
	dropped := b.Sessions.Get(chatID).DropPageSince(mark)

	var errs []error
	for _, msg := range dropped {
		if _, err := b.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID)); err != nil {
			errs = append(errs, fmt.Errorf("tgscreen: delete message %d: %w", msg.MessageID, err))
		}
	}
	return errors.Join(errs...)
}

// Reset clears the chat's tracked page messages and shows s as a fresh
// anchor (sent as a new message rather than an edit).
func (b *Bot) Reset(chatID int64, s Screen) error {
	clearErr := b.ClearPage(chatID)

	session := b.Sessions.Get(chatID)
	anchor := session.Anchor()
	if anchor.MessageID != 0 {
		if _, err := b.Send(tgbotapi.NewDeleteMessage(chatID, anchor.MessageID)); err != nil {
			clearErr = errors.Join(clearErr, fmt.Errorf("tgscreen: delete anchor %d: %w", anchor.MessageID, err))
		}
		session.SetAnchor(tgbotapi.Message{})
	}

	if err := b.Show(chatID, s); err != nil {
		return errors.Join(clearErr, err)
	}
	return clearErr
}

// Flash sends text as a message that deletes itself after ttl. It is meant
// for transient notices (errors, confirmations) that shouldn't clutter the
// chat.
func (b *Bot) Flash(chatID int64, text string, ttl time.Duration) error {
	sent, err := b.Send(tgbotapi.NewMessage(chatID, text))
	if err != nil {
		return fmt.Errorf("tgscreen: flash: %w", err)
	}

	go func() {
		time.Sleep(ttl)
		// Best-effort cleanup: the message may already be gone (e.g. the
		// chat was cleared), so a failed delete here is not actionable.
		_, _ = b.Send(tgbotapi.NewDeleteMessage(chatID, sent.MessageID))
	}()
	return nil
}
