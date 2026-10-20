// Package pipeline — экспериментальный цикл encode→YouTube→retrieve→decode.
package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ytdrive/internal/codec"
	"ytdrive/internal/contentcrypto"
	"ytdrive/internal/drive"
	"ytdrive/internal/ffmpeg"
	"ytdrive/internal/retrieval"
	"ytdrive/internal/youtube"
)

var (
	ErrNotReady      = errors.New("видео ещё обрабатывается на YouTube")
	ErrIntegrity     = errors.New("SHA-256 не совпал — результат не выдаётся")
	ErrNoVideoID     = errors.New("у файла нет video_id")
	ErrNeedManualMP4 = errors.New("автоматическое получение MP4 недоступно: импортируйте ролик вручную")
)

// Config — лимиты учебного эксперимента.
type Config struct {
	MaxBytes       int64 // весь файл (например 50 MiB)
	MaxChunkBytes  int64 // одна часть (например 5 MiB)
	MaxTempBytes   int64 // потолок одного временного MP4
	Privacy        string // private|unlisted|public — default private
	TempDir        string
	Encode         codec.Options
	WaitProcessing time.Duration
}

// Service связывает drive-метаданные, кодек, YouTube и retrieval.
type Service struct {
	drive     *drive.Service
	meta      *drive.Store
	yt        youtube.VideoStorageProvider
	retriever retrieval.VideoRetriever
	box       *contentcrypto.Box
	cfg       Config
}

func New(driveSvc *drive.Service, meta *drive.Store, yt youtube.VideoStorageProvider, ret retrieval.VideoRetriever, box *contentcrypto.Box, cfg Config) *Service {
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 50 << 20
	}
	if cfg.MaxChunkBytes <= 0 {
		cfg.MaxChunkBytes = 5 << 20
	}
	if cfg.MaxTempBytes <= 0 {
		cfg.MaxTempBytes = 1024 << 20
	}
	if cfg.Privacy == "" {
		cfg.Privacy = "private"
	}
	if cfg.WaitProcessing <= 0 {
		cfg.WaitProcessing = 30 * time.Minute
	}
	if cfg.Encode.Width == 0 {
		cfg.Encode = codec.DefaultOptionsV2()
	}
	return &Service{drive: driveSvc, meta: meta, yt: yt, retriever: ret, box: box, cfg: cfg}
}

// UploadEncryptEncodePublish — потоковая многочастная загрузка (codec v2 + chunks).
func (s *Service) UploadEncryptEncodePublish(ctx context.Context, folderID, name, mime string, r io.Reader, sizeHint int64) (drive.File, error) {
	return s.UploadChunkedStream(ctx, folderID, name, mime, r, sizeHint, nil)
}

func (s *Service) waitReady(ctx context.Context, videoID string) (youtube.VideoRef, error) {
	deadline := time.Now().Add(s.cfg.WaitProcessing)
	var last youtube.VideoRef
	for {
		ref, err := s.yt.GetVideoStatus(ctx, videoID)
		if err != nil {
			return last, err
		}
		last = ref
		up := strings.ToLower(ref.UploadStatus)
		proc := strings.ToLower(ref.ProcessingStatus)
		priv := strings.ToLower(ref.PrivacyStatus)
		if up == "rejected" || up == "failed" || proc == "failed" {
			return ref, fmt.Errorf("%w: upload=%s processing=%s", ErrNotReady, ref.UploadStatus, ref.ProcessingStatus)
		}
		privacyOK := s.cfg.Privacy != "public" || priv == "public" || priv == ""
		if privacyOK && (up == "processed" || up == "uploaded") && (proc == "succeeded" || proc == "") {
			return ref, nil
		}
		if time.Now().After(deadline) {
			return ref, fmt.Errorf("%w: timeout upload=%s processing=%s privacy=%s", ErrNotReady, ref.UploadStatus, ref.ProcessingStatus, ref.PrivacyStatus)
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

func (s *Service) persistYouTubeFile(folderID, name, mime string, size int64, plainSHA string, ref youtube.VideoRef, status string) (drive.File, error) {
	key := "youtube:" + ref.VideoID
	f, err := s.meta.CreateFileMeta(folderID, name, mime, "youtube", key, "uploading")
	if err != nil {
		return drive.File{}, err
	}
	if err := s.meta.SetObjectKey(f.ID, key); err != nil {
		return drive.File{}, err
	}
	if err := s.meta.SetYouTubeMeta(f.ID, ref.VideoID, ref.UploadStatus, ref.ProcessingStatus, ref.PrivacyStatus, plainSHA); err != nil {
		return drive.File{}, err
	}
	if status == "ready" {
		if err := s.meta.FinalizeFile(f.ID, size, plainSHA); err != nil {
			return drive.File{}, err
		}
	} else {
		_ = s.meta.SetStatus(f.ID, status)
	}
	return s.meta.GetFile(f.ID)
}

// RestoreFromStream декодирует MP4 из потока, расшифровывает и проверяет SHA-256
// ДО сообщения об успехе. Результат только в RAM.
func (s *Service) RestoreFromStream(ctx context.Context, fileID string, mp4 io.Reader) (string, []byte, error) {
	return s.RestoreFromStreamProgress(ctx, fileID, mp4, nil)
}

// RestoreFromStreamProgress — то же с колбэком прогресса.
func (s *Service) RestoreFromStreamProgress(ctx context.Context, fileID string, mp4 io.Reader, progress retrieval.ProgressFunc) (name string, data []byte, err error) {
	f, err := s.meta.GetFile(fileID)
	if err != nil {
		return "", nil, err
	}
	tmpDir, err := os.MkdirTemp(s.cfg.TempDir, "infidrive-restore-*")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(tmpDir)

	mp4Path := filepath.Join(tmpDir, "in.mp4")
	outPath := filepath.Join(tmpDir, "out.bin")
	w, err := os.OpenFile(mp4Path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", nil, err
	}
	limit := s.cfg.MaxTempBytes
	if limit <= 0 {
		limit = 1024 << 20
	}
	n, copyErr := io.Copy(w, io.LimitReader(mp4, limit))
	closeErr := w.Close()
	if copyErr != nil || closeErr != nil || n == 0 {
		return "", nil, fmt.Errorf("не удалось принять MP4")
	}

	if progress != nil {
		progress("probe", 78, "Восстановление…")
	}
	runner := ffmpeg.New(s.cfg.Encode.FFmpeg, s.cfg.Encode.FFProbe)
	if _, err := runner.ProbeMedia(ctx, mp4Path); err != nil {
		return "", nil, fmt.Errorf("проверка видео: %w", err)
	}

	if progress != nil {
		progress("decode", 85, "Восстановление…")
	}
	res, err := codec.DecodeFile(ctx, mp4Path, outPath, codec.Options{FFmpeg: s.cfg.Encode.FFmpeg, FFProbe: s.cfg.Encode.FFProbe}, nil)
	if err != nil {
		return "", nil, err
	}
	payload, err := os.ReadFile(outPath)
	if err != nil {
		return "", nil, err
	}
	plain := payload
	if s.box != nil {
		if progress != nil {
			progress("decrypt", 92, "Восстановление…")
		}
		plain, err = s.openPayload(fileID, payload)
		if err != nil {
			return "", nil, err
		}
	}
	if progress != nil {
		progress("verify", 96, "Восстановление…")
	}
	sum := sha256.Sum256(plain)
	got := hex.EncodeToString(sum[:])
	expected := f.SHA256
	if expected == "" {
		expected, _ = s.meta.PlainSHA(f.ID)
	}
	if expected != "" && !strings.EqualFold(got, expected) {
		return "", nil, fmt.Errorf("%w: expected %s got %s", ErrIntegrity, expected, got)
	}
	name = f.Name
	if res.Name != "" && name == "" {
		name = res.Name
	}
	if progress != nil {
		progress("done", 100, "Готово")
	}
	return name, plain, nil
}

// Download восстанавливает файл: auto-retrieve или ошибка с предложением manual.
func (s *Service) Download(ctx context.Context, fileID string) (string, []byte, error) {
	return s.DownloadWithProgress(ctx, fileID, nil)
}

// DownloadWithProgress — Download с прогрессом подготовки/восстановления.
func (s *Service) DownloadWithProgress(ctx context.Context, fileID string, progress retrieval.ProgressFunc) (string, []byte, error) {
	f, err := s.meta.GetFile(fileID)
	if err != nil {
		return "", nil, err
	}
	chunks, err := s.meta.ListChunks(fileID)
	if err != nil {
		return "", nil, err
	}
	if len(chunks) > 0 {
		return s.downloadChunks(ctx, f, chunks, progress)
	}
	vid, _ := s.meta.VideoID(fileID)
	if vid == "" {
		return "", nil, ErrNoVideoID
	}
	if s.retriever == nil {
		return "", nil, ErrNeedManualMP4
	}
	if err := s.retriever.Available(ctx); err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrNeedManualMP4, err)
	}
	if progress != nil {
		progress("retrieve", 25, "Подготовка файла…")
	}
	rc, err := s.retriever.Retrieve(ctx, vid)
	if err != nil {
		if errors.Is(err, retrieval.ErrCaptcha) {
			return "", nil, err
		}
		return "", nil, fmt.Errorf("%w: %v", ErrNeedManualMP4, err)
	}
	defer rc.Close()
	return s.RestoreFromStreamProgress(ctx, fileID, rc, progress)
}

func (s *Service) downloadChunks(ctx context.Context, f drive.File, chunks []drive.FileChunk, progress retrieval.ProgressFunc) (string, []byte, error) {
	if s.retriever == nil {
		return "", nil, ErrNeedManualMP4
	}
	if err := s.retriever.Available(ctx); err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrNeedManualMP4, err)
	}
	out := make([]byte, 0, f.Size)
	total := len(chunks)
	for i, ch := range chunks {
		if ch.YouTubeVideoID == "" {
			return "", nil, fmt.Errorf("%w: часть %d без video_id", ErrNotReady, ch.ChunkIndex)
		}
		if progress != nil {
			pct := 10 + (i * 80 / total)
			progress("retrieve", pct, fmt.Sprintf("Часть %d/%d…", i+1, total))
		}
		rc, err := s.retriever.Retrieve(ctx, ch.YouTubeVideoID)
		if err != nil {
			if errors.Is(err, retrieval.ErrCaptcha) {
				return "", nil, err
			}
			return "", nil, fmt.Errorf("%w: chunk %d: %v", ErrNeedManualMP4, ch.ChunkIndex, err)
		}
		plain, err := s.restoreOneChunk(ctx, f.ID, ch, rc)
		_ = rc.Close()
		if err != nil {
			return "", nil, err
		}
		sum := sha256.Sum256(plain)
		got := hex.EncodeToString(sum[:])
		if ch.ChunkSHA256 != "" && !strings.EqualFold(got, ch.ChunkSHA256) {
			return "", nil, fmt.Errorf("%w: chunk %d hash", ErrIntegrity, ch.ChunkIndex)
		}
		_ = s.meta.SetChunkRecovery(ch.ID, "ok")
		out = append(out, plain...)
	}
	if progress != nil {
		progress("verify", 96, "Проверка SHA-256…")
	}
	sum := sha256.Sum256(out)
	got := hex.EncodeToString(sum[:])
	expected := f.SHA256
	if expected == "" {
		expected, _ = s.meta.PlainSHA(f.ID)
	}
	if expected != "" && !strings.EqualFold(got, expected) {
		return "", nil, fmt.Errorf("%w: expected %s got %s", ErrIntegrity, expected, got)
	}
	if progress != nil {
		progress("done", 100, "Готово")
	}
	return f.Name, out, nil
}

func (s *Service) restoreOneChunk(ctx context.Context, fileID string, ch drive.FileChunk, mp4 io.Reader) ([]byte, error) {
	tmpDir, err := os.MkdirTemp(s.cfg.TempDir, "infidrive-rchunk-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	mp4Path := filepath.Join(tmpDir, "in.mp4")
	outPath := filepath.Join(tmpDir, "out.bin")
	w, err := os.OpenFile(mp4Path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	limit := s.cfg.MaxTempBytes
	if limit <= 0 {
		limit = 1024 << 20
	}
	_, copyErr := io.Copy(w, io.LimitReader(mp4, limit))
	closeErr := w.Close()
	if copyErr != nil || closeErr != nil {
		return nil, fmt.Errorf("не удалось принять MP4 части %d", ch.ChunkIndex)
	}
	_, err = codec.DecodeFile(ctx, mp4Path, outPath, codec.Options{FFmpeg: s.cfg.Encode.FFmpeg, FFProbe: s.cfg.Encode.FFProbe}, nil)
	if err != nil {
		return nil, err
	}
	payload, err := os.ReadFile(outPath)
	if err != nil {
		return nil, err
	}
	if s.box != nil {
		aad := contentcrypto.ChunkAAD(fileID, ch.ChunkIndex, ch.ChunkTotal, ch.OriginalSize)
		plain, err := s.box.OpenAAD(payload, aad)
		if err != nil {
			// legacy single-file seal without AAD
			plain, err = s.box.Open(payload)
			if err != nil {
				return nil, err
			}
		}
		return plain, nil
	}
	return payload, nil
}

// RetrieverMeta для admin diagnostics.
func (s *Service) RetrieverMeta(ctx context.Context) retrieval.Meta {
	if pw, ok := s.retriever.(*retrieval.Playwright); ok {
		return pw.Diagnose(ctx)
	}
	if ss, ok := s.retriever.(*retrieval.SSYouTube); ok {
		return ss.Diagnose(ctx)
	}
	if s.retriever == nil {
		return retrieval.Meta{Name: "none", Message: "retriever не настроен", ManualFallback: true}
	}
	err := s.retriever.Available(ctx)
	return retrieval.Meta{
		Name:           s.retriever.Name(),
		Available:      err == nil,
		Message:        errString(err),
		Automatic:      err == nil,
		ManualFallback: true,
	}
}

func errString(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}

func (s *Service) openPayload(fileID string, payload []byte) ([]byte, error) {
	chunks, _ := s.meta.ListChunks(fileID)
	if len(chunks) >= 1 {
		ch := chunks[0]
		aad := contentcrypto.ChunkAAD(fileID, ch.ChunkIndex, ch.ChunkTotal, ch.OriginalSize)
		if plain, err := s.box.OpenAAD(payload, aad); err == nil {
			return plain, nil
		}
	}
	// legacy Seal без AAD
	return s.box.Open(payload)
}

// BufferReader для тестов.
func BufferReader(b []byte) io.ReadCloser { return io.NopCloser(bytes.NewReader(b)) }
