// Package httpapi — обработчики REST и раздача SPA (frontend/dist).
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ytdrive/internal/auth"
	"ytdrive/internal/config"
	"ytdrive/internal/drive"
	"ytdrive/internal/ffmpeg"
	"ytdrive/internal/jobs"
	"ytdrive/internal/storage"
	"ytdrive/internal/youtube"
)

const version = "1.0.0"

// Server связывает хранилище, очередь и статику.
type Server struct {
	store    *storage.Store
	jobs     *jobs.Manager
	cfg      config.Config
	youtube  youtube.VideoStorageProvider
	drive    *drive.Service
	web      http.Handler
	sessions *auth.Manager
	limiter  *auth.LoginLimiter
}

// New собирает HTTP-обработчик. Каталог данных не публикуется.
func New(store *storage.Store, manager *jobs.Manager, cfg config.Config, yt youtube.VideoStorageProvider) *Server {
	return NewWithDrive(store, manager, cfg, yt, nil)
}

// NewWithDrive — полный конструктор с облачным диском.
func NewWithDrive(store *storage.Store, manager *jobs.Manager, cfg config.Config, yt youtube.VideoStorageProvider, driveSvc *drive.Service) *Server {
	web := resolveWebDir(cfg.WebDir)
	if yt == nil {
		yt = youtube.Unconfigured{}
	}
	s := &Server{
		store:   store,
		jobs:    manager,
		cfg:     cfg,
		youtube: yt,
		drive:   driveSvc,
		web:     spaFileServer(web),
		limiter: auth.NewLoginLimiter(10, 15*time.Minute),
	}
	if cfg.AuthEnabled {
		secret := cfg.SessionSecret
		if secret == "" {
			secret = "dev-only-insecure-session-secret!!!!"
		}
		s.sessions = auth.NewManager(secret, cfg.CookieSecure)
	}
	return s
}

// Handler возвращает маршрутизатор.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.handleAPI(w, r)
			return
		}
		// Публичная share-ссылка без авторизации.
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/s/") {
			token := strings.TrimPrefix(r.URL.Path, "/s/")
			token = strings.Trim(token, "/")
			if token != "" && !strings.Contains(token, "/") {
				s.publicShareDownload(w, r, token)
				return
			}
		}
		s.web.ServeHTTP(w, r)
	})
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
	parts := []string{}
	if path != "" {
		parts = strings.Split(path, "/")
	}

	// Публичные маршруты.
	switch {
	case r.Method == http.MethodGet && path == "health":
		s.health(w, r)
		return
	case r.Method == http.MethodGet && path == "auth/csrf":
		s.authCSRF(w, r)
		return
	case r.Method == http.MethodGet && path == "auth/me":
		s.authMe(w, r)
		return
	case r.Method == http.MethodPost && path == "auth/login":
		s.authLogin(w, r)
		return
	case r.Method == http.MethodGet && path == "youtube/callback":
		// OAuth redirect: session обязательна, CSRF не требуется (top-level GET).
		if !s.requireAuthSession(w, r) {
			return
		}
		s.youtubeAuthCallback(w, r)
		return
	}

	if path == "auth/logout" && r.Method == http.MethodPost {
		s.authLogout(w, r)
		return
	}

	if !s.requireAuth(w, r) {
		return
	}

	if s.handleDrive(w, r, path, parts) {
		return
	}

	switch {
	case r.Method == http.MethodGet && path == "system":
		s.healthDetailed(w, r)
	case r.Method == http.MethodGet && path == "youtube":
		s.youtubeStatus(w, r)
	case r.Method == http.MethodGet && path == "youtube/auth/start":
		s.youtubeAuthStart(w, r)
	case r.Method == http.MethodPost && path == "youtube/disconnect":
		s.youtubeDisconnect(w, r)
	case r.Method == http.MethodPost && path == "files":
		s.upload(w, r)
	case r.Method == http.MethodGet && path == "files":
		s.listFiles(w, r)
	case r.Method == http.MethodGet && len(parts) == 2 && parts[0] == "files":
		s.getFile(w, r, parts[1])
	case r.Method == http.MethodPost && len(parts) == 3 && parts[0] == "files" && parts[2] == "encode":
		s.encode(w, r, parts[1])
	case r.Method == http.MethodPost && len(parts) == 3 && parts[0] == "files" && parts[2] == "youtube":
		s.youtubeUpload(w, r, parts[1])
	case r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "files" && parts[2] == "youtube":
		s.youtubeFileStatus(w, r, parts[1])
	case r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "files" && parts[2] == "download":
		s.downloadOriginal(w, r, parts[1])
	case r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "files" && parts[2] == "video":
		s.downloadVideo(w, r, parts[1])
	case r.Method == http.MethodPost && path == "recover":
		s.recover(w, r)
	case r.Method == http.MethodGet && path == "jobs":
		s.listJobs(w, r)
	case r.Method == http.MethodGet && len(parts) == 2 && parts[0] == "jobs":
		s.getJob(w, r, parts[1])
	case r.Method == http.MethodPost && len(parts) == 3 && parts[0] == "jobs" && parts[2] == "cancel":
		s.cancelJob(w, r, parts[1])
	case r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "jobs" && parts[2] == "download":
		s.downloadJob(w, r, parts[1])
	default:
		writeError(w, http.StatusNotFound, "маршрут не найден")
	}
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{
		"status":  "ok",
		"version": version,
		"app":     s.cfg.AppName,
	}
	// Подробности FFmpeg — только для авторизованной диагностики через settings.
	if !s.cfg.AuthEnabled || s.cfg.AppEnv != config.EnvProduction {
		runner := ffmpeg.New(s.cfg.FFmpeg, s.cfg.FFProbe)
		ok := runner.Available() == nil
		out["ffmpeg"] = ok
		out["ffmpeg_source"] = s.cfg.FFmpegSource
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) healthDetailed(w http.ResponseWriter, _ *http.Request) {
	runner := ffmpeg.New(s.cfg.FFmpeg, s.cfg.FFProbe)
	ok := runner.Available() == nil
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"version":       version,
		"app":           s.cfg.AppName,
		"ffmpeg":        ok,
		"ffmpeg_path":   s.cfg.FFmpeg,
		"ffprobe_path":  s.cfg.FFProbe,
		"ffmpeg_source": s.cfg.FFmpegSource,
		"ffmpeg_info":   ffmpeg.Diagnose(s.cfg.FFmpeg, s.cfg.FFProbe),
	})
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUpload+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		if isTooLarge(err) {
			writeError(w, http.StatusRequestEntityTooLarge, "файл слишком большой")
			return
		}
		writeError(w, http.StatusBadRequest, "ожидается multipart-поле file")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "поле file обязательно")
		return
	}
	defer file.Close()
	name := storage.SanitizeName(hdr.Filename)
	rec, err := s.store.SaveUpload(file, name, storage.DetectMIME(name), s.cfg.MaxUpload)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) listFiles(w http.ResponseWriter, _ *http.Request) {
	files, err := s.store.ListFiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, files)
}

func (s *Server) getFile(w http.ResponseWriter, _ *http.Request, id string) {
	f, err := s.store.GetFile(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) encode(w http.ResponseWriter, _ *http.Request, id string) {
	job, err := s.jobs.EnqueueEncode(id)
	if err != nil {
		writeJobError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) downloadOriginal(w http.ResponseWriter, r *http.Request, id string) {
	f, err := s.store.GetFile(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.serveStored(w, r, f.StoredPath, f.OriginalName)
}

func (s *Server) downloadVideo(w http.ResponseWriter, r *http.Request, id string) {
	f, err := s.store.GetFile(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if f.VideoPath == "" {
		writeError(w, http.StatusNotFound, "видео ещё не создано")
		return
	}
	s.serveStored(w, r, f.VideoPath, strings.TrimSuffix(f.OriginalName, filepath.Ext(f.OriginalName))+".mp4")
}

func (s *Server) recover(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUpload+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		if isTooLarge(err) {
			writeError(w, http.StatusRequestEntityTooLarge, "файл слишком большой")
			return
		}
		writeError(w, http.StatusBadRequest, "ожидается multipart-поле file")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "поле file обязательно")
		return
	}
	defer file.Close()
	job, err := s.store.CreateJob(storage.KindDecode, "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dir, err := s.store.CreateJobDir(job.ID)
	if err != nil {
		_ = s.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", err.Error())
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dest := filepath.Join(dir, "input.mp4")
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		_ = s.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", err.Error())
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, copyErr := io.Copy(out, io.LimitReader(file, s.cfg.MaxUpload+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || n > s.cfg.MaxUpload {
		_ = os.RemoveAll(dir)
		msg := "не удалось сохранить видео"
		code := http.StatusBadRequest
		if n > s.cfg.MaxUpload {
			msg = "файл слишком большой"
			code = http.StatusRequestEntityTooLarge
		}
		_ = s.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", msg)
		writeError(w, code, msg)
		return
	}
	if err := s.store.SetJobInput(job.ID, dest); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	job.InputPath = dest
	job.Message = storage.SanitizeName(hdr.Filename)
	if err := s.jobs.EnqueuePrepared(job); err != nil {
		_ = s.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", err.Error())
		writeJobError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) listJobs(w http.ResponseWriter, _ *http.Request) {
	list, err := s.store.ListJobs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getJob(w http.ResponseWriter, _ *http.Request, id string) {
	job, err := s.store.GetJob(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) cancelJob(w http.ResponseWriter, _ *http.Request, id string) {
	if err := s.jobs.Cancel(id); err != nil {
		switch {
		case errors.Is(err, jobs.ErrNotCancellable):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, storage.ErrNotFound), errors.Is(err, storage.ErrInvalidID):
			writeError(w, http.StatusNotFound, "не найдено")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	job, err := s.store.GetJob(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) downloadJob(w http.ResponseWriter, r *http.Request, id string) {
	job, err := s.store.GetJob(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if job.Status != storage.StatusSucceeded || job.OutputPath == "" {
		writeError(w, http.StatusNotFound, "результат ещё не готов")
		return
	}
	name := job.OutputName
	if name == "" {
		name = "restored.bin"
	}
	s.serveStored(w, r, job.OutputPath, name)
}

func (s *Server) serveStored(w http.ResponseWriter, r *http.Request, path, name string) {
	if path == "" {
		writeError(w, http.StatusNotFound, "файл не найден")
		return
	}
	clean := filepath.Clean(path)
	if !s.store.ContainsPath(clean) {
		writeError(w, http.StatusNotFound, "файл не найден")
		return
	}
	if _, err := os.Stat(clean); err != nil {
		writeError(w, http.StatusNotFound, "файл не найден")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", storage.SanitizeName(name)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, clean)
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrNotFound), errors.Is(err, storage.ErrInvalidID):
		writeError(w, http.StatusNotFound, "не найдено")
	case errors.Is(err, storage.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeJobError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrNotFound), errors.Is(err, storage.ErrInvalidID):
		writeError(w, http.StatusNotFound, "файл не найден")
	case errors.Is(err, jobs.ErrBusy), errors.Is(err, jobs.ErrNotCancellable):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, jobs.ErrQueueFull):
		writeError(w, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, jobs.ErrStopped):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func isTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr) || strings.Contains(strings.ToLower(err.Error()), "too large")
}
