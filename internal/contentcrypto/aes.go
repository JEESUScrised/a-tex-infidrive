// Package contentcrypto — AES-256-GCM для полезной нагрузки до кодирования в видео.
// Не путать с шифрованием OAuth-токенов (cryptoutil).
package contentcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	magic   = "IDENC1" // A-Tex InfiDrive encrypted payload v1
	overhead = 6 + 12 + 16 // magic + nonce + GCM tag (approx via Seal)
)

// Box шифрует/расшифровывает байты файла.
type Box struct {
	gcm cipher.AEAD
}

func NewBox(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, errors.New("CONTENT_ENCRYPTION_KEY: нужно 32 байта")
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

// Seal шифрует plaintext. Формат: magic | nonce | ciphertext+tag.
func (b *Box) Seal(plain []byte) ([]byte, error) {
	return b.SealAAD(plain, nil)
}

// SealAAD — authenticated encryption с дополнительными данными (например file_id|chunk_index).
// Nonce уникален на каждый вызов (crypto/rand). AAD не шифруется, но привязан к тегу.
func (b *Box) SealAAD(plain, aad []byte) ([]byte, error) {
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(magic)+len(nonce)+len(plain)+b.gcm.Overhead())
	out = append(out, magic...)
	out = append(out, nonce...)
	out = append(out, b.gcm.Seal(nil, nonce, plain, aad)...)
	return out, nil
}

// Open расшифровывает Seal-пакет без AAD.
func (b *Box) Open(blob []byte) ([]byte, error) {
	return b.OpenAAD(blob, nil)
}

// OpenAAD расшифровывает SealAAD-пакет.
func (b *Box) OpenAAD(blob, aad []byte) ([]byte, error) {
	if len(blob) < len(magic)+b.gcm.NonceSize()+b.gcm.Overhead() {
		return nil, errors.New("слишком короткий ciphertext")
	}
	if string(blob[:len(magic)]) != magic {
		return nil, errors.New("неизвестный формат полезной нагрузки")
	}
	nonce := blob[len(magic) : len(magic)+b.gcm.NonceSize()]
	ct := blob[len(magic)+b.gcm.NonceSize():]
	plain, err := b.gcm.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, fmt.Errorf("расшифровка не удалась: %w", err)
	}
	return plain, nil
}

// ChunkAAD строит стабильный AAD для части файла.
// chunkTotal намеренно не входит: при потоковой загрузке без Content-Length
// число частей заранее неизвестно; индекс + длина достаточны для привязки.
func ChunkAAD(fileID string, chunkIndex, _chunkTotal int, plainLen int64) []byte {
	var idx [4]byte
	binary.BigEndian.PutUint32(idx[:], uint32(chunkIndex))
	var ln [8]byte
	binary.BigEndian.PutUint64(ln[:], uint64(plainLen))
	out := make([]byte, 0, len(fileID)+4+8)
	out = append(out, fileID...)
	out = append(out, idx[:]...)
	out = append(out, ln[:]...)
	return out
}

// WriteLengthPrefixed — вспомогательная запись размера (для тестов).
func WriteLengthPrefixed(w io.Writer, b []byte) error {
	var hdr [8]byte
	binary.BigEndian.PutUint64(hdr[:], uint64(len(b)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}
