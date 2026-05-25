package proxy

import (
	"container/list"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"

	"relay-ai/internal/config"
)

const (
	defaultMaxSessions     = 256
	defaultMaxSessionBytes = 512 * 1024 * 1024 // 512 MiB
	defaultSessionTTL      = 7 * 24 * time.Hour
)

// ChatMessage mirrors codex-relay's ChatMessage for session storage.
type ChatMessage struct {
	Role             string           `json:"role"`
	Content          json.RawMessage  `json:"content,omitempty"`
	ReasoningContent *string          `json:"reasoning_content,omitempty"`
	ToolCalls        []json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID       *string          `json:"tool_call_id,omitempty"`
	Name             *string          `json:"name,omitempty"`
}

type sessionEntry struct {
	messages    []ChatMessage
	bytes       int
	lastUsedAt  time.Time
	orderElem   *list.Element
}

type reasoningEntry struct {
	value      string
	bytes      int
	lastUsedAt time.Time
	orderElem  *list.Element
}

// SessionStore mirrors codex-relay's SessionStore.
// It stores conversation history by response_id and reasoning by call_id/fingerprint.
type SessionStore struct {
	mu sync.Mutex

	sessions    map[string]*sessionEntry
	sessionLRU  *list.List

	reasoning    map[string]*reasoningEntry
	reasoningLRU *list.List

	turnReasoning    map[uint64]*reasoningEntry
	turnReasoningLRU *list.List

	storedBytes    int
	maxSessions    int
	maxStoredBytes int
	ttl            time.Duration

	providers []config.Provider // for model transforms in history replay
}

// NewSessionStore creates a SessionStore with default limits.
func NewSessionStore() *SessionStore {
	return newSessionStoreWithLimits(defaultMaxSessions, defaultMaxSessionBytes, defaultSessionTTL)
}

func newSessionStoreWithLimits(maxSessions, maxStoredBytes int, ttl time.Duration) *SessionStore {
	if maxSessions < 1 {
		maxSessions = 1
	}
	if maxStoredBytes < 1 {
		maxStoredBytes = 1
	}
	if ttl < time.Second {
		ttl = time.Second
	}
	return &SessionStore{
		sessions:         make(map[string]*sessionEntry),
		sessionLRU:       list.New(),
		reasoning:        make(map[string]*reasoningEntry),
		reasoningLRU:     list.New(),
		turnReasoning:    make(map[uint64]*reasoningEntry),
		turnReasoningLRU: list.New(),
		maxSessions:      maxSessions,
		maxStoredBytes:   maxStoredBytes,
		ttl:              ttl,
	}
}

// SetProviders lets the session store access model mappings for history replay.
func (s *SessionStore) SetProviders(providers []config.Provider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers = providers
}

// GetHistory retrieves message history for a previous response_id.
func (s *SessionStore) GetHistory(responseID string) []ChatMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enforceLimits()

	entry, ok := s.sessions[responseID]
	if !ok {
		return nil
	}
	entry.lastUsedAt = time.Now()
	s.sessionLRU.MoveToBack(entry.orderElem)
	return append([]ChatMessage(nil), entry.messages...)
}

// NewID generates a fresh response_id.
func (s *SessionStore) NewID() string {
	return fmt.Sprintf("resp_%d_%d", time.Now().UnixNano(), s.nextSeq())
}

var seqCounter struct {
	sync.Mutex
	n int64
}

func (s *SessionStore) nextSeq() int64 {
	seqCounter.Lock()
	defer seqCounter.Unlock()
	seqCounter.n++
	return seqCounter.n
}

// SaveWithID stores messages under a pre-allocated response_id.
func (s *SessionStore) SaveWithID(id string, messages []ChatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.insertSession(id, messages)
	s.enforceLimits()
}

// Save allocates an ID and stores messages atomically.
func (s *SessionStore) Save(messages []ChatMessage) string {
	id := s.NewID()
	s.SaveWithID(id, messages)
	return id
}

// StoreReasoning stores reasoning_content keyed by tool call_id.
func (s *SessionStore) StoreReasoning(callID, reasoning string) {
	if reasoning == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.insertReasoning(callID, reasoning)
	s.enforceLimits()
}

// GetReasoning retrieves stored reasoning for a call_id.
func (s *SessionStore) GetReasoning(callID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enforceLimits()
	entry, ok := s.reasoning[callID]
	if !ok {
		return ""
	}
	entry.lastUsedAt = time.Now()
	s.reasoningLRU.MoveToBack(entry.orderElem)
	return entry.value
}

// StoreTurnReasoning stores reasoning indexed by assistant message fingerprint.
func (s *SessionStore) StoreTurnReasoning(assistant *ChatMessage, reasoning string) {
	if reasoning == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	content := assistant.textContent()
	if content == "" {
		return
	}
	key := contentKey(content)
	s.insertTurnReasoning(key, reasoning)

	// Also store under each tool call_id
	for _, tc := range assistant.ToolCalls {
		var callID string
		if err := json.Unmarshal(tc, &struct{ ID *string }{&callID}); err == nil && callID != "" {
			s.insertReasoning(callID, reasoning)
		}
	}
	s.enforceLimits()
}

// GetTurnReasoning retrieves reasoning for an assistant turn by content fingerprint.
func (s *SessionStore) GetTurnReasoning(assistant *ChatMessage) string {
	content := assistant.textContent()
	if content == "" {
		return ""
	}
	key := contentKey(content)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enforceLimits()
	entry, ok := s.turnReasoning[key]
	if !ok {
		return ""
	}
	entry.lastUsedAt = time.Now()
	s.turnReasoningLRU.MoveToBack(entry.orderElem)
	return entry.value
}

// Cleanup removes expired entries.
func (s *SessionStore) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enforceLimits()
}

// --- internal ---

func msgBytes(messages []ChatMessage) int {
	n := 0
	for _, m := range messages {
		n += len(m.Role) + len(m.Content)
		if m.ReasoningContent != nil {
			n += len(*m.ReasoningContent)
		}
		for _, tc := range m.ToolCalls {
			n += len(tc)
		}
		if m.ToolCallID != nil {
			n += len(*m.ToolCallID)
		}
		if m.Name != nil {
			n += len(*m.Name)
		}
	}
	return n
}

func contentKey(content string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(content))
	return h.Sum64()
}

func (m *ChatMessage) textContent() string {
	if len(m.Content) == 0 {
		return ""
	}
	// Try as string first
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		return s
	}
	// Try as array of content parts
	var parts []struct {
		Text string `json:"text"`
		Type string `json:"type"`
	}
	if json.Unmarshal(m.Content, &parts) == nil {
		var texts []string
		for _, p := range parts {
			if p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, "")
	}
	return ""
}

func (s *SessionStore) insertSession(id string, messages []ChatMessage) {
	bytes := msgBytes(messages)
	if bytes > s.maxStoredBytes {
		return // too large, skip
	}
	if old, ok := s.sessions[id]; ok {
		s.storedBytes -= old.bytes
		s.sessionLRU.Remove(old.orderElem)
	}
	s.storedBytes += bytes
	elem := s.sessionLRU.PushBack(id)
	s.sessions[id] = &sessionEntry{
		messages:   messages,
		bytes:      bytes,
		lastUsedAt: time.Now(),
		orderElem:  elem,
	}
}

func (s *SessionStore) insertReasoning(callID, reasoning string) {
	bytes := len(callID) + len(reasoning)
	if old, ok := s.reasoning[callID]; ok {
		s.storedBytes -= old.bytes
		s.reasoningLRU.Remove(old.orderElem)
	}
	s.storedBytes += bytes
	elem := s.reasoningLRU.PushBack(callID)
	s.reasoning[callID] = &reasoningEntry{
		value:      reasoning,
		bytes:      bytes,
		lastUsedAt: time.Now(),
		orderElem:  elem,
	}
}

func (s *SessionStore) insertTurnReasoning(key uint64, reasoning string) {
	bytes := 8 + len(reasoning)
	if old, ok := s.turnReasoning[key]; ok {
		s.storedBytes -= old.bytes
		s.turnReasoningLRU.Remove(old.orderElem)
	}
	s.storedBytes += bytes
	elem := s.turnReasoningLRU.PushBack(key)
	s.turnReasoning[key] = &reasoningEntry{
		value:      reasoning,
		bytes:      bytes,
		lastUsedAt: time.Now(),
		orderElem:  elem,
	}
}

func (s *SessionStore) enforceLimits() {
	cutoff := time.Now().Add(-s.ttl)

	// Evict expired sessions
	for s.sessionLRU.Len() > 0 {
		front := s.sessionLRU.Front()
		id := front.Value.(string)
		entry := s.sessions[id]
		if entry.lastUsedAt.After(cutoff) {
			break
		}
		s.sessionLRU.Remove(front)
		s.storedBytes -= entry.bytes
		delete(s.sessions, id)
	}

	// Evict expired reasoning
	for s.reasoningLRU.Len() > 0 {
		front := s.reasoningLRU.Front()
		id := front.Value.(string)
		entry := s.reasoning[id]
		if entry.lastUsedAt.After(cutoff) {
			break
		}
		s.reasoningLRU.Remove(front)
		s.storedBytes -= entry.bytes
		delete(s.reasoning, id)
	}

	// Evict expired turn reasoning
	for s.turnReasoningLRU.Len() > 0 {
		front := s.turnReasoningLRU.Front()
		key := front.Value.(uint64)
		entry := s.turnReasoning[key]
		if entry.lastUsedAt.After(cutoff) {
			break
		}
		s.turnReasoningLRU.Remove(front)
		s.storedBytes -= entry.bytes
		delete(s.turnReasoning, key)
	}

	// Evict over-count sessions
	for len(s.sessions) > s.maxSessions && s.sessionLRU.Len() > 0 {
		s.removeOldestSession()
	}

	// Evict over-bytes sessions
	for s.storedBytes > s.maxStoredBytes && len(s.sessions) > 1 && s.sessionLRU.Len() > 0 {
		s.removeOldestSession()
	}
	for s.storedBytes > s.maxStoredBytes && s.reasoningLRU.Len() > 0 {
		s.removeOldestReasoning()
	}
	for s.storedBytes > s.maxStoredBytes && s.turnReasoningLRU.Len() > 0 {
		s.removeOldestTurnReasoning()
	}
}

func (s *SessionStore) removeOldestSession() {
	front := s.sessionLRU.Front()
	if front == nil {
		return
	}
	id := front.Value.(string)
	entry := s.sessions[id]
	s.sessionLRU.Remove(front)
	s.storedBytes -= entry.bytes
	delete(s.sessions, id)
}

func (s *SessionStore) removeOldestReasoning() {
	front := s.reasoningLRU.Front()
	if front == nil {
		return
	}
	id := front.Value.(string)
	entry := s.reasoning[id]
	s.reasoningLRU.Remove(front)
	s.storedBytes -= entry.bytes
	delete(s.reasoning, id)
}

func (s *SessionStore) removeOldestTurnReasoning() {
	front := s.turnReasoningLRU.Front()
	if front == nil {
		return
	}
	key := front.Value.(uint64)
	entry := s.turnReasoning[key]
	s.turnReasoningLRU.Remove(front)
	s.storedBytes -= entry.bytes
	delete(s.turnReasoning, key)
}
