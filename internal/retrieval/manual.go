package retrieval

import (
	"context"
	"io"
)

// Manual — явный импорт MP4 пользователем/админом (например, файл из YouTube Studio).
// Не является автоматическим retrieval и не маскируется под успех SSYouTube.
type Manual struct {
	MaxBytes int64
}

func (Manual) Name() string { return "manual_mp4_import" }

func (m Manual) Available(context.Context) error { return nil }

// Retrieve не поддерживается без тела запроса — используйте WrapReader.
func (Manual) Retrieve(context.Context, string) (io.ReadCloser, error) {
	return nil, ErrUnavailable
}

// WrapReader ограничивает размер импортируемого MP4.
func (m Manual) WrapReader(r io.Reader) io.ReadCloser {
	max := m.MaxBytes
	if max <= 0 {
		max = 64 << 20
	}
	return &limitedReadCloser{ReadCloser: io.NopCloser(r), remain: max}
}
