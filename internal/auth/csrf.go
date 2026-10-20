package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"
)

// IssueCSRF выставляет читаемую JS cookie с CSRF-токеном (double-submit).
func (m *Manager) IssueCSRF(w http.ResponseWriter) (string, error) {
	tok, err := randomID(24)
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookie,
		Value:    tok,
		Path:     "/",
		HttpOnly: false,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(sessionTTL),
		MaxAge:   int(sessionTTL.Seconds()),
	})
	return tok, nil
}

// CheckCSRF сравнивает заголовок и cookie.
func CheckCSRF(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	h := r.Header.Get(CSRFHeader)
	if h == "" {
		return false
	}
	c, err := r.Cookie(CSRFCookie)
	if err != nil || c.Value == "" {
		return false
	}
	return subtleEqual(h, c.Value)
}

func subtleEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// RandomToken — для тестов.
func RandomToken() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
