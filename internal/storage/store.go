// Package storage хранит метаданные в SQLite и байты файлов вне публичного каталога.
package storage

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	KindEncode        = "encode"
	KindDecode        = "decode"
	KindYouTubeUpload = "youtube_upload"

	StatusQueued     = "queued"
	StatusRunning    = "running"
	StatusSucceeded  = "succeeded"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
	StatusUploading  = "uploading"
	StatusUploaded   = "uploaded"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
)

var (
	ErrNotFound  = errors.New("не найдено")
	ErrInvalidID = errors.New("некорректный идентификатор")
	ErrTooLarge  = errors.New("файл превышает допустимый размер")
)

// File — загруженный пользователем исходный файл.
type File struct {
	ID                      string `json:"id"`
	OriginalName            string `json:"original_name"`
	MIME                    string `json:"mime"`
	Size                    int64  `json:"size"`
	SHA256                  string `json:"sha256"`
	StoredPath              string `json:"-"`
	VideoPath               string `json:"-"`
	CreatedAt               string `json:"created_at"`
	HasVideo                bool   `json:"has_video"`
	YouTubeVideoID          string `json:"youtube_video_id"`
	YouTubeURL              string `json:"youtube_url"`
	YouTubeUploadStatus     string `json:"youtube_upload_status"`
	YouTubeProcessingStatus string `json:"youtube_processing_status"`
	YouTubeUploadedAt       string `json:"youtube_uploaded_at"`
	YouTubeError            string `json:"youtube_error"`
	YouTubePrivacyStatus    string `json:"youtube_privacy_status"`
}

// Job — фоновое кодирование или восстановление.
type Job struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Progress   int    `json:"progress"`
	Message    string `json:"message"`
	Error      string `json:"error"`
	FileID     string `json:"file_id"`
	InputPath  string `json:"-"`
	OutputPath string `json:"-"`
	OutputName string `json:"output_name"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// Paths — каталоги и файл БД. Имя ytdrive.db сохранено для совместимости миграций.
type Paths struct {
	Root     string
	Database string
	Temp     string
}

// Store — SQLite и каталог данных.
type Store struct {
	root string
	tmp  string
	db   *sql.DB
	mu   sync.Mutex
	box  tokenBox
}

// tokenBox шифрует OAuth-токены. Опционально.
type tokenBox interface {
	Seal(string) (string, error)
	Open(string) (string, error)
}

// Open создаёт каталоги и схему. Соединение одно, чтобы не ловить блокировки SQLite.
func Open(root string) (*Store, error) {
	return OpenPaths(Paths{Root: root})
}

// OpenPaths открывает хранилище с явными путями (production).
func OpenPaths(p Paths) (*Store, error) {
	root := p.Root
	if root == "" {
		return nil, fmt.Errorf("не задан каталог хранения")
	}
	tmp := p.Temp
	if tmp == "" {
		tmp = filepath.Join(root, "tmp")
	}
	dbPath := p.Database
	if dbPath == "" {
		dbPath = filepath.Join(root, "ytdrive.db")
	}
	if err := os.MkdirAll(filepath.Join(root, "objects"), 0o750); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, "jobs"), 0o750); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(tmp, 0o750); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o750); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{root: root, tmp: tmp, db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.migrateYouTube(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// SetTokenBox включает шифрование OAuth-токенов at rest.
func (s *Store) SetTokenBox(box tokenBox) { s.box = box }

// TempDir возвращает каталог временных файлов.
func (s *Store) TempDir() string {
	if s.tmp != "" {
		return s.tmp
	}
	return filepath.Join(s.root, "tmp")
}

// ContainsPath проверяет, что path лежит внутри root хранилища (защита от traversal).
func (s *Store) ContainsPath(path string) bool {
	clean := filepath.Clean(path)
	root := filepath.Clean(s.root)
	rel, err := filepath.Rel(root, clean)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Root() string { return s.root }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS files (
    id TEXT PRIMARY KEY,
    original_name TEXT NOT NULL,
    mime TEXT NOT NULL,
    size INTEGER NOT NULL,
    sha256 TEXT NOT NULL,
    stored_path TEXT NOT NULL,
    video_path TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    progress INTEGER NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    file_id TEXT NOT NULL DEFAULT '',
    input_path TEXT NOT NULL DEFAULT '',
    output_path TEXT NOT NULL DEFAULT '',
    output_name TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);`)
	return err
}

func sqliteDSN(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.ToSlash(abs)
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs
	}
	return "file:" + abs + "?_pragma=busy_timeout(5000)"
}

// SaveUpload пишет поток на диск и считает SHA-256. При ошибке каталог удаляется.
func (s *Store) SaveUpload(r io.Reader, name, mime string, maxBytes int64) (File, error) {
	if maxBytes <= 0 {
		return File{}, fmt.Errorf("не задан лимит размера")
	}
	id, err := newID()
	if err != nil {
		return File{}, err
	}
	dir := filepath.Join(s.root, "objects", id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return File{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(dir)
		}
	}()
	dest := filepath.Join(dir, "original")
	f, sum, n, err := writeLimited(dest, r, maxBytes)
	if err != nil {
		if f != nil {
			f.Close()
		}
		return File{}, err
	}
	if err := f.Close(); err != nil {
		return File{}, err
	}
	name = SanitizeName(name)
	if mime == "" {
		mime = "application/octet-stream"
	}
	rec := File{
		ID:           id,
		OriginalName: name,
		MIME:         mime,
		Size:         n,
		SHA256:       sum,
		StoredPath:   dest,
		CreatedAt:    now(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(`INSERT INTO files (id, original_name, mime, size, sha256, stored_path, video_path, created_at)
		VALUES (?, ?, ?, ?, ?, ?, '', ?)`,
		rec.ID, rec.OriginalName, rec.MIME, rec.Size, rec.SHA256, rec.StoredPath, rec.CreatedAt)
	if err != nil {
		return File{}, err
	}
	cleanup = false
	return rec, nil
}

// CreateJobDir готовит рабочий каталог задания и возвращает путь.
func (s *Store) CreateJobDir(id string) (string, error) {
	if !ValidID(id) {
		return "", ErrInvalidID
	}
	dir := filepath.Join(s.root, "jobs", id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	return dir, nil
}

func (s *Store) GetFile(id string) (File, error) {
	if !ValidID(id) {
		return File{}, ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.db.QueryRow(`SELECT id, original_name, mime, size, sha256, stored_path, video_path, created_at,
		youtube_video_id, youtube_url, youtube_upload_status, youtube_processing_status,
		youtube_uploaded_at, youtube_error, youtube_privacy_status
		FROM files WHERE id = ?`, id)
	return scanFile(row)
}

func (s *Store) ListFiles() ([]File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, original_name, mime, size, sha256, stored_path, video_path, created_at,
		youtube_video_id, youtube_url, youtube_upload_status, youtube_processing_status,
		youtube_uploaded_at, youtube_error, youtube_privacy_status
		FROM files ORDER BY created_at DESC LIMIT 200`)
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

func (s *Store) SetVideo(fileID, path string) error {
	if !ValidID(fileID) {
		return ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE files SET video_path = ? WHERE id = ?`, path, fileID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateJob(kind, fileID, inputPath string) (Job, error) {
	if fileID != "" && !ValidID(fileID) {
		return Job{}, ErrInvalidID
	}
	id, err := newID()
	if err != nil {
		return Job{}, err
	}
	job := Job{
		ID:        id,
		Kind:      kind,
		Status:    StatusQueued,
		FileID:    fileID,
		InputPath: inputPath,
		CreatedAt: now(),
		UpdatedAt: now(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(`INSERT INTO jobs (id, kind, status, progress, message, error, file_id, input_path, output_path, output_name, created_at, updated_at)
		VALUES (?, ?, ?, 0, '', '', ?, ?, '', '', ?, ?)`,
		job.ID, job.Kind, job.Status, job.FileID, job.InputPath, job.CreatedAt, job.UpdatedAt)
	if err != nil {
		return Job{}, err
	}
	return job, nil
}

func (s *Store) GetJob(id string) (Job, error) {
	if !ValidID(id) {
		return Job{}, ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.db.QueryRow(`SELECT id, kind, status, progress, message, error, file_id, input_path, output_path, output_name, created_at, updated_at FROM jobs WHERE id = ?`, id)
	return scanJob(row)
}

func (s *Store) ListJobs() ([]Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, kind, status, progress, message, error, file_id, input_path, output_path, output_name, created_at, updated_at FROM jobs ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	if out == nil {
		out = []Job{}
	}
	return out, rows.Err()
}

func (s *Store) UpdateJob(id, status string, progress int, message, errText string) error {
	if !ValidID(id) {
		return ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE jobs SET status = ?, progress = ?, message = ?, error = ?, updated_at = ? WHERE id = ?`,
		status, progress, message, errText, now(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetJobInput(id, path string) error {
	if !ValidID(id) {
		return ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE jobs SET input_path = ?, updated_at = ? WHERE id = ?`, path, now(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetJobOutput(id, path, name string) error {
	if !ValidID(id) {
		return ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE jobs SET output_path = ?, output_name = ?, updated_at = ? WHERE id = ?`,
		path, SanitizeName(name), now(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanFile(row rowScanner) (File, error) {
	var f File
	err := row.Scan(
		&f.ID, &f.OriginalName, &f.MIME, &f.Size, &f.SHA256, &f.StoredPath, &f.VideoPath, &f.CreatedAt,
		&f.YouTubeVideoID, &f.YouTubeURL, &f.YouTubeUploadStatus, &f.YouTubeProcessingStatus,
		&f.YouTubeUploadedAt, &f.YouTubeError, &f.YouTubePrivacyStatus,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, err
	}
	f.HasVideo = f.VideoPath != ""
	return f, nil
}

func scanJob(row rowScanner) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.Kind, &j.Status, &j.Progress, &j.Message, &j.Error, &j.FileID, &j.InputPath, &j.OutputPath, &j.OutputName, &j.CreatedAt, &j.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	return j, nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// NowUTC — метка времени для метаданных YouTube.
func NowUTC() string { return now() }

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// ValidID допускает только 32 шестнадцатеричных символа.
func ValidID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}
