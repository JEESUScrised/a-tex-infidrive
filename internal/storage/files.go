package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// SanitizeName оставляет только базовое имя и убирает символы, опасные для путей.
func SanitizeName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		switch r {
		case '<', '>', ':', '"', '|', '?', '*', '/', '\\':
			return '_'
		default:
			return r
		}
	}, name)
	name = strings.Trim(name, ". ")
	if name == "" || name == "." || name == ".." {
		return "file.bin"
	}
	if len(name) > 180 {
		ext := filepath.Ext(name)
		if len(ext) > 20 {
			ext = ""
		}
		base := name
		if len(base) > 180-len(ext) {
			base = base[:180-len(ext)]
		}
		name = base + ext
	}
	return name
}

// DetectMIME определяет тип по расширению. Неизвестный тип — двоичный поток.
func DetectMIME(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return "application/octet-stream"
	}
	kind := mime.TypeByExtension(ext)
	if kind == "" {
		return "application/octet-stream"
	}
	if i := strings.Index(kind, ";"); i >= 0 {
		kind = strings.TrimSpace(kind[:i])
	}
	return kind
}

func writeLimited(path string, r io.Reader, maxBytes int64) (*os.File, string, int64, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, "", 0, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, maxBytes+1))
	if err != nil {
		return f, "", n, err
	}
	if n > maxBytes {
		return f, "", n, ErrTooLarge
	}
	if err := f.Sync(); err != nil {
		return f, "", n, err
	}
	return f, hex.EncodeToString(h.Sum(nil)), n, nil
}
