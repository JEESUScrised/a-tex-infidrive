package youtube

import (
	"context"
	"sync"
)

// MemoryTokenStore — хранилище токенов для тестов.
type MemoryTokenStore struct {
	mu   sync.Mutex
	data map[string]TokenRecord
}

// NewMemoryTokenStore создаёт пустое хранилище.
func NewMemoryTokenStore() *MemoryTokenStore {
	return &MemoryTokenStore{data: map[string]TokenRecord{}}
}

func (m *MemoryTokenStore) LoadToken(_ context.Context, provider string) (TokenRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tok, ok := m.data[provider]
	if !ok {
		return TokenRecord{}, errNotFound
	}
	return tok, nil
}

func (m *MemoryTokenStore) SaveToken(_ context.Context, provider string, tok TokenRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.data[provider]; ok && tok.RefreshToken == "" {
		tok.RefreshToken = old.RefreshToken
	}
	m.data[provider] = tok
	return nil
}

func (m *MemoryTokenStore) DeleteToken(_ context.Context, provider string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, provider)
	return nil
}

type notFoundError struct{}

func (notFoundError) Error() string { return "не найдено" }

var errNotFound = notFoundError{}
