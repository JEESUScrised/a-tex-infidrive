package drive

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"ytdrive/internal/storage"
)

var (
	ErrNotFound = errors.New("не найдено")
	ErrConflict = errors.New("конфликт")
	ErrDenied   = errors.New("доступ запрещён")
	ErrInvalid  = errors.New("некорректный запрос")
)

// Store — метаданные диска в SQLite (без пользовательских байтов).
type Store struct {
	db *sql.DB
	mu sync.Mutex
}

func Open(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS drive_folders (
    id TEXT PRIMARY KEY,
    parent_id TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS drive_files (
    id TEXT PRIMARY KEY,
    folder_id TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    mime TEXT NOT NULL,
    size INTEGER NOT NULL DEFAULT 0,
    sha256 TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL,
    object_key TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS share_links (
    id TEXT PRIMARY KEY,
    file_id TEXT NOT NULL,
    token TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL DEFAULT '',
    max_downloads INTEGER NOT NULL DEFAULT 0,
    download_count INTEGER NOT NULL DEFAULT 0,
    revoked INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_drive_files_folder ON drive_files(folder_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_share_token ON share_links(token);
`)
	if err != nil {
		return err
	}
	for _, def := range []string{
		"youtube_video_id TEXT NOT NULL DEFAULT ''",
		"youtube_upload_status TEXT NOT NULL DEFAULT ''",
		"youtube_processing_status TEXT NOT NULL DEFAULT ''",
		"youtube_privacy_status TEXT NOT NULL DEFAULT ''",
		"plain_sha256 TEXT NOT NULL DEFAULT ''",
		"chunk_total INTEGER NOT NULL DEFAULT 1",
		"codec_version INTEGER NOT NULL DEFAULT 1",
		"manifest_complete INTEGER NOT NULL DEFAULT 0",
		"recovery_status TEXT NOT NULL DEFAULT 'pending'",
	} {
		_, err := s.db.Exec(`ALTER TABLE drive_files ADD COLUMN ` + def)
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return err
		}
	}
	return s.migrateChunks()
}

func (s *Store) SetYouTubeMeta(id, videoID, upload, processing, privacy, plainSHA string) error {
	if !storage.ValidID(id) {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE drive_files SET youtube_video_id = ?, youtube_upload_status = ?, youtube_processing_status = ?,
		youtube_privacy_status = ?, plain_sha256 = ?, updated_at = ? WHERE id = ?`,
		videoID, upload, processing, privacy, plainSHA, now(), id)
	return err
}

func (s *Store) SetStatus(id, status string) error {
	if !storage.ValidID(id) {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE drive_files SET status = ?, updated_at = ? WHERE id = ?`, status, now(), id)
	return err
}

// SetRecoveryStatus: pending | available | verified | failed.
// ready в status означает «загружен и доступен для попытки восстановления», не «SHA проверен».
func (s *Store) SetRecoveryStatus(id, recovery string) error {
	if !storage.ValidID(id) {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE drive_files SET recovery_status = ?, updated_at = ? WHERE id = ?`, recovery, now(), id)
	return err
}

func (s *Store) VideoID(id string) (string, error) {
	if !storage.ValidID(id) {
		return "", ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var v string
	err := s.db.QueryRow(`SELECT youtube_video_id FROM drive_files WHERE id = ?`, id).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}

func (s *Store) PlainSHA(id string) (string, error) {
	if !storage.ValidID(id) {
		return "", ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var v string
	err := s.db.QueryRow(`SELECT COALESCE(NULLIF(plain_sha256,''), sha256) FROM drive_files WHERE id = ?`, id).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}

func (s *Store) CreateFolder(parentID, name string) (Folder, error) {
	name = storage.SanitizeName(name)
	if name == "" || name == "file.bin" {
		return Folder{}, ErrInvalid
	}
	if parentID != "" && !storage.ValidID(parentID) {
		return Folder{}, ErrInvalid
	}
	id, err := newID()
	if err != nil {
		return Folder{}, err
	}
	f := Folder{ID: id, ParentID: parentID, Name: name, CreatedAt: now()}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(`INSERT INTO drive_folders (id, parent_id, name, created_at) VALUES (?, ?, ?, ?)`,
		f.ID, f.ParentID, f.Name, f.CreatedAt)
	return f, err
}

func (s *Store) ListFolders(parentID string) ([]Folder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, parent_id, name, created_at FROM drive_folders WHERE parent_id = ? ORDER BY name`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Folder
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.ID, &f.ParentID, &f.Name, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if out == nil {
		out = []Folder{}
	}
	return out, rows.Err()
}

func (s *Store) CreateFileMeta(folderID, name, mime, provider, objectKey, status string) (File, error) {
	if folderID != "" && !storage.ValidID(folderID) {
		return File{}, ErrInvalid
	}
	name = storage.SanitizeName(name)
	id, err := newID()
	if err != nil {
		return File{}, err
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	ts := now()
	f := File{
		ID: id, FolderID: folderID, Name: name, MIME: mime,
		Provider: provider, ObjectKey: objectKey, Status: status,
		CreatedAt: ts, UpdatedAt: ts,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(`INSERT INTO drive_files
		(id, folder_id, name, mime, size, sha256, provider, object_key, status, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, 0, '', ?, ?, ?, ?, ?, '')`,
		f.ID, f.FolderID, f.Name, f.MIME, f.Provider, f.ObjectKey, f.Status, f.CreatedAt, f.UpdatedAt)
	return f, err
}

func (s *Store) SetObjectKey(id, key string) error {
	if !storage.ValidID(id) || key == "" {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE drive_files SET object_key = ?, updated_at = ? WHERE id = ?`, key, now(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) FinalizeFile(id string, size int64, sha256 string) error {
	if !storage.ValidID(id) {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE drive_files SET size = ?, sha256 = ?, status = 'ready', recovery_status = 'available', updated_at = ? WHERE id = ? AND deleted_at = ''`,
		size, sha256, now(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) FailFile(id, message string) error {
	if !storage.ValidID(id) {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE drive_files SET status = 'error', updated_at = ? WHERE id = ?`, now(), id)
	_ = message
	return err
}

func (s *Store) GetFile(id string) (File, error) {
	if !storage.ValidID(id) {
		return File{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.db.QueryRow(`SELECT id, folder_id, name, mime, size, sha256, provider, object_key, status, created_at, updated_at, deleted_at,
		COALESCE(recovery_status,'pending') FROM drive_files WHERE id = ?`, id)
	return scanFile(row)
}

func (s *Store) ListFiles(folderID string, trash bool) ([]File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var rows *sql.Rows
	var err error
	if trash {
		rows, err = s.db.Query(`SELECT id, folder_id, name, mime, size, sha256, provider, object_key, status, created_at, updated_at, deleted_at,
			COALESCE(recovery_status,'pending') FROM drive_files WHERE deleted_at != '' ORDER BY deleted_at DESC LIMIT 500`)
	} else {
		rows, err = s.db.Query(`SELECT id, folder_id, name, mime, size, sha256, provider, object_key, status, created_at, updated_at, deleted_at,
			COALESCE(recovery_status,'pending') FROM drive_files WHERE folder_id = ? AND deleted_at = '' AND status = 'ready' ORDER BY name LIMIT 500`, folderID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if out == nil {
		out = []File{}
	}
	return out, rows.Err()
}

func (s *Store) SoftDelete(id string) error {
	if !storage.ValidID(id) {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE drive_files SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, now(), now(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Restore(id string) error {
	if !storage.ValidID(id) {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE drive_files SET deleted_at = '', updated_at = ? WHERE id = ? AND deleted_at != ''`, now(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Purge(id string) (File, error) {
	f, err := s.GetFile(id)
	if err != nil {
		return File{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(`DELETE FROM share_links WHERE file_id = ?`, id)
	if err != nil {
		return File{}, err
	}
	_, err = s.db.Exec(`DELETE FROM drive_files WHERE id = ?`, id)
	return f, err
}

func (s *Store) CreateShare(fileID string, expiresAt string, maxDownloads int) (ShareLink, error) {
	if !storage.ValidID(fileID) {
		return ShareLink{}, ErrInvalid
	}
	tok, err := randomToken(32)
	if err != nil {
		return ShareLink{}, err
	}
	id, err := newID()
	if err != nil {
		return ShareLink{}, err
	}
	link := ShareLink{
		ID: id, FileID: fileID, Token: tok, ExpiresAt: expiresAt,
		MaxDownloads: maxDownloads, CreatedAt: now(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(`INSERT INTO share_links (id, file_id, token, expires_at, max_downloads, download_count, revoked, created_at)
		VALUES (?, ?, ?, ?, ?, 0, 0, ?)`,
		link.ID, link.FileID, link.Token, link.ExpiresAt, link.MaxDownloads, link.CreatedAt)
	return link, err
}

func (s *Store) GetShareByToken(token string) (ShareLink, error) {
	if len(token) < 16 {
		return ShareLink{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.db.QueryRow(`SELECT id, file_id, token, expires_at, max_downloads, download_count, revoked, created_at
		FROM share_links WHERE token = ?`, token)
	var link ShareLink
	var revoked int
	err := row.Scan(&link.ID, &link.FileID, &link.Token, &link.ExpiresAt, &link.MaxDownloads, &link.DownloadCount, &revoked, &link.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ShareLink{}, ErrNotFound
	}
	if err != nil {
		return ShareLink{}, err
	}
	link.Revoked = revoked != 0
	return link, nil
}

func (s *Store) ListShares() ([]ShareLink, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT s.id, s.file_id, s.token, s.expires_at, s.max_downloads, s.download_count, s.revoked, s.created_at, f.name
		FROM share_links s LEFT JOIN drive_files f ON f.id = s.file_id
		WHERE s.revoked = 0 ORDER BY s.created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ShareLink
	for rows.Next() {
		var link ShareLink
		var revoked int
		var name sql.NullString
		if err := rows.Scan(&link.ID, &link.FileID, &link.Token, &link.ExpiresAt, &link.MaxDownloads, &link.DownloadCount, &revoked, &link.CreatedAt, &name); err != nil {
			return nil, err
		}
		link.Revoked = revoked != 0
		if name.Valid {
			link.FileName = name.String
		}
		out = append(out, link)
	}
	if out == nil {
		out = []ShareLink{}
	}
	return out, rows.Err()
}

func (s *Store) RevokeShare(id string) error {
	if !storage.ValidID(id) {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE share_links SET revoked = 1 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) IncShareDownload(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE share_links SET download_count = download_count + 1 WHERE id = ?`, id)
	return err
}

func (s *Store) UsageBytes() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n sql.NullInt64
	err := s.db.QueryRow(`SELECT SUM(size) FROM drive_files WHERE deleted_at = '' AND status = 'ready'`).Scan(&n)
	if err != nil {
		return 0, err
	}
	if !n.Valid {
		return 0, nil
	}
	return n.Int64, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanFile(row scanner) (File, error) {
	var f File
	err := row.Scan(&f.ID, &f.FolderID, &f.Name, &f.MIME, &f.Size, &f.SHA256, &f.Provider, &f.ObjectKey, &f.Status, &f.CreatedAt, &f.UpdatedAt, &f.DeletedAt, &f.RecoveryStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, err
	}
	return f, nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ObjectKeyFor строит непредсказуемый ключ объекта (не последовательный id пути).
func ObjectKeyFor(fileID, name string) string {
	safe := storage.SanitizeName(name)
	safe = strings.ReplaceAll(safe, " ", "_")
	return fmt.Sprintf("drive/%s/%s", fileID, safe)
}
