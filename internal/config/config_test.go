package config

import (
	"encoding/hex"
	"testing"
)

func TestProductionRejectsWeakSecrets(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD_HASH", "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUV")
	t.Setenv("SESSION_SECRET", "changeme-not-good-enough-secret!!")
	t.Setenv("TOKEN_ENCRYPTION_KEY", hex.EncodeToString(make([]byte, 32)))
	t.Setenv("PUBLIC_URL", "https://app.a-tex-infidrive.ru")
	_, err := Load()
	if err == nil {
		t.Fatal("expected weak session secret error")
	}
}

func TestProductionRequiresAuth(t *testing.T) {
	clearProdEnvs(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("SESSION_SECRET", "this-is-a-sufficiently-long-random-session-secret-value-ok")
	t.Setenv("TOKEN_ENCRYPTION_KEY", hex.EncodeToString(make([]byte, 32)))
	t.Setenv("PUBLIC_URL", "https://app.a-tex-infidrive.ru")
	_, err := Load()
	if err == nil {
		t.Fatal("expected missing admin error")
	}
}

func TestDevelopmentKeepsYTDRIVEAliases(t *testing.T) {
	clearProdEnvs(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("YTDRIVE_ADDR", "127.0.0.1:9090")
	t.Setenv("YTDRIVE_DATA_DIR", "data-test")
	t.Setenv("YTDRIVE_MAX_UPLOAD_MB", "8")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:9090" {
		t.Fatalf("addr %s", cfg.Addr)
	}
	if cfg.StoragePath != "data-test" {
		t.Fatalf("storage %s", cfg.StoragePath)
	}
	if cfg.MaxUpload != 8<<20 {
		t.Fatalf("max %d", cfg.MaxUpload)
	}
	if cfg.AppName != "A-Tex InfiDrive" {
		t.Fatalf("name %s", cfg.AppName)
	}
}

func clearProdEnvs(t *testing.T) {
	t.Helper()
	keys := []string{
		"APP_ENV", "APP_NAME", "SERVER_HOST", "SERVER_PORT", "PUBLIC_URL",
		"YTDRIVE_ADDR", "YTDRIVE_DATA_DIR", "STORAGE_PATH", "DATABASE_PATH", "TEMP_PATH",
		"UPLOAD_MAX_MB", "YTDRIVE_MAX_UPLOAD_MB", "MAX_CONCURRENT_JOBS", "YTDRIVE_WORKERS",
		"ADMIN_USERNAME", "ADMIN_PASSWORD", "ADMIN_PASSWORD_HASH",
		"SESSION_SECRET", "TOKEN_ENCRYPTION_KEY",
		"GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_SECRET", "GOOGLE_REDIRECT_URL",
	}
	for _, k := range keys {
		// Пустое значение блокирует подстановку из локального .env в тестах.
		t.Setenv(k, "")
	}
}
