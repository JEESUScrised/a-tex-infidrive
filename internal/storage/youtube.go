package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"ytdrive/internal/youtube"
)

// YouTubeMeta — поля публикации файла на YouTube.
type YouTubeMeta struct {
	VideoID          string `json:"youtube_video_id"`
	URL              string `json:"youtube_url"`
	UploadStatus     string `json:"youtube_upload_status"`
	ProcessingStatus string `json:"youtube_processing_status"`
	UploadedAt       string `json:"youtube_uploaded_at"`
	Error            string `json:"youtube_error"`
	PrivacyStatus    string `json:"youtube_privacy_status"`
}

func (s *Store) migrateYouTube() error {
	cols := []string{
		"youtube_video_id TEXT NOT NULL DEFAULT ''",
		"youtube_url TEXT NOT NULL DEFAULT ''",
		"youtube_upload_status TEXT NOT NULL DEFAULT ''",
		"youtube_processing_status TEXT NOT NULL DEFAULT ''",
		"youtube_uploaded_at TEXT NOT NULL DEFAULT ''",
		"youtube_error TEXT NOT NULL DEFAULT ''",
		"youtube_privacy_status TEXT NOT NULL DEFAULT ''",
	}
	for _, def := range cols {
		_, err := s.db.Exec(`ALTER TABLE files ADD COLUMN ` + def)
		if err != nil && !isDupColumn(err) {
			return err
		}
	}
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS oauth_tokens (
    provider TEXT PRIMARY KEY,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    token_type TEXT NOT NULL,
    expiry TEXT NOT NULL,
    updated_at TEXT NOT NULL
);`)
	return err
}

func isDupColumn(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column")
}

// SetYouTube сохраняет идентификатор и статусы публикации.
func (s *Store) SetYouTube(fileID string, meta YouTubeMeta) error {
	if !ValidID(fileID) {
		return ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE files SET
		youtube_video_id = ?,
		youtube_url = ?,
		youtube_upload_status = ?,
		youtube_processing_status = ?,
		youtube_uploaded_at = ?,
		youtube_error = ?,
		youtube_privacy_status = ?
		WHERE id = ?`,
		meta.VideoID, meta.URL, meta.UploadStatus, meta.ProcessingStatus,
		meta.UploadedAt, meta.Error, meta.PrivacyStatus, fileID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// FailInterruptedJobs помечает задания, «зависшие» после аварийного останова.
func (s *Store) FailInterruptedJobs() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE jobs SET status = ?, error = ?, message = ?, updated_at = ?
		WHERE status IN (?, ?, ?, ?)`,
		StatusFailed, "прервано перезапуском сервера", "прервано", now(),
		StatusRunning, StatusUploading, StatusUploaded, StatusProcessing)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// LoadToken реализует youtube.TokenPersister.
func (s *Store) LoadToken(_ context.Context, provider string) (youtube.TokenRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.db.QueryRow(`SELECT access_token, refresh_token, token_type, expiry FROM oauth_tokens WHERE provider = ?`, provider)
	var rec youtube.TokenRecord
	var expiry string
	err := row.Scan(&rec.AccessToken, &rec.RefreshToken, &rec.TokenType, &expiry)
	if errors.Is(err, sql.ErrNoRows) {
		return youtube.TokenRecord{}, ErrNotFound
	}
	if err != nil {
		return youtube.TokenRecord{}, err
	}
	if expiry != "" {
		if t, e := time.Parse(time.RFC3339, expiry); e == nil {
			rec.Expiry = t
		}
	}
	return rec, nil
}

// SaveToken реализует youtube.TokenPersister. Значения токенов в логи не пишутся.
func (s *Store) SaveToken(_ context.Context, provider string, tok youtube.TokenRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry := ""
	if !tok.Expiry.IsZero() {
		expiry = tok.Expiry.UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(`INSERT INTO oauth_tokens (provider, access_token, refresh_token, token_type, expiry, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider) DO UPDATE SET
			access_token = excluded.access_token,
			refresh_token = CASE WHEN excluded.refresh_token = '' THEN oauth_tokens.refresh_token ELSE excluded.refresh_token END,
			token_type = excluded.token_type,
			expiry = excluded.expiry,
			updated_at = excluded.updated_at`,
		provider, tok.AccessToken, tok.RefreshToken, tok.TokenType, expiry, now())
	return err
}

// DeleteToken реализует youtube.TokenPersister.
func (s *Store) DeleteToken(_ context.Context, provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM oauth_tokens WHERE provider = ?`, provider)
	return err
}

var _ youtube.TokenPersister = (*Store)(nil)
