// Package youtube — официальная интеграция с YouTube Data API v3 и OAuth 2.0.
// Неофициальное скачивание роликов и обход ограничений не реализуются.
package youtube

import (
	"context"
	"errors"
)

var (
	// ErrNotConfigured — нет OAuth-клиентских данных или провайдер не создан.
	ErrNotConfigured = errors.New("загрузка на YouTube не настроена")
	// ErrNotConnected — пользователь не авторизовал аккаунт.
	ErrNotConnected = errors.New("YouTube-аккаунт не подключён")
	// ErrAlreadyUploaded — у файла уже есть video_id, повторная публикация запрещена.
	ErrAlreadyUploaded = errors.New("видео этого файла уже загружено на YouTube")
	// ErrAuth — ошибка авторизации или истёкший refresh token.
	ErrAuth = errors.New("ошибка авторизации YouTube")
	// ErrQuota — исчерпана или ограничена квота API.
	ErrQuota = errors.New("квота YouTube API исчерпана или ограничена")
	// ErrTemporary — временная сетевая или серверная ошибка.
	ErrTemporary = errors.New("временная ошибка YouTube API")
	// ErrInvalidState — OAuth state не совпал (защита от CSRF).
	ErrInvalidState = errors.New("некорректный OAuth state")
)

// VideoRef — результат загрузки через YouTube Data API v3.
type VideoRef struct {
	VideoID          string
	URL              string
	UploadStatus     string
	ProcessingStatus string
	PrivacyStatus    string
	ChannelID        string
	ChannelTitle     string
}

// ChannelInfo — подключённый канал пользователя.
type ChannelInfo struct {
	ID    string
	Title string
}

// ProgressFunc сообщает прогресс загрузки байт (не путать с кодированием кадров).
type ProgressFunc func(sent, total int64)

// UploadOptions — параметры публикации.
type UploadOptions struct {
	Title       string
	Description string
	Privacy     string // по умолчанию private
	Progress    ProgressFunc
}

// VideoStorageProvider — граница интеграции с YouTube.
type VideoStorageProvider interface {
	Configured() bool
	Connected(ctx context.Context) bool
	AuthURL(state string) (string, error)
	HandleCallback(ctx context.Context, code, state string) error
	GetConnectedChannel(ctx context.Context) (ChannelInfo, error)
	Disconnect(ctx context.Context) error
	UploadVideo(ctx context.Context, filePath string, opt UploadOptions) (VideoRef, error)
	GetVideoStatus(ctx context.Context, videoID string) (VideoRef, error)

	// Совместимость с ранним интерфейсом.
	Upload(ctx context.Context, filePath, title, description string) (VideoRef, error)
	ProcessingStatus(ctx context.Context, videoID string) (string, error)
}

// Unconfigured — честная заглушка без клиентских секретов.
type Unconfigured struct{}

func (Unconfigured) Configured() bool { return false }

func (Unconfigured) Connected(context.Context) bool { return false }

func (Unconfigured) AuthURL(string) (string, error) { return "", ErrNotConfigured }

func (Unconfigured) HandleCallback(context.Context, string, string) error {
	return ErrNotConfigured
}

func (Unconfigured) GetConnectedChannel(context.Context) (ChannelInfo, error) {
	return ChannelInfo{}, ErrNotConfigured
}

func (Unconfigured) Disconnect(context.Context) error { return ErrNotConfigured }

func (Unconfigured) UploadVideo(context.Context, string, UploadOptions) (VideoRef, error) {
	return VideoRef{}, ErrNotConfigured
}

func (Unconfigured) GetVideoStatus(context.Context, string) (VideoRef, error) {
	return VideoRef{}, ErrNotConfigured
}

func (Unconfigured) Upload(context.Context, string, string, string) (VideoRef, error) {
	return VideoRef{}, ErrNotConfigured
}

func (Unconfigured) ProcessingStatus(context.Context, string) (string, error) {
	return "", ErrNotConfigured
}
