// Package cryptoutil — шифрование чувствительных данных at rest (AES-256-GCM).
package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const prefix = "enc:v1:"

// Box шифрует и расшифровывает строки. Nil key — прозрачный режим (без шифрования).
type Box struct {
	gcm cipher.AEAD
}

// NewBox принимает 32-байтный ключ. Пустой ключ отключает шифрование.
func NewBox(key []byte) (*Box, error) {
	if len(key) == 0 {
		return &Box{}, nil
	}
	if len(key) != 32 {
		return nil, errors.New("ключ шифрования должен быть 32 байта")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{gcm: gcm}, nil
}

func (b *Box) Enabled() bool { return b != nil && b.gcm != nil }

// Seal шифрует plaintext. Пустая строка остаётся пустой.
func (b *Box) Seal(plaintext string) (string, error) {
	if plaintext == "" || !b.Enabled() {
		return plaintext, nil
	}
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := b.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.RawStdEncoding.EncodeToString(out), nil
}

// Open расшифровывает ciphertext. Поддерживает legacy plaintext без префикса.
func (b *Box) Open(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if !strings.HasPrefix(ciphertext, prefix) {
		return ciphertext, nil
	}
	if !b.Enabled() {
		return "", errors.New("зашифрованный токен, но ключ не задан")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(ciphertext, prefix))
	if err != nil {
		return "", fmt.Errorf("повреждённый ciphertext: %w", err)
	}
	ns := b.gcm.NonceSize()
	if len(raw) < ns {
		return "", errors.New("слишком короткий ciphertext")
	}
	plain, err := b.gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
