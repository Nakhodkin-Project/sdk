package tgscreen

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
//
// Show is self-healing: if the anchor can no longer be edited — most often
// because the user deleted it, but also when it is too old or its id is no
// longer valid — Show drops the dead anchor and sends s as a fresh one, so
// navigation never silently dead-ends with nothing on screen. The only edit
// failure it treats as success is "message is not modified", which means the
// anchor already shows s.
func (b *Bot) Show(chatID int64, s Screen) error {
	session := b.Sessions.Get(chatID)
	anchor := session.Anchor()

	markup := s.Markup
	if markup.InlineKeyboard == nil {
		markup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{}
	}

	if anchor.MessageID != 0 {
		edit := tgbotapi.NewEditMessageText(chatID, anchor.MessageID, s.Text)
		edit.ReplyMarkup = &markup
		edit.ParseMode = s.ParseMode
		edited, err := b.Send(edit)
		if err == nil {
			session.SetAnchor(edited)
			return nil
		}
		if isNotModifiedErr(err) {
			// The anchor already shows s exactly as requested; Telegram rejects
			// the edit but the screen is already correct.
			return nil
		}
		// The anchor can't be edited — almost always because the user deleted
		// it, but this also covers "message too old", an invalid id, or any
		// other Telegram wording we don't want to depend on. Drop it (deleting
		// best-effort in case it somehow still exists, to avoid a duplicate)
		// and fall through to sending a fresh anchor, so navigation never
		// silently dead-ends after the anchor disappears.
		_, _ = b.Request(tgbotapi.NewDeleteMessage(chatID, anchor.MessageID))
		session.SetAnchor(tgbotapi.Message{})
	}

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

// isNotModifiedErr reports whether err is Telegram's "message is not
// modified" error, returned when an edit's text and markup are identical to
// the message's current content.
func isNotModifiedErr(err error) bool {
	var apiErr *tgbotapi.Error
	return errors.As(err, &apiErr) && strings.Contains(apiErr.Message, "message is not modified")
}

// Track adds msg to the messages tracked as part of the chat's current
// screen, so a later ClearPage (or Reset) removes it.
func (b *Bot) Track(chatID int64, msg tgbotapi.Message) {
	b.Sessions.Get(chatID).AppendPage(msg)
}

// Untrack removes messageID from the messages tracked as part of the chat's
// current screen, e.g. after deleting it directly, so a later ClearPage (or
// Reset) doesn't try to delete it again.
func (b *Bot) Untrack(chatID int64, messageID int) {
	b.Sessions.Get(chatID).RemoveFromPage(messageID)
}

// Delete removes a single message from a chat. It is a thin convenience over
// the raw API for the single-anchor pattern of consuming a user's input
// message so only the anchor window remains.
func (b *Bot) Delete(chatID int64, messageID int) error {
	if _, err := b.Request(tgbotapi.NewDeleteMessage(chatID, messageID)); err != nil {
		return fmt.Errorf("tgscreen: delete message %d: %w", messageID, err)
	}
	return nil
}

// ClearPage deletes every message tracked via Track for chatID. It attempts
// to delete all of them even if some deletions fail, returning a joined
// error for any that did.
func (b *Bot) ClearPage(chatID int64) error {
	page := b.Sessions.Get(chatID).TakePage()
	return b.deleteMessages(chatID, page)
}

// Undo deletes the page messages tracked since mark (a value previously
// obtained from Session.PageLen) — used to let the user redo a step without
// disturbing messages tracked by earlier, already-confirmed steps.
func (b *Bot) Undo(chatID int64, mark int) error {
	dropped := b.Sessions.Get(chatID).DropPageSince(mark)
	return b.deleteMessages(chatID, dropped)
}

// deleteMessages deletes msgs concurrently, since each call is a separate
// round trip to the Telegram API and a chat screen can track many messages.
// It attempts to delete all of them even if some deletions fail, returning a
// joined error for any that did.
func (b *Bot) deleteMessages(chatID int64, msgs []tgbotapi.Message) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, msg := range msgs {
		wg.Add(1)
		go func(messageID int) {
			defer wg.Done()
			if _, err := b.Request(tgbotapi.NewDeleteMessage(chatID, messageID)); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("tgscreen: delete message %d: %w", messageID, err))
				mu.Unlock()
			}
		}(msg.MessageID)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// Resend deletes the chat's current anchor message, if any, without
// touching tracked page messages, and sends s as a new message that becomes
// the new anchor. Use this instead of Show when other messages have been
// sent since the anchor was last shown, so the navigation screen stays
// pinned below them at the bottom of the chat. Deleting the old anchor is
// best-effort: if it is already gone (e.g. the user deleted it), Resend still
// sends the fresh anchor rather than failing.
func (b *Bot) Resend(chatID int64, s Screen) error {
	session := b.Sessions.Get(chatID)
	if anchor := session.Anchor(); anchor.MessageID != 0 {
		// Best-effort: the anchor may already be gone (e.g. the user deleted
		// it). A failed delete must not stop us from sending the fresh anchor,
		// or navigation would dead-end with nothing on screen.
		_, _ = b.Request(tgbotapi.NewDeleteMessage(chatID, anchor.MessageID))
		session.SetAnchor(tgbotapi.Message{})
	}
	return b.Show(chatID, s)
}

// Reset clears the chat's tracked page messages and shows s as a fresh
// anchor (sent as a new message rather than an edit). Deleting the old anchor
// is best-effort, so an already-gone anchor does not block the fresh screen.
func (b *Bot) Reset(chatID int64, s Screen) error {
	clearErr := b.ClearPage(chatID)

	session := b.Sessions.Get(chatID)
	if anchor := session.Anchor(); anchor.MessageID != 0 {
		// Best-effort: a missing anchor (already deleted by the user) must not
		// be treated as an error or block the fresh Show below.
		_, _ = b.Request(tgbotapi.NewDeleteMessage(chatID, anchor.MessageID))
		session.SetAnchor(tgbotapi.Message{})
	}

	if err := b.Show(chatID, s); err != nil {
		return errors.Join(clearErr, err)
	}
	return clearErr
}

// Promote converts the current anchor into a pinned content slot (e.g. an
// advertisement) and sends main as a fresh anchor below it. This preserves the
// visual relationship "pinned content above, live navigation below" even after
// the user scrolls or the anchor is deleted.
//
// The two-step flow:
//   1. Edit the current anchor to show above. If the anchor is dead (deleted by
//      the user) or missing, above is sent as a new message instead.
//   2. Clear the stored anchor, then call Show(main) — which now sends a fresh
//      message because there is no stored anchor — so the live anchor always
//      sits at the bottom of the chat, just below the promoted slot.
//
// Typical usage: call Promote in the /start handler to set up the initial
// layout (above = ad or banner, main = the bot's home screen). For all
// subsequent navigations triggered by inline buttons, use Show — which edits
// the anchor in place without disturbing the promoted slot above it.
func (b *Bot) Promote(chatID int64, above, main Screen) error {
	aboveMarkup := above.Markup
	if aboveMarkup.InlineKeyboard == nil {
		aboveMarkup.InlineKeyboard = [][]tgbotapi.InlineKeyboardButton{}
	}

	session := b.Sessions.Get(chatID)
	anchor := session.Anchor()

	var promotedMsg tgbotapi.Message
	if anchor.MessageID != 0 {
		// Repurpose the existing anchor as the promoted slot.
		edit := tgbotapi.NewEditMessageText(chatID, anchor.MessageID, above.Text)
		edit.ReplyMarkup = &aboveMarkup
		edit.ParseMode = above.ParseMode
		edited, err := b.Send(edit)
		if err == nil {
			promotedMsg = edited
		} else if isNotModifiedErr(err) {
			// Already showing above — anchor is the promoted slot unchanged.
			promotedMsg = anchor
		} else {
			// Anchor is dead — fall back to a fresh message for the promoted slot.
			msg := tgbotapi.NewMessage(chatID, above.Text)
			msg.ReplyMarkup = aboveMarkup
			msg.ParseMode = above.ParseMode
			sent, err := b.Send(msg)
			if err != nil {
				return fmt.Errorf("tgscreen: promote: %w", err)
			}
			promotedMsg = sent
		}
		// Drop the stored anchor so Show below sends main as a fresh message.
		session.SetAnchor(tgbotapi.Message{})
	} else {
		// No existing anchor — send the promoted slot as a fresh message.
		msg := tgbotapi.NewMessage(chatID, above.Text)
		msg.ReplyMarkup = aboveMarkup
		msg.ParseMode = above.ParseMode
		sent, err := b.Send(msg)
		if err != nil {
			return fmt.Errorf("tgscreen: promote: %w", err)
		}
		promotedMsg = sent
	}

	// Track the promoted slot so /start can clean it up on the next restart.
	session.SetPromoted(promotedMsg)

	// Send main as a new anchor below the promoted slot.
	return b.Show(chatID, main)
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
		_, _ = b.Request(tgbotapi.NewDeleteMessage(chatID, sent.MessageID))
	}()
	return nil
}
