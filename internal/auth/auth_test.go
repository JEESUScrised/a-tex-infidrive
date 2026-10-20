package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPasswordAndSession(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if CheckPassword(hash, "wrong") == nil {
		t.Fatal("expected fail")
	}
	if err := CheckPassword(hash, "correct-horse-battery"); err != nil {
		t.Fatal(err)
	}

	m := NewManager("session-secret-at-least-32-chars-long!!", false)
	rec := httptest.NewRecorder()
	if err := m.Create(rec, "admin"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	user, err := m.User(req)
	if err != nil || user != "admin" {
		t.Fatalf("%q %v", user, err)
	}
}

func TestCSRF(t *testing.T) {
	m := NewManager("session-secret-at-least-32-chars-long!!", false)
	rec := httptest.NewRecorder()
	tok, err := m.IssueCSRF(rec)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/files", nil)
	req.Header.Set(CSRFHeader, tok)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	if !CheckCSRF(req) {
		t.Fatal("csrf should pass")
	}
	req2 := httptest.NewRequest(http.MethodPost, "/api/files", nil)
	if CheckCSRF(req2) {
		t.Fatal("csrf should fail")
	}
}

func TestLoginLimiter(t *testing.T) {
	l := NewLoginLimiter(3, time.Minute)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "1.2.3.4:55"
	for i := 0; i < 3; i++ {
		if !l.Allow(req) {
			t.Fatalf("attempt %d blocked", i)
		}
		l.Fail(req)
	}
	if l.Allow(req) {
		t.Fatal("expected block")
	}
}
