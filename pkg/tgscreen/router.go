package tgscreen

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Context carries everything a HandlerFunc needs to respond to an update.
type Context struct {
	*Bot
	ChatID  int64
	Update  tgbotapi.Update
	Message *tgbotapi.Message
	Session *Session
}

// HandlerFunc reacts to an update routed by a Router.
type HandlerFunc func(ctx *Context) error

// Router dispatches updates to registered handlers based on callback data,
// command text, or the chat's active Conversation.
type Router struct {
	callbacks     map[string]HandlerFunc
	commands      map[string]HandlerFunc
	conversations map[string]*Conversation
	fallback      HandlerFunc
	myChatMember  HandlerFunc
}

// NewRouter returns an empty Router.
func NewRouter() *Router {
	return &Router{
		callbacks:     make(map[string]HandlerFunc),
		commands:      make(map[string]HandlerFunc),
		conversations: make(map[string]*Conversation),
	}
}

// Callback registers h to run when an inline keyboard button with the given
// callback data is pressed.
func (r *Router) Callback(data string, h HandlerFunc) *Router {
	r.callbacks[data] = h
	return r
}

// Command registers h to run when an incoming message's text matches cmd
// exactly (e.g. "/start").
func (r *Router) Command(cmd string, h HandlerFunc) *Router {
	r.commands[cmd] = h
	return r
}

// Conversation registers a multi-step Conversation under name, so it can be
// started with Start and resumed via the chat's Session.Conv.
func (r *Router) Conversation(name string, c *Conversation) *Router {
	r.conversations[name] = c
	return r
}

// Fallback registers h to run for updates that no other registration
// matches.
func (r *Router) Fallback(h HandlerFunc) *Router {
	r.fallback = h
	return r
}

// MyChatMember registers h to run when the bot's membership status in a chat
// changes (e.g. the user blocks, unblocks, or restarts the bot).
func (r *Router) MyChatMember(h HandlerFunc) *Router {
	r.myChatMember = h
	return r
}

// ConfirmYes registers callback data that confirms the active conversation's
// current step and advances to the next one.
func (r *Router) ConfirmYes(data string) *Router {
	return r.Callback(data, func(ctx *Context) error { return r.confirm(ctx, true) })
}

// ConfirmNo registers callback data that rejects the active conversation's
// current step, re-showing its prompt so the user can redo it.
func (r *Router) ConfirmNo(data string) *Router {
	return r.Callback(data, func(ctx *Context) error { return r.confirm(ctx, false) })
}

func (r *Router) confirm(ctx *Context, yes bool) error {
	conv := ctx.Session.Conv()
	if conv == nil {
		return r.runFallback(ctx)
	}
	c, ok := r.conversations[conv.Name]
	if !ok {
		return r.runFallback(ctx)
	}
	return c.Confirm(ctx, yes)
}

// Start begins the named Conversation for ctx.ChatID, showing its first
// step's prompt.
func (r *Router) Start(ctx *Context, name string) error {
	c, ok := r.conversations[name]
	if !ok {
		return fmt.Errorf("tgscreen: unknown conversation %q", name)
	}
	return c.Start(ctx, name)
}

// Dispatch routes u to the matching registration. Callback queries are
// answered automatically before their handler runs.
func (r *Router) Dispatch(b *Bot, u tgbotapi.Update) error {
	if u.MyChatMember != nil {
		chatID := u.MyChatMember.Chat.ID
		ctx := &Context{
			Bot:     b,
			ChatID:  chatID,
			Update:  u,
			Session: b.Sessions.Get(chatID),
		}
		if r.myChatMember != nil {
			return r.myChatMember(ctx)
		}
		return nil
	}

	msg := u.Message
	if msg == nil && u.CallbackQuery != nil {
		msg = u.CallbackQuery.Message
	}
	if msg == nil {
		return nil
	}

	chatID := msg.Chat.ID
	ctx := &Context{
		Bot:     b,
		ChatID:  chatID,
		Update:  u,
		Message: msg,
		Session: b.Sessions.Get(chatID),
	}

	if u.CallbackQuery != nil {
		if _, err := b.Request(tgbotapi.NewCallback(u.CallbackQuery.ID, "")); err != nil {
			return fmt.Errorf("tgscreen: answer callback: %w", err)
		}
		if h, ok := r.callbacks[u.CallbackQuery.Data]; ok {
			return h(ctx)
		}
		return r.runFallback(ctx)
	}

	if conv := ctx.Session.Conv(); conv != nil && !conv.Blocked {
		if c, ok := r.conversations[conv.Name]; ok {
			return c.Advance(ctx, u.Message)
		}
	}

	if u.Message != nil {
		if h, ok := r.commands[u.Message.Text]; ok {
			return h(ctx)
		}
	}

	return r.runFallback(ctx)
}

func (r *Router) runFallback(ctx *Context) error {
	if r.fallback == nil {
		return nil
	}
	return r.fallback(ctx)
}
