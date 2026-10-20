// Package config читает настройки из окружения и необязательного файла .env.
package config

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"ytdrive/internal/codec"
	"ytdrive/internal/ffmpeg"
)

const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

// Config — параметры сервера.
// Секреты читаются только из окружения / .env и не коммитятся.
type Config struct {
	AppEnv   string
	AppName  string
	Addr     string
	PublicURL string

	DataDir      string
	DatabasePath string
	StoragePath  string
	TempPath     string
	WebDir       string
	SiteDir      string

	MaxUpload int64
	Workers   int
	QueueSize int

	FFmpeg       string
	FFProbe      string
	FFmpegSource string
	Encode       codec.Options

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	AdminUsername     string
	AdminPasswordHash string
	SessionSecret     string
	TokenEncryptionKey []byte

	// AuthEnabled — true, если заданы учётные данные администратора.
	AuthEnabled bool
	// CookieSecure — принудительный Secure для cookies (или авто по PublicURL/HTTPS).
	CookieSecure bool
}

// Load собирает конфигурацию. Уже заданные переменные окружения важнее .env.
// Сохранены алиасы YTDRIVE_* для обратной совместимости.
func Load() (Config, error) {
	loadDotEnv(".env")
	ffHint := firstEnv("FFMPEG_PATH", "YTDRIVE_FFMPEG")
	fpHint := firstEnv("FFPROBE_PATH", "YTDRIVE_FFPROBE")
	ff, fp, src := ffmpeg.Resolve(ffHint, fpHint)

	host := firstEnv("SERVER_HOST", "")
	port := firstEnv("SERVER_PORT", "")
	addr := firstEnv("YTDRIVE_ADDR", "")
	if addr == "" {
		if host == "" {
			host = "127.0.0.1"
		}
		if port == "" {
			port = "8092"
		}
		addr = host + ":" + port
	}

	dataDir := firstEnv("STORAGE_PATH", "YTDRIVE_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}

	maxMB := firstEnvInt([]string{"UPLOAD_MAX_MB", "YTDRIVE_MAX_UPLOAD_MB"}, 64)
	workers := firstEnvInt([]string{"MAX_CONCURRENT_JOBS", "YTDRIVE_WORKERS"}, 1)

	encKey, err := parseEncryptionKey(firstEnv("TOKEN_ENCRYPTION_KEY", ""))
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppEnv:             strings.ToLower(env("APP_ENV", EnvDevelopment)),
		AppName:            env("APP_NAME", "A-Tex InfiDrive"),
		Addr:               addr,
		PublicURL:          strings.TrimRight(firstEnv("PUBLIC_URL", ""), "/"),
		DataDir:            dataDir,
		DatabasePath:       firstEnv("DATABASE_PATH", ""),
		StoragePath:        dataDir,
		TempPath:           firstEnv("TEMP_PATH", ""),
		WebDir:             firstEnv("YTDRIVE_WEB_DIR", ""),
		SiteDir:            firstEnv("SITE_DIR", "site"),
		MaxUpload:          int64(maxMB) << 20,
		Workers:            workers,
		QueueSize:          firstEnvInt([]string{"YTDRIVE_QUEUE"}, 16),
		FFmpeg:             ff,
		FFProbe:            fp,
		FFmpegSource:       src,
		GoogleClientID:     firstEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: firstEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:  firstEnv("GOOGLE_REDIRECT_URL", "http://localhost:8092/api/youtube/callback"),
		AdminUsername:      firstEnv("ADMIN_USERNAME", ""),
		AdminPasswordHash:  firstEnv("ADMIN_PASSWORD_HASH", ""),
		SessionSecret:      firstEnv("SESSION_SECRET", ""),
		TokenEncryptionKey: encKey,
	}

	// Опционально: хеш из plaintext только вне production (удобство разработки).
	if cfg.AdminPasswordHash == "" {
		if pw := firstEnv("ADMIN_PASSWORD", ""); pw != "" {
			if cfg.AppEnv == EnvProduction {
				return Config{}, errors.New("в production используйте ADMIN_PASSWORD_HASH, а не ADMIN_PASSWORD")
			}
			hash, err := hashPasswordDev(pw)
			if err != nil {
				return Config{}, err
			}
			cfg.AdminPasswordHash = hash
		}
	}

	cfg.AuthEnabled = cfg.AdminUsername != "" && cfg.AdminPasswordHash != ""
	cfg.CookieSecure = cfg.AppEnv == EnvProduction || strings.HasPrefix(cfg.PublicURL, "https://")

	cfg.Encode = codec.DefaultOptions()
	cfg.Encode.FFmpeg = cfg.FFmpeg
	cfg.Encode.FFProbe = cfg.FFProbe
	applyEncodeEnv(&cfg)

	if cfg.Workers < 1 {
		cfg.Workers = 1
	}
	if cfg.QueueSize < 1 {
		cfg.QueueSize = 1
	}
	if cfg.MaxUpload < 1 {
		cfg.MaxUpload = 1 << 20
	}
	if cfg.TempPath == "" {
		cfg.TempPath = joinUnder(cfg.StoragePath, "tmp")
	}
	if cfg.DatabasePath == "" {
		cfg.DatabasePath = joinUnder(cfg.StoragePath, "ytdrive.db")
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate проверяет production-ограничения и целостность секретов.
func (c Config) Validate() error {
	if c.AppEnv != EnvProduction {
		return nil
	}
	var errs []string
	if !c.AuthEnabled {
		errs = append(errs, "нужны ADMIN_USERNAME и ADMIN_PASSWORD_HASH")
	}
	if len(c.SessionSecret) < 32 {
		errs = append(errs, "SESSION_SECRET должен быть не короче 32 символов")
	}
	if isWeakSecret(c.SessionSecret) {
		errs = append(errs, "SESSION_SECRET слишком слабый или тестовый")
	}
	if len(c.TokenEncryptionKey) != 32 {
		errs = append(errs, "TOKEN_ENCRYPTION_KEY должен декодироваться в 32 байта")
	}
	if c.PublicURL == "" || !strings.HasPrefix(c.PublicURL, "https://") {
		errs = append(errs, "PUBLIC_URL должен быть https://…")
	}
	if c.GoogleRedirectURL != "" && strings.HasPrefix(c.GoogleRedirectURL, "http://") && !strings.Contains(c.GoogleRedirectURL, "localhost") {
		errs = append(errs, "GOOGLE_REDIRECT_URL в production должен использовать HTTPS")
	}
	if len(errs) > 0 {
		return fmt.Errorf("некорректная production-конфигурация: %s", strings.Join(errs, "; "))
	}
	return nil
}

func applyEncodeEnv(cfg *Config) {
	if v := os.Getenv("YTDRIVE_FPS"); v != "" {
		cfg.Encode.FPS = envInt("YTDRIVE_FPS", cfg.Encode.FPS)
	}
	if v := os.Getenv("YTDRIVE_CRF"); v != "" {
		cfg.Encode.CRF = envInt("YTDRIVE_CRF", cfg.Encode.CRF)
	}
	if v := os.Getenv("YTDRIVE_GOP"); v != "" {
		cfg.Encode.GOP = envInt("YTDRIVE_GOP", cfg.Encode.GOP)
	}
	if v := os.Getenv("YTDRIVE_PRESET"); v != "" {
		cfg.Encode.Preset = v
	}
	if v := os.Getenv("YTDRIVE_FRAME_REPEAT"); v != "" {
		cfg.Encode.FrameRepeat = envInt("YTDRIVE_FRAME_REPEAT", cfg.Encode.FrameRepeat)
	}
	if v := os.Getenv("YTDRIVE_PACKET_REPEAT"); v != "" {
		cfg.Encode.PacketRepeat = envInt("YTDRIVE_PACKET_REPEAT", cfg.Encode.PacketRepeat)
	}
	if v := os.Getenv("YTDRIVE_PARITY_GROUP"); v != "" {
		cfg.Encode.ParityGroup = envInt("YTDRIVE_PARITY_GROUP", cfg.Encode.ParityGroup)
	}
	if v := os.Getenv("YTDRIVE_GZIP"); v == "1" || strings.EqualFold(v, "true") {
		cfg.Encode.Gzip = true
	}
}

func parseEncryptionKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if key, err := hex.DecodeString(raw); err == nil && len(key) == 32 {
		return key, nil
	}
	if key, err := base64.StdEncoding.DecodeString(raw); err == nil && len(key) == 32 {
		return key, nil
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	return nil, errors.New("TOKEN_ENCRYPTION_KEY: ожидается 32 байта (hex, base64 или raw)")
}

func isWeakSecret(s string) bool {
	low := strings.ToLower(s)
	for _, bad := range []string{"changeme", "secret", "password", "test", "example", "123456", "infidrive"} {
		if strings.Contains(low, bad) && len(s) < 48 {
			return true
		}
	}
	return false
}

func joinUnder(root, name string) string {
	if root == "" {
		return name
	}
	return strings.TrimRight(root, `/\`) + string(os.PathSeparator) + name
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if key == "" {
			continue
		}
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func firstEnvInt(keys []string, fallback int) int {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			n, err := strconv.Atoi(v)
			if err == nil {
				return n
			}
		}
	}
	return fallback
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, val)
	}
}
