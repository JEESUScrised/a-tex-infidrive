package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"ytdrive/internal/blobstore"
	"ytdrive/internal/drive"
	"ytdrive/internal/storage"
)

func (s *Server) handleDrive(w http.ResponseWriter, r *http.Request, path string, parts []string) bool {
	if s.drive == nil {
		return false
	}
	switch {
	case r.Method == http.MethodGet && path == "drive":
		folderID := r.URL.Query().Get("folder_id")
		listing, err := s.drive.List(folderID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return true
		}
		writeJSON(w, http.StatusOK, listing)
		return true
	case r.Method == http.MethodPost && path == "drive/folders":
		var body struct {
			ParentID string `json:"parent_id"`
			Name     string `json:"name"`
		}
		if err := readJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "некорректный JSON")
			return true
		}
		f, err := s.drive.CreateFolder(body.ParentID, body.Name)
		if err != nil {
			writeDriveError(w, err)
			return true
		}
		writeJSON(w, http.StatusCreated, f)
		return true
	case r.Method == http.MethodPost && path == "drive/upload":
		s.driveUpload(w, r)
		return true
	case r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "drive" && parts[1] == "files" && parts[2] != "" && !isDriveAction(parts):
		// reserved
		return false
	case r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "drive" && parts[2] == "download":
		s.driveDownload(w, r, parts[1])
		return true
	case r.Method == http.MethodPost && len(parts) == 3 && parts[0] == "drive" && parts[2] == "share":
		s.driveShare(w, r, parts[1])
		return true
	case r.Method == http.MethodDelete && len(parts) == 2 && parts[0] == "drive":
		if err := s.drive.SoftDelete(parts[1]); err != nil {
			writeDriveError(w, err)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "trashed"})
		return true
	case r.Method == http.MethodGet && path == "drive/trash":
		list, err := s.drive.Trash()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return true
		}
		writeJSON(w, http.StatusOK, list)
		return true
	case r.Method == http.MethodPost && len(parts) == 3 && parts[0] == "drive" && parts[1] == "trash" && parts[2] != "":
		// restore: POST /api/drive/trash/{id}/restore handled below
		return false
	case r.Method == http.MethodPost && len(parts) == 4 && parts[0] == "drive" && parts[1] == "trash" && parts[3] == "restore":
		if err := s.drive.Restore(parts[2]); err != nil {
			writeDriveError(w, err)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
		return true
	case r.Method == http.MethodDelete && len(parts) == 3 && parts[0] == "drive" && parts[1] == "trash":
		if err := s.drive.Purge(r.Context(), parts[2]); err != nil {
			writeDriveError(w, err)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "purged"})
		return true
	case r.Method == http.MethodGet && path == "drive/shares":
		list, err := s.drive.ListShares()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return true
		}
		writeJSON(w, http.StatusOK, list)
		return true
	case r.Method == http.MethodDelete && len(parts) == 3 && parts[0] == "drive" && parts[1] == "shares":
		if err := s.drive.RevokeShare(parts[2]); err != nil {
			writeDriveError(w, err)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
		return true
	case r.Method == http.MethodGet && path == "drive/provider":
		out := map[string]any{
			"provider": s.drive.ProviderName(),
			"ok":       false,
		}
		if err := s.drive.ProviderAvailable(r.Context()); err != nil {
			out["error"] = err.Error()
		} else {
			out["ok"] = true
		}
		if n, err := s.drive.Usage(); err == nil {
			out["usage_bytes"] = n
		}
		writeJSON(w, http.StatusOK, out)
		return true
	}
	return false
}

func isDriveAction(parts []string) bool {
	return false
}

func (s *Server) driveUpload(w http.ResponseWriter, r *http.Request) {
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
	folderID := r.FormValue("folder_id")
	mime := storage.DetectMIME(hdr.Filename)
	rec, err := s.drive.UploadStream(r.Context(), folderID, hdr.Filename, mime, file, hdr.Size)
	if err != nil {
		writeDriveError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) driveDownload(w http.ResponseWriter, r *http.Request, id string) {
	rc, f, err := s.drive.Download(r.Context(), id)
	if err != nil {
		writeDriveError(w, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", f.MIME)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", storage.SanitizeName(f.Name)))
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func (s *Server) driveShare(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		TTLHours     int `json:"ttl_hours"`
		MaxDownloads int `json:"max_downloads"`
	}
	_ = readJSON(r, &body)
	var ttl time.Duration
	if body.TTLHours > 0 {
		ttl = time.Duration(body.TTLHours) * time.Hour
	}
	link, err := s.drive.CreateShare(id, ttl, body.MaxDownloads)
	if err != nil {
		writeDriveError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

func (s *Server) publicShareMeta(w http.ResponseWriter, r *http.Request, token string) {
	if s.drive == nil {
		writeError(w, http.StatusNotFound, "не найдено")
		return
	}
	link, err := s.drive.PeekShareMeta(token)
	if err != nil {
		writeDriveError(w, err)
		return
	}
	view := drive.PublicShareView{
		FileName: link.FileName,
		FileSize: link.FileSize,
		ExpiresAt: link.ExpiresAt,
	}
	if s.pipeline != nil && s.cfg.ExperimentalYouTubeStore {
		meta := s.pipeline.RetrieverMeta(r.Context())
		view.DownloadAvailable = meta.Available && meta.Automatic
		if !view.DownloadAvailable {
			view.RetrievalHint = "Автоматическое восстановление недоступно"
		}
	} else {
		view.DownloadAvailable = true
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) publicShareDownload(w http.ResponseWriter, r *http.Request, token string) {
	if s.pipeline != nil && s.cfg.ExperimentalYouTubeStore {
		link, err := s.drive.OpenShareMeta(token)
		if err != nil {
			writeDriveError(w, err)
			return
		}
		name, data, err := s.pipeline.Download(r.Context(), link.FileID)
		if err != nil {
			writePipelineError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", storage.SanitizeName(name)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}
	if s.drive == nil {
		writeError(w, http.StatusNotFound, "не найдено")
		return
	}
	rc, f, _, err := s.drive.OpenShare(r.Context(), token)
	if err != nil {
		writeDriveError(w, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", f.MIME)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", storage.SanitizeName(f.Name)))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func writeDriveError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, drive.ErrNotFound), errors.Is(err, blobstore.ErrNotFound):
		writeError(w, http.StatusNotFound, "не найдено")
	case errors.Is(err, drive.ErrDenied):
		writeError(w, http.StatusForbidden, "ссылка недействительна")
	case errors.Is(err, drive.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, storage.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, blobstore.ErrNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "хранилище не настроено")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dst)
}
