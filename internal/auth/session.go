package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	SessionCookie = "infidrive_session"
	CSRFCookie    = "infidrive_csrf"
	CSRFHeader    = "X-CSRF-Token"
	sessionTTL    = 12 * time.Hour
)

var ErrUnauthorized = errors.New("требуется авторизация")

type sessionPayload struct {
	User string `json:"u"`
	Exp  int64  `json:"e"`
	ID   string `json:"i"`
}

// Manager хранит сессии администратора (подписанные cookies + server-side revoke).
type Manager struct {
	secret []byte
	secure bool
	mu     sync.Mutex
	alive  map[string]time.Time
}

func NewManager(secret string, cookieSecure bool) *Manager {
	return &Manager{
		secret: []byte(secret),
		secure: cookieSecure,
		alive:  map[string]time.Time{},
	}
}

func (m *Manager) Create(w http.ResponseWriter, username string) error {
	id, err := randomID(16)
	if err != nil {
		return err
	}
	exp := time.Now().Add(sessionTTL)
	payload := sessionPayload{User: username, Exp: exp.Unix(), ID: id}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	sig := m.sign(raw)
	value := base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(sig)
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  exp,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	m.mu.Lock()
	m.alive[id] = exp
	m.gcLocked()
	m.mu.Unlock()
	return nil
}

func (m *Manager) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (m *Manager) User(r *http.Request) (string, error) {
	c, err := r.Cookie(SessionCookie)
	if err != nil || c.Value == "" {
		return "", ErrUnauthorized
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 2 {
		return "", ErrUnauthorized
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", ErrUnauthorized
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrUnauthorized
	}
	if !hmac.Equal(sig, m.sign(raw)) {
		return "", ErrUnauthorized
	}
	var p sessionPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", ErrUnauthorized
	}
	if time.Now().Unix() > p.Exp || p.User == "" || p.ID == "" {
		return "", ErrUnauthorized
	}
	m.mu.Lock()
	exp, ok := m.alive[p.ID]
	m.mu.Unlock()
	if !ok || time.Now().After(exp) {
		// После рестарта процесса cookie ещё валидна по подписи — принимаем и восстанавливаем.
		if time.Unix(p.Exp, 0).After(time.Now()) {
			m.mu.Lock()
			m.alive[p.ID] = time.Unix(p.Exp, 0)
			m.mu.Unlock()
			return p.User, nil
		}
		return "", ErrUnauthorized
	}
	return p.User, nil
}

func (m *Manager) Revoke(r *http.Request) {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 2 {
		return
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return
	}
	var p sessionPayload
	if json.Unmarshal(raw, &p) != nil {
		return
	}
	m.mu.Lock()
	delete(m.alive, p.ID)
	m.mu.Unlock()
}

func (m *Manager) sign(raw []byte) []byte {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write(raw)
	return mac.Sum(nil)
}

func (m *Manager) gcLocked() {
	now := time.Now()
	for id, exp := range m.alive {
		if now.After(exp) {
			delete(m.alive, id)
		}
	}
}

func randomID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
