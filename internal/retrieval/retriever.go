// Package retrieval — экспериментальное получение MP4 по video_id.
// Неофициальный обход ограничений YouTube и CAPTCHA не реализуются.
package retrieval

import (
	"context"
	"errors"
	"io"
)

var (
	// ErrUnavailable — автоматическое получение невозможно или не настроено.
	ErrUnavailable = errors.New("автоматическое получение видео недоступно")
	// ErrForbidden — источник запрещён политикой безопасности (SSRF и т.п.).
	ErrForbidden = errors.New("источник видео запрещён")
	// ErrTooLarge — ответ превышает лимит.
	ErrTooLarge = errors.New("видео слишком большое")
	// ErrBadContent — неожиданный Content-Type или пустой ответ.
	ErrBadContent = errors.New("некорректный видеопоток")
)

// VideoRetriever получает MP4 по идентификатору YouTube-ролика.
type VideoRetriever interface {
	Name() string
	Available(ctx context.Context) error
	Retrieve(ctx context.Context, videoID string) (io.ReadCloser, error)
}

// Meta — диагностическая информация об адаптере.
type Meta struct {
	Name        string `json:"name"`
	Available   bool   `json:"available"`
	Message     string `json:"message"`
	Automatic   bool   `json:"automatic"`
	ManualFallback bool `json:"manual_fallback"`
}
