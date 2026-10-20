package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ytdrive/internal/auth"
	"ytdrive/internal/codec"
	"ytdrive/internal/config"
	"ytdrive/internal/jobs"
	"ytdrive/internal/storage"
)

func TestAuthProtectsPrivateAPI(t *testing.T) {
	st, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	hash, err := auth.HashPassword("test-password-ok")
	if err != nil {
		t.Fatal(err)
	}
	m := jobs.New(st, codec.DefaultOptions(), 1, 2, nil)
	m.Start()
	t.Cleanup(m.Stop)
	cfg := config.Config{
		AppName:           "A-Tex InfiDrive",
		AppEnv:            config.EnvDevelopment,
		MaxUpload:         1024,
		FFmpeg:            "ffmpeg",
		FFProbe:           "ffprobe",
		WebDir:            t.TempDir(),
		AdminUsername:     "admin",
		AdminPasswordHash: hash,
		SessionSecret:     "session-secret-at-least-32-chars-long!!",
		AuthEnabled:       true,
		CookieSecure:      false,
	}
	srv := httptest.NewServer(New(st, m, cfg, nil).Handler())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/files")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("files without auth: %d", res.StatusCode)
	}

	health, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer health.Body.Close()
	if health.StatusCode != http.StatusOK {
		t.Fatalf("health %d", health.StatusCode)
	}
	var h map[string]any
	if err := json.NewDecoder(health.Body).Decode(&h); err != nil {
		t.Fatal(err)
	}
	if h["status"] != "ok" {
		t.Fatalf("%#v", h)
	}

	csrfRes, err := http.Get(srv.URL + "/api/auth/csrf")
	if err != nil {
		t.Fatal(err)
	}
	defer csrfRes.Body.Close()
	var csrfBody map[string]string
	if err := json.NewDecoder(csrfRes.Body).Decode(&csrfBody); err != nil {
		t.Fatal(err)
	}
	csrf := csrfBody["csrf_token"]
	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "test-password-ok"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.CSRFHeader, csrf)
	for _, c := range csrfRes.Cookies() {
		req.AddCookie(c)
	}
	loginRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login %d", loginRes.StatusCode)
	}

	listReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/files", nil)
	for _, c := range loginRes.Cookies() {
		listReq.AddCookie(c)
	}
	// GET не требует CSRF.
	listRes, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatal(err)
	}
	defer listRes.Body.Close()
	if listRes.StatusCode != http.StatusOK {
		t.Fatalf("list %d", listRes.StatusCode)
	}
}
