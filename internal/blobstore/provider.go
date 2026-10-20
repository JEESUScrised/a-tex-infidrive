// Package blobstore — абстракция внешнего объектного хранилища.
// Пользовательские байты не должны долговременно жить на диске VPS.
package blobstore

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrNotFound      = errors.New("объект не найден")
	ErrNotConfigured = errors.New("хранилище не настроено")
	ErrUnavailable   = errors.New("хранилище недоступно")
)

// ObjectMeta — метаданные объекта во внешнем хранилище.
type ObjectMeta struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

// PutOptions — параметры записи.
type PutOptions struct {
	ContentType string
	Size        int64 // 0 если неизвестен
}

// StorageProvider — независимый адаптер внешнего хранилища.
type StorageProvider interface {
	Name() string
	Available(ctx context.Context) error
	Put(ctx context.Context, key string, r io.Reader, opts PutOptions) (ObjectMeta, error)
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectMeta, error)
	Delete(ctx context.Context, key string) error
	Head(ctx context.Context, key string) (ObjectMeta, error)
	PresignPut(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error)
	PresignGet(ctx context.Context, key string, expiry time.Duration) (string, error)
}
