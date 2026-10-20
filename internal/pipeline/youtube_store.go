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
	"ytdrive/internal/storage"
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
	MaxBytes       int64
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
		cfg.MaxBytes = 128 << 10
	}
	if cfg.Privacy == "" {
		cfg.Privacy = "private"
	}
	if cfg.WaitProcessing <= 0 {
		cfg.WaitProcessing = 2 * time.Minute
	}
	return &Service{drive: driveSvc, meta: meta, yt: yt, retriever: ret, box: box, cfg: cfg}
}

// UploadEncryptEncodePublish принимает исходный файл, шифрует, кодирует во временный MP4,
// загружает на YouTube и удаляет локальные байты. В SQLite остаётся metadata + video_id.
func (s *Service) UploadEncryptEncodePublish(ctx context.Context, folderID, name, mime string, r io.Reader, sizeHint int64) (drive.File, error) {
	if !s.yt.Configured() || !s.yt.Connected(ctx) {
		return drive.File{}, youtube.ErrNotConnected
	}
	if sizeHint > s.cfg.MaxBytes {
		return drive.File{}, storage.ErrTooLarge
	}
	plain, err := io.ReadAll(io.LimitReader(r, s.cfg.MaxBytes+1))
	if err != nil {
		return drive.File{}, err
	}
	if int64(len(plain)) > s.cfg.MaxBytes {
		return drive.File{}, storage.ErrTooLarge
	}
	plainSHA := sha256.Sum256(plain)
	plainHex := hex.EncodeToString(plainSHA[:])

	payload := plain
	if s.box != nil {
		payload, err = s.box.Seal(plain)
		if err != nil {
			return drive.File{}, err
		}
	}

	tmpDir, err := os.MkdirTemp(s.cfg.TempDir, "infidrive-exp-*")
	if err != nil {
		return drive.File{}, err
	}
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "payload.bin")
	mp4Path := filepath.Join(tmpDir, "out.mp4")
	if err := os.WriteFile(srcPath, payload, 0o600); err != nil {
		return drive.File{}, err
	}

	opt := s.cfg.Encode
	opt.DisplayName = storage.SanitizeName(name)
	encRes, err := codec.EncodeFile(ctx, srcPath, mp4Path, opt, nil)
	if err != nil {
		return drive.File{}, err
	}

	title := strings.TrimSuffix(opt.DisplayName, filepath.Ext(opt.DisplayName))
	if title == "" {
		title = "InfiDrive"
	}
	ref, err := s.yt.UploadVideo(ctx, mp4Path, youtube.UploadOptions{
		Title:       title,
		Description: "A-Tex InfiDrive experimental (encrypted). Do not treat as archive.",
		Privacy:     s.cfg.Privacy,
	})
	if err != nil {
		return drive.File{}, err
	}

	// Ждём processing / проверяем статусы — не считаем готовым только по video_id.
	ref, err = s.waitReady(ctx, ref.VideoID)
	if err != nil {
		// video_id уже есть — сохраняем метаданные со статусом processing/error
		f, metaErr := s.persistYouTubeFile(folderID, name, mime, int64(len(plain)), plainHex, ref, "processing")
		if metaErr != nil {
			return drive.File{}, fmt.Errorf("%v; meta: %w", err, metaErr)
		}
		f.Status = "processing"
		return f, err
	}

	_ = encRes
	return s.persistYouTubeFile(folderID, name, mime, int64(len(plain)), plainHex, ref, "ready")
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
	n, copyErr := io.Copy(w, io.LimitReader(mp4, 64<<20))
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
		plain, err = s.box.Open(payload)
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
	if _, err := s.meta.GetFile(fileID); err != nil {
		return "", nil, err
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

// BufferReader для тестов.
func BufferReader(b []byte) io.ReadCloser { return io.NopCloser(bytes.NewReader(b)) }
