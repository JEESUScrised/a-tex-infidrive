package blobstore

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"
)

// Memory — in-process провайдер для тестов и локальной разработки без S3.
// Не использовать как постоянное хранилище на VPS.
type Memory struct {
	mu   sync.RWMutex
	data map[string]memObj
}

type memObj struct {
	body        []byte
	contentType string
	mod         time.Time
}

func NewMemory() *Memory {
	return &Memory{data: map[string]memObj{}}
}

func (m *Memory) Name() string { return "memory" }

func (m *Memory) Available(context.Context) error { return nil }

func (m *Memory) Put(_ context.Context, key string, r io.Reader, opts PutOptions) (ObjectMeta, error) {
	if key == "" {
		return ObjectMeta{}, errorsInvalidKey
	}
	b, err := io.ReadAll(io.LimitReader(r, 512<<20)) // 512 MiB cap for memory provider
	if err != nil {
		return ObjectMeta{}, err
	}
	if opts.Size > 0 && int64(len(b)) > opts.Size {
		b = b[:opts.Size]
	}
	ct := opts.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	m.mu.Lock()
	m.data[key] = memObj{body: b, contentType: ct, mod: time.Now().UTC()}
	m.mu.Unlock()
	return ObjectMeta{Key: key, Size: int64(len(b)), ContentType: ct, LastModified: time.Now().UTC()}, nil
}

func (m *Memory) Get(_ context.Context, key string) (io.ReadCloser, ObjectMeta, error) {
	m.mu.RLock()
	obj, ok := m.data[key]
	m.mu.RUnlock()
	if !ok {
		return nil, ObjectMeta{}, ErrNotFound
	}
	meta := ObjectMeta{Key: key, Size: int64(len(obj.body)), ContentType: obj.contentType, LastModified: obj.mod}
	return io.NopCloser(bytes.NewReader(obj.body)), meta, nil
}

func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[key]; !ok {
		return ErrNotFound
	}
	delete(m.data, key)
	return nil
}

func (m *Memory) Head(_ context.Context, key string) (ObjectMeta, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	obj, ok := m.data[key]
	if !ok {
		return ObjectMeta{}, ErrNotFound
	}
	return ObjectMeta{Key: key, Size: int64(len(obj.body)), ContentType: obj.contentType, LastModified: obj.mod}, nil
}

func (m *Memory) PresignPut(context.Context, string, string, time.Duration) (string, error) {
	return "", ErrNotConfigured
}

func (m *Memory) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", ErrNotConfigured
}

// BytesForTest возвращает копию объекта (только тесты).
func (m *Memory) BytesForTest(key string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	o, ok := m.data[key]
	if !ok {
		return nil, false
	}
	out := make([]byte, len(o.body))
	copy(out, o.body)
	return out, true
}

var errorsInvalidKey = errInvalidKey{}

type errInvalidKey struct{}

func (errInvalidKey) Error() string { return "некорректный ключ объекта" }
