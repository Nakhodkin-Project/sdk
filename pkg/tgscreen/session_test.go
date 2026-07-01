package tgscreen_test

import (
	"testing"

	"github.com/Nakhodkin-Project/sdk/pkg/tgscreen"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestMemoryStoreGetCreatesAndPersists(t *testing.T) {
	store := tgscreen.NewMemoryStore()

	s1 := store.Get(1)
	s1.Set("k", "v")

	s2 := store.Get(1)
	if s1 != s2 {
		t.Fatal("Get should return the same session for the same chatID")
	}

	v, ok := tgscreen.Get[string](s2, "k")
	if !ok || v != "v" {
		t.Fatalf("Get(k) = %q, %v; want %q, true", v, ok, "v")
	}

	store.Reset(1)
	s3 := store.Get(1)
	if s3 == s1 {
		t.Fatal("Get after Reset should return a new session")
	}
	if _, ok := tgscreen.Get[string](s3, "k"); ok {
		t.Fatal("data should not survive Reset")
	}
}

func TestSessionGetTypeMismatch(t *testing.T) {
	s := tgscreen.NewMemoryStore().Get(1)
	s.Set("k", 42)

	if _, ok := tgscreen.Get[string](s, "k"); ok {
		t.Fatal("Get with the wrong type should return ok=false")
	}
	if _, ok := tgscreen.Get[int](s, "missing"); ok {
		t.Fatal("Get for a missing key should return ok=false")
	}
}

func TestSessionDelete(t *testing.T) {
	s := tgscreen.NewMemoryStore().Get(1)
	s.Set("k", "v")
	s.Delete("k")

	if _, ok := tgscreen.Get[string](s, "k"); ok {
		t.Fatal("Get after Delete should return ok=false")
	}
}

func TestSessionAnchor(t *testing.T) {
	s := tgscreen.NewMemoryStore().Get(1)

	if got := s.Anchor().MessageID; got != 0 {
		t.Fatalf("Anchor().MessageID = %d, want 0 before SetAnchor", got)
	}

	s.SetAnchor(tgbotapi.Message{MessageID: 7})
	if got := s.Anchor().MessageID; got != 7 {
		t.Fatalf("Anchor().MessageID = %d, want 7", got)
	}
}

func TestSessionPageTracking(t *testing.T) {
	s := tgscreen.NewMemoryStore().Get(1)

	s.AppendPage(tgbotapi.Message{MessageID: 1})
	s.AppendPage(tgbotapi.Message{MessageID: 2})

	if got := s.Page(); len(got) != 2 {
		t.Fatalf("Page() = %v, want 2 messages", got)
	}

	taken := s.TakePage()
	if len(taken) != 2 {
		t.Fatalf("TakePage() returned %d messages, want 2", len(taken))
	}
	if got := s.Page(); len(got) != 0 {
		t.Fatalf("Page() after TakePage = %v, want empty", got)
	}
}

func TestSessionConvState(t *testing.T) {
	s := tgscreen.NewMemoryStore().Get(1)

	if s.Conv() != nil {
		t.Fatal("Conv() should be nil before SetConv")
	}

	s.SetConv(&tgscreen.ConvState{Name: "add_item", Step: 1})
	got := s.Conv()
	if got == nil || got.Name != "add_item" || got.Step != 1 {
		t.Fatalf("Conv() = %+v, want {Name: add_item, Step: 1}", got)
	}

	s.SetConv(nil)
	if s.Conv() != nil {
		t.Fatal("Conv() should be nil after SetConv(nil)")
	}
}
