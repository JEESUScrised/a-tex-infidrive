// Package youtube описывает будущую официальную загрузку на YouTube.
// Неофициальное скачивание роликов и обход ограничений здесь не реализуются.
package youtube

import (
	"context"
	"errors"
)

// ErrNotConfigured означает, что OAuth и ключи API не заданы.
// Вызов нельзя выдавать за успешную публикацию.
var ErrNotConfigured = errors.New("загрузка на YouTube не настроена")

// VideoRef — результат будущей загрузки через YouTube Data API v3.
type VideoRef struct {
	VideoID string
	URL     string
	Status  string
}

// VideoStorageProvider — граница интеграции.
// Планируемые шаги: OAuth 2.0, загрузка MP4, сохранение video_id и URL,
// опрос статуса обработки, разбор ошибок квоты и авторизации.
type VideoStorageProvider interface {
	Upload(ctx context.Context, filePath, title, description string) (VideoRef, error)
	ProcessingStatus(ctx context.Context, videoID string) (string, error)
}

// Unconfigured — честная заглушка первой итерации: она всегда возвращает ошибку.
type Unconfigured struct{}

func (Unconfigured) Upload(context.Context, string, string, string) (VideoRef, error) {
	return VideoRef{}, ErrNotConfigured
}

func (Unconfigured) ProcessingStatus(context.Context, string) (string, error) {
	return "", ErrNotConfigured
}
