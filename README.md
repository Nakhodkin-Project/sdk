# tg-bot-sdk

A small Go SDK for building Telegram bots whose UI lives in **one anchor
message** instead of a scrolling feed of menus. Navigation edits that single
message in place, so buttons always stay in the same spot on screen —
convenient for one-handed/thumb use.

```go
import "github.com/matvievsky/tg-bot-sdk/pkg/tgscreen"
```

## Concepts

### Screen

A `Screen` is a view: text plus an inline keyboard.

```go
var mainMenu = tgscreen.Screen{
    Text: "Чем я могу помочь?",
    Markup: tgbotapi.NewInlineKeyboardMarkup(
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData("🔍 Искать", "look_around"),
        ),
    ),
}
```

### Bot

`tgscreen.Bot` wraps `*tgbotapi.BotAPI` and adds screen-oriented helpers,
backed by a `SessionStore` (in-memory by default, via `NewMemoryStore`):

```go
bot := tgscreen.New(api, nil) // nil -> in-memory session store
```

| Method | What it does |
|---|---|
| `Show(chatID, screen)` | Edits the chat's anchor message in place (text + keyboard in one request), or sends a new message and makes it the anchor if there isn't one yet. Self-healing: if the anchor can't be edited (the user deleted it, it's too old, etc.), `Show` drops it and sends a fresh anchor instead of dead-ending. |
| `Resend(chatID, screen)` | Deletes the current anchor (best-effort) and sends `screen` as a new anchor at the bottom of the chat, leaving tracked page messages in place. Use when other messages were sent since the anchor was last shown. |
| `Reset(chatID, screen)` | Clears tracked page messages and the old anchor, then shows `screen` as a brand-new anchor. Use this for "back to menu"-style transitions. |
| `Track(chatID, msg)` | Remembers `msg` as part of the current screen, so `ClearPage`/`Reset` will delete it later. |
| `ClearPage(chatID)` | Deletes every message tracked via `Track`. |
| `Flash(chatID, text, ttl)` | Sends a message that deletes itself after `ttl` — for transient errors/notices. |

All of these return `error`.

### Session

Each chat gets a `*tgscreen.Session` (via `bot.Sessions.Get(chatID)`) holding:

- `Anchor()` / `SetAnchor()` — the current anchor message
- `Page()` / `AppendPage()` / `TakePage()` — messages tracked for the current screen
- `Conv()` / `SetConv()` — active `*ConvState` (name, step, blocked), or nil
- arbitrary app data via `Set(key, value)` and the generic `tgscreen.Get[T](session, key)`

`Session` is safe for concurrent use.

### Router

`Router` dispatches updates without one giant `switch`:

```go
router := tgscreen.NewRouter().
    Command("/start", func(ctx *tgscreen.Context) error {
        return ctx.Reset(ctx.ChatID, mainMenu)
    }).
    Callback("look_around", func(ctx *tgscreen.Context) error {
        return ctx.Show(ctx.ChatID, searchScreen)
    })

for update := range updates {
    if err := router.Dispatch(bot, update); err != nil {
        log.Println(err)
    }
}
```

- `Command(text, handler)` — matches an exact incoming message text (e.g. `/start`).
- `Callback(data, handler)` — matches inline-keyboard callback data. The
  callback query is answered automatically before the handler runs.
- `Fallback(handler)` — runs when nothing else matches.
- `Conversation(name, conv)` — registers a multi-step `Conversation` (see below).
- `ConfirmYes(data)` / `ConfirmNo(data)` — wire callback data to confirm or
  redo the active conversation's current step.

`ctx *tgscreen.Context` embeds `*Bot` and also carries `ChatID`, `Update`,
`Message`, and `Session`, so handlers can call `ctx.Show`, `ctx.Track`,
`ctx.Session.Set`, etc. directly.

### Conversation

A `Conversation` is a sequence of `Step`s — "ask, collect a reply, confirm,
move on" — driven through the same anchor message:

```go
var addItem = &tgscreen.Conversation{
    Steps: []tgscreen.Step{
        {
            Prompt:  tgscreen.Screen{Text: "Что вы потеряли?"},
            SaveAs:  "name",
            Confirm: confirmScreen,
        },
        {
            Prompt:  tgscreen.Screen{Text: "Где потеряли?"},
            SaveAs:  "place",
            Confirm: confirmScreen,
        },
    },
    OnDone: func(ctx *tgscreen.Context) error {
        name, _ := tgscreen.Get[tgbotapi.Message](ctx.Session, "name")
        place, _ := tgscreen.Get[tgbotapi.Message](ctx.Session, "place")
        // ... save the item, then:
        return ctx.Reset(ctx.ChatID, mainMenu)
    },
}

router.Conversation("add_item", addItem)
router.ConfirmYes("yes")
router.ConfirmNo("no")
```

- Start it from a handler with `conv.Start(ctx, "add_item")` (or
  `router.Start(ctx, "add_item")`).
- While a step is awaiting input, `Router.Dispatch` routes incoming messages
  to `conv.Advance`, which by default stores the message under `SaveAs` and
  shows `Confirm`.
- A step can set `OnInput` instead of `SaveAs`/`Confirm` to run custom logic
  (e.g. geocoding, validation) before deciding what confirmation screen to
  show, or to return `ok=false` to keep waiting on the same step (after
  reporting an error via `ctx.Flash`).
- `ConfirmYes`/`ConfirmNo` callbacks call `conv.Confirm(ctx, true/false)`:
  `false` re-shows the current step's prompt so the user can redo it; `true`
  advances to the next step, or runs `OnDone` once all steps are confirmed.

## Example

See [`cmd/bot/main.go`](cmd/bot/main.go) for a complete runnable example: a
main menu and a two-step survey conversation, both driven through one anchor
message. Run it with:

```sh
TG_BOT_API_KEY=<your-token> go run ./cmd/bot
```
