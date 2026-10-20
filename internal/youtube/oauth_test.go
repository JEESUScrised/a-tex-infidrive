package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestStateRejectsReplayAndUnknown(t *testing.T) {
	s := NewStateStore(time.Minute)
	st, err := s.Issue()
	if err != nil {
		t.Fatal(err)
	}
	if !s.Consume(st) {
		t.Fatal("first consume")
	}
	if s.Consume(st) {
		t.Fatal("replay accepted")
	}
	if s.Consume("unknown") {
		t.Fatal("unknown accepted")
	}
}

func TestOAuthCallbackPersistsTokens(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("code") != "good-code" {
			http.Error(w, "bad code", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-1",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "refresh-1",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store := NewMemoryTokenStore()
	svc, err := NewService(Credentials{
		ClientID:     "cid",
		ClientSecret: "csecret",
		RedirectURL:  "http://localhost/api/youtube/callback",
		Endpoint: oauth2.Endpoint{
			AuthURL:  srv.URL + "/auth",
			TokenURL: srv.URL + "/token",
		},
	}, store)
	if err != nil {
		t.Fatal(err)
	}
	_, state, err := svc.BeginAuth()
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.HandleCallback(context.Background(), "good-code", state); err != nil {
		t.Fatal(err)
	}
	if !svc.Connected(context.Background()) {
		t.Fatal("not connected")
	}
	tok, err := store.LoadToken(context.Background(), providerGoogle)
	if err != nil || tok.AccessToken != "access-1" || tok.RefreshToken != "refresh-1" {
		t.Fatalf("%+v %v", tok, err)
	}
	if err := svc.HandleCallback(context.Background(), "good-code", state); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("replay state: %v", err)
	}
	if err := svc.Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if svc.Connected(context.Background()) {
		t.Fatal("still connected")
	}
}

func TestOAuthMissingCredentials(t *testing.T) {
	_, err := NewService(Credentials{}, NewMemoryTokenStore())
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
}
