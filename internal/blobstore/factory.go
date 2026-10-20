package blobstore

import (
	"fmt"
	"strings"
)

// Config — выбор провайдера из окружения приложения.
type Config struct {
	Provider        string
	S3Endpoint      string
	S3Region        string
	S3Bucket        string
	S3AccessKeyID   string
	S3SecretAccessKey string
	S3ForcePathStyle bool
	S3Prefix        string
}

func NewFromConfig(cfg Config) (StorageProvider, error) {
	switch strings.ToLower(cfg.Provider) {
	case "", "memory":
		return NewMemory(), nil
	case "s3":
		return NewS3(S3Config{
			Endpoint:        cfg.S3Endpoint,
			Region:          cfg.S3Region,
			Bucket:          cfg.S3Bucket,
			AccessKeyID:     cfg.S3AccessKeyID,
			SecretAccessKey: cfg.S3SecretAccessKey,
			ForcePathStyle:  cfg.S3ForcePathStyle,
			Prefix:          cfg.S3Prefix,
		})
	default:
		return nil, fmt.Errorf("неизвестный STORAGE_PROVIDER: %s", cfg.Provider)
	}
}
