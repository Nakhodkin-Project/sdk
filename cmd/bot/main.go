// Command bot is a minimal demo of the tgscreen SDK: a main menu screen and
// a two-step conversation, both driven through a single anchor message.
package main

import (
	"fmt"
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/matvievsky/tg-bot-sdk/pkg/tgscreen"
)

const (
	cbMenu   = "menu"
	cbSurvey = "survey"
	cbYes    = "yes"
	cbNo     = "no"
)

var menuScreen = tgscreen.Screen{
	Text: "Главное меню",
	Markup: tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📝 Пройти опрос", cbSurvey),
		),
	),
}

var confirmScreen = tgscreen.Screen{
	Text: "Все верно?",
	Markup: tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Да", cbYes),
			tgbotapi.NewInlineKeyboardButtonData("❌ Нет", cbNo),
		),
	),
}

var survey = &tgscreen.Conversation{
	Steps: []tgscreen.Step{
		{
			Prompt:  tgscreen.Screen{Text: "Как вас зовут?"},
			SaveAs:  "name",
			Confirm: confirmScreen,
		},
		{
			Prompt:  tgscreen.Screen{Text: "Сколько вам лет?"},
			SaveAs:  "age",
			Confirm: confirmScreen,
		},
	},
	OnDone: func(ctx *tgscreen.Context) error {
		name, _ := tgscreen.Get[tgbotapi.Message](ctx.Session, "name")
		age, _ := tgscreen.Get[tgbotapi.Message](ctx.Session, "age")
		return ctx.Reset(ctx.ChatID, tgscreen.Screen{
			Text:   fmt.Sprintf("Спасибо, %s (%s)!", name.Text, age.Text),
			Markup: menuScreen.Markup,
		})
	},
}

func main() {
	token := os.Getenv("TG_BOT_API_KEY")
	if token == "" {
		log.Fatal("TG_BOT_API_KEY is required")
	}

	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("auth: %s", err)
	}
	log.Printf("authorized as %s", api.Self.UserName)

	bot := tgscreen.New(api, nil)

	router := tgscreen.NewRouter().
		Command("/start", func(ctx *tgscreen.Context) error {
			return ctx.Reset(ctx.ChatID, menuScreen)
		}).
		Callback(cbMenu, func(ctx *tgscreen.Context) error {
			return ctx.Reset(ctx.ChatID, menuScreen)
		}).
		Callback(cbSurvey, func(ctx *tgscreen.Context) error {
			return survey.Start(ctx, "survey")
		}).
		Conversation("survey", survey)

	router.ConfirmYes(cbYes)
	router.ConfirmNo(cbNo)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, err := api.GetUpdatesChan(u)
	if err != nil {
		log.Fatalf("updates: %s", err)
	}

	for update := range updates {
		if err := router.Dispatch(bot, update); err != nil {
			log.Println(err)
		}
	}
}
