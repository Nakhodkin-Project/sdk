package tgscreen

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"

// Step is one stage of a Conversation: a Prompt is shown in the chat's
// anchor, the user's reply is collected, and a Confirm screen asks them to
// approve it before moving on.
type Step struct {
	// Prompt is shown (in the anchor) when this step begins.
	Prompt Screen

	// SaveAs is the Session key the user's reply is stored under, using the
	// default OnInput. Unused if OnInput is set.
	SaveAs string

	// Confirm is shown after the reply is accepted, asking the user to
	// approve it. Unused if OnInput is set.
	Confirm Screen

	// OnInput, if set, replaces the default "store under SaveAs, show
	// Confirm" behaviour. It returns the screen to show for confirmation,
	// or ok=false to keep waiting on this step (e.g. after reporting a
	// validation error via ctx.Flash).
	OnInput func(ctx *Context, msg *tgbotapi.Message) (confirm Screen, ok bool, err error)
}

// Conversation is a sequence of Steps, each collecting and confirming one
// piece of input, run one at a time against a chat's anchor message.
type Conversation struct {
	Steps []Step

	// OnDone runs once every step has been confirmed.
	OnDone func(ctx *Context) error
}

// Start begins the conversation for ctx.ChatID under the given name (used to
// resume it via Session.Conv) and shows the first step's prompt.
func (c *Conversation) Start(ctx *Context, name string) error {
	ctx.Session.SetConv(&ConvState{Name: name, Mark: ctx.Session.PageLen()})
	return ctx.Show(ctx.ChatID, c.Steps[0].Prompt)
}

// Advance processes a message sent while the conversation is waiting on
// input for its current step.
func (c *Conversation) Advance(ctx *Context, msg *tgbotapi.Message) error {
	conv := ctx.Session.Conv()
	step := c.Steps[conv.Step]

	onInput := step.OnInput
	if onInput == nil {
		onInput = func(ctx *Context, msg *tgbotapi.Message) (Screen, bool, error) {
			ctx.Session.Set(step.SaveAs, *msg)
			return step.Confirm, true, nil
		}
	}

	confirmScreen, ok, err := onInput(ctx, msg)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	ctx.Track(ctx.ChatID, *msg)
	if err := ctx.Resend(ctx.ChatID, confirmScreen); err != nil {
		return err
	}

	conv.Blocked = true
	ctx.Session.SetConv(conv)
	return nil
}

// Confirm handles a yes/no response to the active step's confirmation
// screen. yes=false re-shows the current step's prompt so the user can redo
// it; yes=true advances to the next step, or runs OnDone once every step has
// been confirmed.
func (c *Conversation) Confirm(ctx *Context, yes bool) error {
	conv := ctx.Session.Conv()
	if conv == nil {
		return nil
	}
	conv.Blocked = false

	if !yes {
		if err := ctx.Undo(ctx.ChatID, conv.Mark); err != nil {
			return err
		}
		ctx.Session.SetConv(conv)
		return ctx.Show(ctx.ChatID, c.Steps[conv.Step].Prompt)
	}

	conv.Step++
	if conv.Step >= len(c.Steps) {
		ctx.Session.SetConv(nil)
		if c.OnDone != nil {
			return c.OnDone(ctx)
		}
		return nil
	}

	conv.Mark = ctx.Session.PageLen()
	ctx.Session.SetConv(conv)
	return ctx.Show(ctx.ChatID, c.Steps[conv.Step].Prompt)
}
