package drive

import (
	"database/sql"
	"errors"
	"strings"

	"ytdrive/internal/storage"
)

// FileChunk — одна часть файла (отдельный YouTube-ролик).
type FileChunk struct {
	ID                 string `json:"id"`
	FileID             string `json:"file_id"`
	ChunkIndex         int    `json:"chunk_index"`
	ChunkTotal         int    `json:"chunk_total"`
	OriginalSize       int64  `json:"original_size"`
	EncodedSize        int64  `json:"encoded_size,omitempty"`
	ChunkSHA256        string `json:"chunk_sha256"`
	CodecVersion       int    `json:"codec_version"`
	YouTubeVideoID     string `json:"youtube_video_id,omitempty"`
	UploadStatus       string `json:"upload_status,omitempty"`
	ProcessingStatus   string `json:"processing_status,omitempty"`
	PrivacyStatus      string `json:"privacy_status,omitempty"`
	RecoveryStatus     string `json:"recovery_status,omitempty"`
	Attempts           int    `json:"attempts"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

// FileManifest — сводка по чанкам для admin/diagnostics.
type FileManifest struct {
	FileID       string      `json:"file_id"`
	ChunkTotal   int         `json:"chunk_total"`
	CodecVersion int         `json:"codec_version"`
	ReadyChunks  int         `json:"ready_chunks"`
	Chunks       []FileChunk `json:"chunks"`
}

func (s *Store) migrateChunks() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS file_chunks (
    id TEXT PRIMARY KEY,
    file_id TEXT NOT NULL,
    chunk_index INTEGER NOT NULL,
    chunk_total INTEGER NOT NULL,
    original_size INTEGER NOT NULL DEFAULT 0,
    encoded_size INTEGER NOT NULL DEFAULT 0,
    chunk_sha256 TEXT NOT NULL DEFAULT '',
    codec_version INTEGER NOT NULL DEFAULT 1,
    youtube_video_id TEXT NOT NULL DEFAULT '',
    upload_status TEXT NOT NULL DEFAULT '',
    processing_status TEXT NOT NULL DEFAULT '',
    privacy_status TEXT NOT NULL DEFAULT '',
    recovery_status TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(file_id, chunk_index)
);
CREATE INDEX IF NOT EXISTS idx_file_chunks_file ON file_chunks(file_id);
`)
	return err
}

// CreateChunkMeta создаёт запись части до загрузки на YouTube.
func (s *Store) CreateChunkMeta(fileID string, index, total int, originalSize int64, chunkSHA string, codecVersion int) (FileChunk, error) {
	if !storage.ValidID(fileID) || index < 0 || total < 1 || index >= total {
		return FileChunk{}, ErrInvalid
	}
	id, err := newID()
	if err != nil {
		return FileChunk{}, err
	}
	ts := now()
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(`INSERT INTO file_chunks (
		id, file_id, chunk_index, chunk_total, original_size, chunk_sha256, codec_version,
		upload_status, recovery_status, attempts, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, fileID, index, total, originalSize, chunkSHA, codecVersion,
		"pending", "pending", 0, ts, ts)
	if err != nil {
		return FileChunk{}, err
	}
	return FileChunk{
		ID: id, FileID: fileID, ChunkIndex: index, ChunkTotal: total,
		OriginalSize: originalSize, ChunkSHA256: chunkSHA, CodecVersion: codecVersion,
		UploadStatus: "pending", RecoveryStatus: "pending", CreatedAt: ts, UpdatedAt: ts,
	}, nil
}

// UpdateChunkYouTube сохраняет результат загрузки части.
func (s *Store) UpdateChunkYouTube(id string, videoID string, upload, processing, privacy string, encodedSize int64) error {
	if !storage.ValidID(id) {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE file_chunks SET youtube_video_id=?, upload_status=?, processing_status=?,
		privacy_status=?, encoded_size=?, attempts=attempts+1, updated_at=? WHERE id=?`,
		videoID, upload, processing, privacy, encodedSize, now(), id)
	return err
}

// SetChunkRecovery обновляет статус восстановления части.
func (s *Store) SetChunkRecovery(id, status string) error {
	if !storage.ValidID(id) {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE file_chunks SET recovery_status=?, updated_at=? WHERE id=?`, status, now(), id)
	return err
}

// ListChunks возвращает части файла по возрастанию индекса.
func (s *Store) ListChunks(fileID string) ([]FileChunk, error) {
	if !storage.ValidID(fileID) {
		return nil, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, file_id, chunk_index, chunk_total, original_size, encoded_size,
		chunk_sha256, codec_version, youtube_video_id, upload_status, processing_status, privacy_status,
		recovery_status, attempts, created_at, updated_at
		FROM file_chunks WHERE file_id=? ORDER BY chunk_index ASC`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FileChunk
	for rows.Next() {
		var c FileChunk
		if err := rows.Scan(&c.ID, &c.FileID, &c.ChunkIndex, &c.ChunkTotal, &c.OriginalSize, &c.EncodedSize,
			&c.ChunkSHA256, &c.CodecVersion, &c.YouTubeVideoID, &c.UploadStatus, &c.ProcessingStatus, &c.PrivacyStatus,
			&c.RecoveryStatus, &c.Attempts, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Manifest собирает сводку по частям.
func (s *Store) Manifest(fileID string) (FileManifest, error) {
	chunks, err := s.ListChunks(fileID)
	if err != nil {
		return FileManifest{}, err
	}
	m := FileManifest{FileID: fileID, Chunks: chunks}
	ready := 0
	for _, c := range chunks {
		m.ChunkTotal = c.ChunkTotal
		m.CodecVersion = c.CodecVersion
		if c.YouTubeVideoID != "" && (c.UploadStatus == "processed" || c.UploadStatus == "uploaded" || c.UploadStatus == "ready") {
			ready++
		}
		if c.YouTubeVideoID != "" && c.ProcessingStatus == "succeeded" {
			ready++
		}
	}
	// count unique ready by video id present + not failed
	ready = 0
	for _, c := range chunks {
		if c.YouTubeVideoID == "" {
			continue
		}
		up := strings.ToLower(c.UploadStatus)
		proc := strings.ToLower(c.ProcessingStatus)
		if up == "rejected" || up == "failed" || proc == "failed" {
			continue
		}
		ready++
	}
	m.ReadyChunks = ready
	return m, nil
}

// SetFileChunkTotal обновляет ожидаемое число частей у файла.
func (s *Store) SetFileChunkTotal(fileID string, total, codecVersion int, complete bool) error {
	if !storage.ValidID(fileID) {
		return ErrInvalid
	}
	comp := 0
	if complete {
		comp = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE drive_files SET chunk_total=?, codec_version=?, manifest_complete=?, updated_at=? WHERE id=?`,
		total, codecVersion, comp, now(), fileID)
	return err
}

// MissingChunkIndexes — индексы без video_id (для resume).
func (s *Store) MissingChunkIndexes(fileID string) ([]int, error) {
	chunks, err := s.ListChunks(fileID)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	total := chunks[0].ChunkTotal
	have := map[int]bool{}
	for _, c := range chunks {
		if c.YouTubeVideoID != "" {
			have[c.ChunkIndex] = true
		}
	}
	var missing []int
	for i := 0; i < total; i++ {
		if !have[i] {
			missing = append(missing, i)
		}
	}
	return missing, nil
}

func (s *Store) getChunkByIndex(fileID string, index int) (FileChunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c FileChunk
	err := s.db.QueryRow(`SELECT id, file_id, chunk_index, chunk_total, original_size, encoded_size,
		chunk_sha256, codec_version, youtube_video_id, upload_status, processing_status, privacy_status,
		recovery_status, attempts, created_at, updated_at
		FROM file_chunks WHERE file_id=? AND chunk_index=?`, fileID, index).Scan(
		&c.ID, &c.FileID, &c.ChunkIndex, &c.ChunkTotal, &c.OriginalSize, &c.EncodedSize,
		&c.ChunkSHA256, &c.CodecVersion, &c.YouTubeVideoID, &c.UploadStatus, &c.ProcessingStatus, &c.PrivacyStatus,
		&c.RecoveryStatus, &c.Attempts, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return FileChunk{}, ErrNotFound
	}
	return c, err
}
