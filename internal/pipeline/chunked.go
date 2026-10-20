package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ytdrive/internal/codec"
	"ytdrive/internal/contentcrypto"
	"ytdrive/internal/drive"
	"ytdrive/internal/format"
	"ytdrive/internal/storage"
	"ytdrive/internal/youtube"
)

// UploadProgress сообщает о ходе многочастной загрузки.
type UploadProgress func(chunkIndex, chunkTotal int, phase string, pct int, msg string)

// UploadChunkedStream принимает поток до MaxBytes, режет на части ≤ MaxChunkBytes,
// шифрует/кодирует/загружает последовательно. Не держит весь файл и все MP4 сразу.
func (s *Service) UploadChunkedStream(ctx context.Context, folderID, name, mime string, r io.Reader, sizeHint int64, progress UploadProgress) (drive.File, error) {
	if !s.yt.Configured() || !s.yt.Connected(ctx) {
		return drive.File{}, youtube.ErrNotConnected
	}
	maxFile := s.cfg.MaxBytes
	chunkSize := s.cfg.MaxChunkBytes
	if chunkSize <= 0 {
		chunkSize = 5 << 20
	}
	if maxFile <= 0 {
		maxFile = 50 << 20
	}
	if sizeHint > maxFile {
		return drive.File{}, storage.ErrTooLarge
	}

	codecVer := int(format.Version2)
	encOpt := s.cfg.Encode
	if encOpt.FormatVersion == 0 {
		encOpt = codec.DefaultOptionsV2()
	}
	if encOpt.FormatVersion == format.Version2 {
		codecVer = 2
	}

	// Предварительно создаём карточку файла (байты ещё не готовы).
	f, err := s.meta.CreateFileMeta(folderID, name, mime, "youtube", "youtube:pending", "uploading")
	if err != nil {
		return drive.File{}, err
	}
	fileID := f.ID

	hasher := sha256.New()
	tr := io.TeeReader(io.LimitReader(r, maxFile+1), hasher)

	var (
		index      int
		totalKnown int
		totalBytes int64
		buf        = make([]byte, chunkSize)
	)

	// Первый проход: если sizeHint известен — сразу считаем total.
	if sizeHint > 0 {
		totalKnown = int((sizeHint + chunkSize - 1) / chunkSize)
		if totalKnown < 1 {
			totalKnown = 1
		}
		_ = s.meta.SetFileChunkTotal(fileID, totalKnown, codecVer, false)
	}

	for {
		if err := ctx.Err(); err != nil {
			_ = s.meta.SetStatus(fileID, "error")
			return drive.File{}, err
		}
		n, readErr := io.ReadFull(tr, buf)
		if n == 0 && (readErr == io.EOF || readErr == io.ErrUnexpectedEOF) {
			break
		}
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			_ = s.meta.SetStatus(fileID, "error")
			return drive.File{}, readErr
		}
		plain := append([]byte(nil), buf[:n]...)
		totalBytes += int64(n)

		if totalBytes > maxFile {
			_ = s.meta.SetStatus(fileID, "error")
			return drive.File{}, storage.ErrTooLarge
		}

		// Уточняем total по ходу, если sizeHint не был задан.
		chunkTotal := totalKnown
		if chunkTotal == 0 {
			// Пока неизвестно — ставим provisional и обновим в конце.
			chunkTotal = index + 1
			if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
				chunkTotal = index + 1
			} else {
				chunkTotal = index + 2 // как минимум ещё одна
			}
		}

		if progress != nil {
			progress(index, max(chunkTotal, index+1), "encrypt", 10, fmt.Sprintf("Часть %d: шифрование", index+1))
		}
		chunkSHA := sha256.Sum256(plain)
		chunkHex := hex.EncodeToString(chunkSHA[:])

		ch, err := s.meta.CreateChunkMeta(fileID, index, max(chunkTotal, index+1), int64(n), chunkHex, codecVer)
		if err != nil {
			_ = s.meta.SetStatus(fileID, "error")
			return drive.File{}, err
		}

		aad := contentcrypto.ChunkAAD(fileID, index, max(chunkTotal, index+1), int64(n))
		payload := plain
		if s.box != nil {
			payload, err = s.box.SealAAD(plain, aad)
			if err != nil {
				_ = s.meta.SetStatus(fileID, "error")
				return drive.File{}, err
			}
		}

		if progress != nil {
			progress(index, max(chunkTotal, index+1), "encode", 40, fmt.Sprintf("Часть %d: кодирование", index+1))
		}
		tmpDir, err := os.MkdirTemp(s.cfg.TempDir, "infidrive-chunk-*")
		if err != nil {
			return drive.File{}, err
		}
		srcPath := filepath.Join(tmpDir, "payload.bin")
		mp4Path := filepath.Join(tmpDir, "out.mp4")
		cleanup := func() { _ = os.RemoveAll(tmpDir) }

		if err := os.WriteFile(srcPath, payload, 0o600); err != nil {
			cleanup()
			return drive.File{}, err
		}
		// Освобождаем plaintext/ciphertext из heap как можно раньше.
		plain = nil
		payload = nil

		opt := encOpt
		opt.DisplayName = fmt.Sprintf("chunk-%d", index)
		if progress != nil {
			progress(index, max(chunkTotal, index+1), "encode", 55, fmt.Sprintf("Часть %d: FFmpeg", index+1))
		}
		encRes, err := codec.EncodeFile(ctx, srcPath, mp4Path, opt, nil)
		_ = os.Remove(srcPath) // payload больше не нужен на диске
		if err != nil {
			cleanup()
			_ = s.meta.SetStatus(fileID, "error")
			return drive.File{}, err
		}

		st, _ := os.Stat(mp4Path)
		encSize := int64(0)
		if st != nil {
			encSize = st.Size()
		}
		if s.cfg.MaxTempBytes > 0 && encSize > s.cfg.MaxTempBytes {
			cleanup()
			_ = s.meta.SetStatus(fileID, "error")
			return drive.File{}, fmt.Errorf("временный MP4 (%d) превышает MAX_TEMP_MIB", encSize)
		}

		if progress != nil {
			progress(index, max(chunkTotal, index+1), "upload", 75, fmt.Sprintf("Часть %d: YouTube", index+1))
		}
		title := fmt.Sprintf("infidrive-%s-c%02d", fileID[:8], index)
		ref, err := s.yt.UploadVideo(ctx, mp4Path, youtube.UploadOptions{
			Title:       title,
			Description: "A-Tex InfiDrive chunk (encrypted). Not an archive.",
			Privacy:     s.cfg.Privacy,
		})
		cleanup() // MP4 больше не нужен локально
		if err != nil {
			_ = s.meta.SetStatus(fileID, "error")
			return drive.File{}, err
		}

		ref, waitErr := s.waitReadyBackoff(ctx, ref.VideoID)
		_ = s.meta.UpdateChunkYouTube(ch.ID, ref.VideoID, ref.UploadStatus, ref.ProcessingStatus, ref.PrivacyStatus, encSize)
		if waitErr != nil {
			// Часть загружена, но processing ещё идёт — продолжаем остальные, файл не ready.
			_ = s.meta.SetStatus(fileID, "processing")
		}
		_ = encRes

		index++
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
	}

	if index == 0 {
		_ = s.meta.SetStatus(fileID, "error")
		return drive.File{}, fmt.Errorf("пустой файл")
	}

	plainHex := hex.EncodeToString(hasher.Sum(nil))
	_ = s.meta.SetFileChunkTotal(fileID, index, codecVer, true)
	chunks, _ := s.meta.ListChunks(fileID)

	// Первый video_id для обратной совместимости UI (v1 single-video поля).
	if len(chunks) > 0 && chunks[0].YouTubeVideoID != "" {
		_ = s.meta.SetYouTubeMeta(fileID, chunks[0].YouTubeVideoID, chunks[0].UploadStatus, chunks[0].ProcessingStatus, chunks[0].PrivacyStatus, plainHex)
		_ = s.meta.SetObjectKey(fileID, "youtube:"+chunks[0].YouTubeVideoID)
	}

	missing, _ := s.meta.MissingChunkIndexes(fileID)
	if len(missing) > 0 {
		_ = s.meta.SetStatus(fileID, "processing")
		f, _ = s.meta.GetFile(fileID)
		return f, fmt.Errorf("%w: не хватает частей %v", ErrNotReady, missing)
	}
	if err := s.meta.FinalizeFile(fileID, totalBytes, plainHex); err != nil {
		return drive.File{}, err
	}
	if progress != nil {
		progress(index-1, index, "done", 100, "Загрузка завершена")
	}
	return s.meta.GetFile(fileID)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// waitReadyBackoff — polling со экспоненциальной задержкой и общим дедлайном.
func (s *Service) waitReadyBackoff(ctx context.Context, videoID string) (youtube.VideoRef, error) {
	deadline := s.cfg.WaitProcessing
	if deadline <= 0 {
		deadline = 30 * time.Minute
	}
	end := time.Now().Add(deadline)
	var last youtube.VideoRef
	delay := 2 * time.Second
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
		if time.Now().After(end) {
			return ref, fmt.Errorf("%w: timeout upload=%s processing=%s privacy=%s", ErrNotReady, ref.UploadStatus, ref.ProcessingStatus, ref.PrivacyStatus)
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(delay):
		}
		if delay < 30*time.Second {
			delay *= 2
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
		}
	}
}
