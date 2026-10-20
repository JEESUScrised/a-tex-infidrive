package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"ytdrive/internal/pipeline"
	"ytdrive/internal/retrieval"
	"ytdrive/internal/storage"
	"ytdrive/internal/youtube"
)

func (s *Server) handlePipeline(w http.ResponseWriter, r *http.Request, path string, parts []string) bool {
	if s.pipeline == nil {
		return false
	}
	switch {
	case r.Method == http.MethodPost && path == "drive/upload":
		// Перехватываем upload в experimental YouTube pipeline, если включён.
		if s.cfg.ExperimentalYouTubeStore {
			s.pipelineUpload(w, r)
			return true
		}
		return false
	case r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "drive" && parts[2] == "download":
		if s.cfg.ExperimentalYouTubeStore {
			s.pipelineDownload(w, r, parts[1])
			return true
		}
		return false
	case r.Method == http.MethodPost && len(parts) == 3 && parts[0] == "drive" && parts[2] == "restore":
		s.pipelineManualRestore(w, r, parts[1])
		return true
	case r.Method == http.MethodPost && len(parts) == 3 && parts[0] == "drive" && parts[2] == "restore-job":
		s.pipelineStartRestoreJob(w, r, parts[1])
		return true
	case r.Method == http.MethodGet && len(parts) == 2 && parts[0] == "restore-jobs":
		s.pipelineRestoreJobStatus(w, r, parts[1])
		return true
	case r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "restore-jobs" && parts[2] == "file":
		s.pipelineRestoreJobFile(w, r, parts[1])
		return true
	case r.Method == http.MethodGet && path == "admin/retrieval":
		writeJSON(w, http.StatusOK, s.pipeline.RetrieverMeta(r.Context()))
		return true
	case r.Method == http.MethodGet && len(parts) == 4 && parts[0] == "admin" && parts[1] == "files" && parts[3] == "manifest":
		s.adminFileManifest(w, r, parts[2])
		return true
	}
	return false
}

func (s *Server) adminFileManifest(w http.ResponseWriter, _ *http.Request, fileID string) {
	if s.drive == nil {
		writeError(w, http.StatusNotFound, "не найдено")
		return
	}
	m, err := s.drive.Manifest(fileID)
	if err != nil {
		writeDriveError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) pipelineStartRestoreJob(w http.ResponseWriter, r *http.Request, fileID string) {
	if s.restoreHub == nil {
		writeError(w, http.StatusServiceUnavailable, "restore hub недоступен")
		return
	}
	job, err := s.restoreHub.Start(fileID)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	go s.pipeline.RunRestoreJob(context.Background(), s.restoreHub, job.ID, fileID)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) pipelineRestoreJobStatus(w http.ResponseWriter, r *http.Request, id string) {
	st, ok := s.restoreHub.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "задание не найдено")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) pipelineRestoreJobFile(w http.ResponseWriter, r *http.Request, id string) {
	name, data, err := s.restoreHub.TakeResult(id)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", storage.SanitizeName(name)))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) pipelineUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUpload+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "ожидается multipart file")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "поле file обязательно")
		return
	}
	defer file.Close()
	folderID := r.FormValue("folder_id")
	rec, err := s.pipeline.UploadEncryptEncodePublish(r.Context(), folderID, hdr.Filename, storage.DetectMIME(hdr.Filename), file, hdr.Size)
	if err != nil {
		if errors.Is(err, pipeline.ErrNotReady) {
			writeJSON(w, http.StatusAccepted, map[string]any{"file": rec, "warning": err.Error()})
			return
		}
		writePipelineError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) pipelineDownload(w http.ResponseWriter, r *http.Request, id string) {
	name, data, err := s.pipeline.Download(r.Context(), id)
	if err != nil {
		writePipelineError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", storage.SanitizeName(name)))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) pipelineManualRestore(w http.ResponseWriter, r *http.Request, id string) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "ожидается multipart file (MP4)")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "поле file обязательно (MP4 из Studio)")
		return
	}
	defer file.Close()
	name, data, err := s.pipeline.RestoreFromStream(r.Context(), id, file)
	if err != nil {
		writePipelineError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", storage.SanitizeName(name)))
	w.Header().Set("X-InfiDrive-Restore", "manual-mp4")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func writePipelineError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, retrieval.ErrCaptcha):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": err.Error(),
			"code":  "captcha_required",
			"hint":  "Сторонний сервис запросил проверку; автоматическое восстановление остановлено",
		})
	case errors.Is(err, pipeline.ErrNeedManualMP4), errors.Is(err, retrieval.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": err.Error(),
			"code":  "retrieval_unavailable",
			"hint":  "Импортируйте файл вручную или повторите позже",
		})
	case errors.Is(err, pipeline.ErrIntegrity):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, youtube.ErrNotConnected), errors.Is(err, youtube.ErrNotConfigured):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, storage.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, pipeline.ErrNotReady):
		writeError(w, http.StatusAccepted, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
