package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"ytdrive/internal/auth"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) authCSRF(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSON(w, http.StatusOK, map[string]string{"csrf_token": ""})
		return
	}
	tok, err := s.sessions.IssueCSRF(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось выдать CSRF")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"csrf_token": tok})
}

func (s *Server) authMe(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AuthEnabled {
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated": true,
			"auth_required": false,
			"username":      "local",
			"app_name":      s.cfg.AppName,
		})
		return
	}
	user, err := s.sessions.User(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated": false,
			"auth_required": true,
			"app_name":      s.cfg.AppName,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"auth_required": true,
		"username":      user,
		"app_name":      s.cfg.AppName,
	})
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AuthEnabled {
		writeError(w, http.StatusBadRequest, "авторизация отключена")
		return
	}
	if !s.limiter.Allow(r) {
		writeError(w, http.StatusTooManyRequests, "слишком много попыток входа")
		return
	}
	if !auth.CheckCSRF(r) {
		writeError(w, http.StatusForbidden, "недействительный CSRF-токен")
		return
	}
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username != s.cfg.AdminUsername {
		writeError(w, http.StatusUnauthorized, auth.ErrInvalidCredentials.Error())
		return
	}
	if err := auth.CheckPassword(s.cfg.AdminPasswordHash, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if err := s.sessions.Create(w, req.Username); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать сессию")
		return
	}
	if _, err := s.sessions.IssueCSRF(w); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось обновить CSRF")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"username":      req.Username,
		"app_name":      s.cfg.AppName,
	})
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	if s.sessions != nil {
		if !auth.CheckCSRF(r) {
			writeError(w, http.StatusForbidden, "недействительный CSRF-токен")
			return
		}
		s.sessions.Revoke(r)
		s.sessions.Clear(w)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if !s.cfg.AuthEnabled {
		return true
	}
	if _, err := s.sessions.User(r); err != nil {
		writeError(w, http.StatusUnauthorized, auth.ErrUnauthorized.Error())
		return false
	}
	if !auth.CheckCSRF(r) {
		writeError(w, http.StatusForbidden, "недействительный CSRF-токен")
		return false
	}
	return true
}

// requireAuthSession — для GET (без CSRF), например OAuth callback.
func (s *Server) requireAuthSession(w http.ResponseWriter, r *http.Request) bool {
	if !s.cfg.AuthEnabled {
		return true
	}
	if _, err := s.sessions.User(r); err != nil {
		writeError(w, http.StatusUnauthorized, auth.ErrUnauthorized.Error())
		return false
	}
	return true
}
