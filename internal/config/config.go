// Package config читает настройки из окружения и необязательного файла .env.
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"ytdrive/internal/codec"
	"ytdrive/internal/ffmpeg"
)

// Config — параметры сервера. Секретов здесь нет: YouTube OAuth в этой версии не используется.
type Config struct {
	Addr         string
	DataDir      string
	WebDir       string
	MaxUpload    int64
	Workers      int
	QueueSize    int
	FFmpeg       string
	FFProbe      string
	FFmpegSource string
	Encode       codec.Options
}

// Load собирает конфигурацию. Уже заданные переменные окружения важнее .env.
func Load() Config {
	loadDotEnv(".env")
	ffHint := firstEnv("FFMPEG_PATH", "YTDRIVE_FFMPEG")
	fpHint := firstEnv("FFPROBE_PATH", "YTDRIVE_FFPROBE")
	ff, fp, src := ffmpeg.Resolve(ffHint, fpHint)

	cfg := Config{
		Addr:         env("YTDRIVE_ADDR", ":8080"),
		DataDir:      env("YTDRIVE_DATA_DIR", "data"),
		WebDir:       env("YTDRIVE_WEB_DIR", ""),
		MaxUpload:    int64(envInt("YTDRIVE_MAX_UPLOAD_MB", 64)) << 20,
		Workers:      envInt("YTDRIVE_WORKERS", 1),
		QueueSize:    envInt("YTDRIVE_QUEUE", 16),
		FFmpeg:       ff,
		FFProbe:      fp,
		FFmpegSource: src,
	}
	cfg.Encode = codec.DefaultOptions()
	cfg.Encode.FFmpeg = cfg.FFmpeg
	cfg.Encode.FFProbe = cfg.FFProbe
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
	if cfg.Workers < 1 {
		cfg.Workers = 1
	}
	if cfg.QueueSize < 1 {
		cfg.QueueSize = 1
	}
	if cfg.MaxUpload < 1 {
		cfg.MaxUpload = 1 << 20
	}
	return cfg
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
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
