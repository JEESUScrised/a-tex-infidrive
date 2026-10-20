package youtube

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// ScopeUpload — минимальный scope для videos.insert.
const ScopeUpload = "https://www.googleapis.com/auth/youtube.upload"

// ScopeReadonly — чтение канала (название) после подключения.
const ScopeReadonly = "https://www.googleapis.com/auth/youtube.readonly"

// Credentials — OAuth-клиент из Google Cloud Console.
type Credentials struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	// Endpoint пуст для боевого Google; в тестах подставляется httptest.
	Endpoint oauth2.Endpoint
}

func (c Credentials) valid() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.RedirectURL != ""
}

func (c Credentials) config() *oauth2.Config {
	ep := google.Endpoint
	if c.Endpoint.AuthURL != "" && c.Endpoint.TokenURL != "" {
		ep = c.Endpoint
	}
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		RedirectURL:  c.RedirectURL,
		Scopes:       []string{ScopeUpload, ScopeReadonly},
		Endpoint:     ep,
	}
}

// StateStore хранит одноразовые OAuth state для защиты от CSRF.
type StateStore struct {
	mu   sync.Mutex
	data map[string]time.Time
	ttl  time.Duration
}

// NewStateStore создаёт хранилище state с временем жизни.
func NewStateStore(ttl time.Duration) *StateStore {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &StateStore{data: map[string]time.Time{}, ttl: ttl}
}

// Issue создаёт новый state.
func (s *StateStore) Issue() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	state := hex.EncodeToString(b[:])
	s.mu.Lock()
	s.gcLocked()
	s.data[state] = time.Now().Add(s.ttl)
	s.mu.Unlock()
	return state, nil
}

// Consume проверяет и удаляет state. Повторное использование невозможно.
func (s *StateStore) Consume(state string) bool {
	if state == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	exp, ok := s.data[state]
	if !ok {
		return false
	}
	delete(s.data, state)
	return time.Now().Before(exp)
}

func (s *StateStore) gcLocked() {
	now := time.Now()
	for k, exp := range s.data {
		if now.After(exp) {
			delete(s.data, k)
		}
	}
}

// TokenRecord — сохранённые токены без вывода в логи.
type TokenRecord struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	Expiry       time.Time
}

func tokenFromOAuth(t *oauth2.Token) TokenRecord {
	if t == nil {
		return TokenRecord{}
	}
	return TokenRecord{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    t.TokenType,
		Expiry:       t.Expiry,
	}
}

func (r TokenRecord) toOAuth() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  r.AccessToken,
		RefreshToken: r.RefreshToken,
		TokenType:    r.TokenType,
		Expiry:       r.Expiry,
	}
}

// TokenPersister сохраняет и читает токены.
type TokenPersister interface {
	LoadToken(ctx context.Context, provider string) (TokenRecord, error)
	SaveToken(ctx context.Context, provider string, tok TokenRecord) error
	DeleteToken(ctx context.Context, provider string) error
}

const providerGoogle = "google_youtube"

// savingTokenSource обновляет access token и сразу сохраняет его.
type savingTokenSource struct {
	base TokenPersister
	src  oauth2.TokenSource
	mu   sync.Mutex
	tok  *oauth2.Token
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tok, err := s.src.Token()
	if err != nil {
		return nil, err
	}
	if s.tok == nil || tok.AccessToken != s.tok.AccessToken || !tok.Expiry.Equal(s.tok.Expiry) {
		rec := tokenFromOAuth(tok)
		if rec.RefreshToken == "" && s.tok != nil {
			rec.RefreshToken = s.tok.RefreshToken
		}
		if err := s.base.SaveToken(context.Background(), providerGoogle, rec); err != nil {
			return nil, err
		}
		s.tok = tok
	}
	return tok, nil
}
