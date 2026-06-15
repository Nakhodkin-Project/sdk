package tgscreen_test

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/Nakhodkin-Project/sdk/pkg/tgscreen"
)

// fakeCall records one request made against fakeTelegram.
type fakeCall struct {
	Method string
	Form   url.Values
}

// fakeTelegram is an http.RoundTripper standing in for the Telegram Bot API:
// it records every call and returns canned responses without touching the
// network.
type fakeTelegram struct {
	mu     sync.Mutex
	nextID int
	calls  []fakeCall
}

func (f *fakeTelegram) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.ParseForm(); err != nil {
		return nil, err
	}
	method := path.Base(req.URL.Path)

	f.mu.Lock()
	f.calls = append(f.calls, fakeCall{Method: method, Form: req.PostForm})
	f.mu.Unlock()

	var result string
	switch method {
	case "deleteMessage", "answerCallbackQuery":
		result = "true"
	case "getMe":
		result = `{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}`
	default:
		chatID := req.PostForm.Get("chat_id")
		msgID := req.PostForm.Get("message_id")
		if msgID == "" {
			f.mu.Lock()
			f.nextID++
			msgID = strconv.Itoa(f.nextID)
			f.mu.Unlock()
		}
		result = fmt.Sprintf(`{"message_id":%s,"chat":{"id":%s}}`, msgID, chatID)
	}

	body := fmt.Sprintf(`{"ok":true,"result":%s}`, result)
	return &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func (f *fakeTelegram) callsTo(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	n := 0
	for _, c := range f.calls {
		if c.Method == method {
			n++
		}
	}
	return n
}

// newTestBot returns a Bot backed by a fakeTelegram transport, so tests can
// exercise Bot/Router/Conversation without network access.
func newTestBot() (*tgscreen.Bot, *fakeTelegram) {
	fake := &fakeTelegram{}
	api, _ := tgbotapi.NewBotAPIWithClient("TEST:TOKEN", tgbotapi.APIEndpoint, &http.Client{Transport: fake})
	return tgscreen.New(api, nil), fake
}
