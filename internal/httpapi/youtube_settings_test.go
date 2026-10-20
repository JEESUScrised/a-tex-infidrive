package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ytdrive/internal/codec"
	"ytdrive/internal/config"
	"ytdrive/internal/jobs"
	"ytdrive/internal/storage"
	"ytdrive/internal/youtube"
)

func TestYouTubeStatusIncludesAdminFields(t *testing.T) {
	st, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	m := jobs.New(st, codec.DefaultOptions(), 1, 2, nil)
	m.Start()
	t.Cleanup(m.Stop)
	cfg := config.Config{
		AppName:            "A-Tex InfiDrive",
		AppEnv:             config.EnvDevelopment,
		WebDir:             t.TempDir(),
		MaxUpload:          1024,
		GoogleRedirectURL:  "https://app.a-tex-infidrive.ru/api/youtube/callback",
		YouTubePrivacy:     "public",
		ExperimentalYouTubeStore: true,
	}
	srv := httptest.NewServer(NewWithDrive(st, m, cfg, youtube.Unconfigured{}, nil, nil).Handler())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/youtube")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["enabled"] != false {
		t.Fatalf("enabled %#v", body["enabled"])
	}
	if body["redirect_uri"] != cfg.GoogleRedirectURL {
		t.Fatalf("redirect %#v", body["redirect_uri"])
	}
	if body["privacy_status"] != "public" {
		t.Fatalf("privacy %#v", body["privacy_status"])
	}
	if body["experimental_store"] != true {
		t.Fatalf("experimental %#v", body["experimental_store"])
	}
	if body["oauth_credentials_set"] != false {
		t.Fatalf("creds %#v", body["oauth_credentials_set"])
	}
	msg, _ := body["message"].(string)
	if msg == "" {
		t.Fatal("empty message")
	}
}

func TestYouTubeCallbackRedirectsToSettings(t *testing.T) {
	st, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	m := jobs.New(st, codec.DefaultOptions(), 1, 2, nil)
	cfg := config.Config{AppEnv: config.EnvDevelopment, WebDir: t.TempDir(), MaxUpload: 1024}
	s := NewWithDrive(st, m, cfg, youtube.Unconfigured{}, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/youtube/callback?code=x&state=y", nil)
	s.youtubeAuthCallback(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("code %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/settings?") {
		t.Fatalf("location %q", loc)
	}
}
