package tgscreen

import (
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ConvState tracks progress through a multi-step Conversation.
type ConvState struct {
	Name    string
	Step    int
	Blocked bool

	// Mark is the Session page length when the current step began. Confirm
	// uses it to undo only the messages this step tracked.
	Mark int
}

// Session holds the per-chat UI state needed to keep navigation pinned to a
// single anchor message, plus a small key/value store for app data.
type Session struct {
	mu sync.Mutex

	anchor   tgbotapi.Message
	promoted tgbotapi.Message // message currently occupying the promoted slot (e.g. ad above anchor)
	page     []tgbotapi.Message
	conv     *ConvState
	data     map[string]any
}

func newSession() *Session {
	return &Session{data: make(map[string]any)}
}

// Anchor returns the chat's current anchor message (the screen being shown).
func (s *Session) Anchor() tgbotapi.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.anchor
}

// SetAnchor replaces the chat's anchor message.
func (s *Session) SetAnchor(msg tgbotapi.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.anchor = msg
}

// Promoted returns the message currently occupying the promoted slot (e.g. an
// ad pinned above the anchor), or a zero Message if none has been set.
func (s *Session) Promoted() tgbotapi.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.promoted
}

// SetPromoted records msg as the current promoted slot message.
func (s *Session) SetPromoted(msg tgbotapi.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.promoted = msg
}

// Page returns the messages tracked as part of the current screen.
func (s *Session) Page() []tgbotapi.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	page := make([]tgbotapi.Message, len(s.page))
	copy(page, s.page)
	return page
}

// AppendPage adds msg to the messages tracked as part of the current screen.
func (s *Session) AppendPage(msg tgbotapi.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.page = append(s.page, msg)
}

// TakePage returns the tracked page messages and clears the list.
func (s *Session) TakePage() []tgbotapi.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	page := s.page
	s.page = nil
	return page
}

// PageLen returns the number of currently tracked page messages.
func (s *Session) PageLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.page)
}

// RemoveFromPage removes the tracked page message with the given messageID,
// if present, so a later ClearPage (or Reset) won't try to delete it again.
// It returns true if a message was removed.
func (s *Session) RemoveFromPage(messageID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, msg := range s.page {
		if msg.MessageID == messageID {
			s.page = append(s.page[:i], s.page[i+1:]...)
			return true
		}
	}
	return false
}

// DropPageSince removes and returns the tracked page messages from index n
// onward.
func (s *Session) DropPageSince(n int) []tgbotapi.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n < 0 || n > len(s.page) {
		return nil
	}
	dropped := s.page[n:]
	s.page = s.page[:n]
	return dropped
}

// Conv returns the active conversation state, or nil if there is none.
func (s *Session) Conv() *ConvState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conv
}

// SetConv sets the active conversation state. Pass nil to clear it.
func (s *Session) SetConv(c *ConvState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conv = c
}

// Set stores a value under key.
func (s *Session) Set(key string, v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = v
}

// Delete removes the value stored under key.
func (s *Session) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

// Get retrieves a typed value stored under key. ok is false if no value is
// stored under key or if it does not have type T.
func Get[T any](s *Session, key string) (value T, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, exists := s.data[key]
	if !exists {
		return value, false
	}
	value, ok = v.(T)
	return value, ok
}

// SessionStore manages per-chat sessions.
type SessionStore interface {
	// Get returns the session for chatID, creating it if necessary.
	Get(chatID int64) *Session
	// Reset discards the session for chatID, including its conversation
	// state, tracked page messages and stored data. The anchor message
	// itself is not deleted from the chat.
	Reset(chatID int64)
}

type memoryStore struct {
	mu sync.Mutex
	m  map[int64]*Session
}

// NewMemoryStore returns an in-memory, concurrency-safe SessionStore.
func NewMemoryStore() SessionStore {
	return &memoryStore{m: make(map[int64]*Session)}
}

func (m *memoryStore) Get(chatID int64) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.m[chatID]
	if !ok {
		s = newSession()
		m.m[chatID] = s
	}
	return s
}

func (m *memoryStore) Reset(chatID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, chatID)
}
