package httpapi

import (
	"errors"
	"net/http"
	"net/url"

	"ytdrive/internal/jobs"
	"ytdrive/internal/storage"
	"ytdrive/internal/youtube"
)

func (s *Server) youtubeStatus(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"enabled":   s.youtube.Configured(),
		"connected": false,
		"provider":  "unconfigured",
		"message":   youtube.ErrNotConfigured.Error(),
		"channel":   nil,
	}
	if !s.youtube.Configured() {
		writeJSON(w, http.StatusOK, out)
		return
	}
	out["provider"] = "youtube_data_api_v3"
	out["message"] = "OAuth настроен. Подключите Google-аккаунт для загрузки."
	if s.youtube.Connected(r.Context()) {
		out["connected"] = true
		out["message"] = "YouTube-аккаунт подключён."
		if ch, err := s.youtube.GetConnectedChannel(r.Context()); err == nil {
			out["channel"] = map[string]string{"id": ch.ID, "title": ch.Title}
			out["message"] = "Подключён канал: " + ch.Title
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) youtubeAuthStart(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.youtube.(*youtube.Service)
	if !ok || !s.youtube.Configured() {
		writeError(w, http.StatusServiceUnavailable, youtube.ErrNotConfigured.Error())
		return
	}
	authURL, _, err := svc.BeginAuth()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось начать OAuth")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"auth_url": authURL})
}

func (s *Server) youtubeAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if err := s.youtube.HandleCallback(r.Context(), code, state); err != nil {
		msg := "ошибка OAuth"
		switch {
		case errors.Is(err, youtube.ErrInvalidState):
			msg = youtube.ErrInvalidState.Error()
		case errors.Is(err, youtube.ErrNotConfigured):
			msg = youtube.ErrNotConfigured.Error()
		case errors.Is(err, youtube.ErrAuth):
			msg = "авторизация отклонена или недействительна"
		}
		http.Redirect(w, r, "/youtube?error="+url.QueryEscape(msg), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/youtube?connected=1", http.StatusFound)
}

func (s *Server) youtubeDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := s.youtube.Disconnect(r.Context()); err != nil {
		if errors.Is(err, youtube.ErrNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "не удалось отключить аккаунт")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

func (s *Server) youtubeUpload(w http.ResponseWriter, r *http.Request, fileID string) {
	job, err := s.jobs.EnqueueYouTubeUpload(fileID)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrNotFound), errors.Is(err, storage.ErrInvalidID):
			writeError(w, http.StatusNotFound, "не найдено")
		case errors.Is(err, youtube.ErrNotConfigured):
			writeError(w, http.StatusServiceUnavailable, err.Error())
		case errors.Is(err, youtube.ErrNotConnected):
			writeError(w, http.StatusUnauthorized, err.Error())
		case errors.Is(err, youtube.ErrAlreadyUploaded):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, jobs.ErrNoVideo):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, jobs.ErrBusy):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, jobs.ErrQueueFull):
			writeError(w, http.StatusTooManyRequests, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) youtubeFileStatus(w http.ResponseWriter, r *http.Request, fileID string) {
	f, err := s.store.GetFile(fileID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	out := map[string]any{
		"file_id":                   f.ID,
		"youtube_video_id":          f.YouTubeVideoID,
		"youtube_url":               f.YouTubeURL,
		"youtube_upload_status":     f.YouTubeUploadStatus,
		"youtube_processing_status": f.YouTubeProcessingStatus,
		"youtube_uploaded_at":       f.YouTubeUploadedAt,
		"youtube_error":             f.YouTubeError,
		"youtube_privacy_status":    f.YouTubePrivacyStatus,
	}
	if f.YouTubeVideoID != "" && s.youtube.Configured() && s.youtube.Connected(r.Context()) {
		if ref, err := s.youtube.GetVideoStatus(r.Context(), f.YouTubeVideoID); err == nil {
			out["youtube_upload_status"] = ref.UploadStatus
			out["youtube_processing_status"] = ref.ProcessingStatus
			out["youtube_privacy_status"] = ref.PrivacyStatus
			_ = s.store.SetYouTube(f.ID, storage.YouTubeMeta{
				VideoID:          f.YouTubeVideoID,
				URL:              f.YouTubeURL,
				UploadStatus:     ref.UploadStatus,
				ProcessingStatus: ref.ProcessingStatus,
				UploadedAt:       f.YouTubeUploadedAt,
				PrivacyStatus:    ref.PrivacyStatus,
			})
		}
	}
	writeJSON(w, http.StatusOK, out)
}
