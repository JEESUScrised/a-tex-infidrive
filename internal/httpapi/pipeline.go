package httpapi

import (
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
	case r.Method == http.MethodGet && path == "admin/retrieval":
		writeJSON(w, http.StatusOK, s.pipeline.RetrieverMeta(r.Context()))
		return true
	}
	return false
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
	case errors.Is(err, pipeline.ErrNeedManualMP4), errors.Is(err, retrieval.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":  err.Error(),
			"code":   "retrieval_unavailable",
			"hint":   "Импортируйте MP4 вручную: POST /api/drive/{id}/restore",
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
