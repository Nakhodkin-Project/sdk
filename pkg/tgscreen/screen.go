package tgscreen

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"

// Screen describes a single UI view: the text and inline keyboard shown in
// a chat's anchor message. Navigating between screens edits the anchor in
// place instead of sending a new message, so the bot's controls always stay
// in the same spot on screen.
type Screen struct {
	Text      string
	Markup    tgbotapi.InlineKeyboardMarkup
	ParseMode string
}
