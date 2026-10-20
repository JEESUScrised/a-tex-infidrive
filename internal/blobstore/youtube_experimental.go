package blobstore

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrExperimentalIncomplete — обратное получение данных через YouTube не реализовано
// разрешённым способом (нет официального download исходного файла).
var ErrExperimentalIncomplete = errors.New("youtube experimental provider: download/restore не готов для production")

// YouTubeExperimental — заглушка StorageProvider.
// Загрузка/кодирование остаются в legacy youtube.Service + codec; этот адаптер
// явно не имитирует восстановление.
type YouTubeExperimental struct{}

func (YouTubeExperimental) Name() string { return "youtube_experimental" }

func (YouTubeExperimental) Available(context.Context) error {
	return ErrExperimentalIncomplete
}

func (YouTubeExperimental) Put(context.Context, string, io.Reader, PutOptions) (ObjectMeta, error) {
	return ObjectMeta{}, ErrExperimentalIncomplete
}

func (YouTubeExperimental) Get(context.Context, string) (io.ReadCloser, ObjectMeta, error) {
	return nil, ObjectMeta{}, ErrExperimentalIncomplete
}

func (YouTubeExperimental) Delete(context.Context, string) error {
	return ErrExperimentalIncomplete
}

func (YouTubeExperimental) Head(context.Context, string) (ObjectMeta, error) {
	return ObjectMeta{}, ErrExperimentalIncomplete
}

func (YouTubeExperimental) PresignPut(context.Context, string, string, time.Duration) (string, error) {
	return "", ErrExperimentalIncomplete
}

func (YouTubeExperimental) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", ErrExperimentalIncomplete
}
