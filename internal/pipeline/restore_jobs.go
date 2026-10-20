package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"ytdrive/internal/retrieval"
)

// RestoreJobStatus — состояние асинхронного восстановления для UI.
type RestoreJobStatus struct {
	ID        string `json:"id"`
	FileID    string `json:"file_id"`
	State     string `json:"state"` // queued|preparing|restoring|done|error
	Stage     string `json:"stage"`
	Percent   int    `json:"percent"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
	FileName  string `json:"file_name,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type restoreJob struct {
	RestoreJobStatus
	data []byte
	mu   sync.Mutex
}

// RestoreHub хранит одну активную операцию восстановления (лимит эксперимента).
type RestoreHub struct {
	mu     sync.Mutex
	active string
	jobs   map[string]*restoreJob
}

func NewRestoreHub() *RestoreHub {
	return &RestoreHub{jobs: map[string]*restoreJob{}}
}

func (h *RestoreHub) Start(fileID string) (RestoreJobStatus, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active != "" {
		if j, ok := h.jobs[h.active]; ok {
			j.mu.Lock()
			st := j.State
			j.mu.Unlock()
			if st == "queued" || st == "preparing" || st == "restoring" {
				return RestoreJobStatus{}, errors.New("уже выполняется другая операция восстановления")
			}
		}
	}
	id := newJobID()
	now := time.Now().UTC().Format(time.RFC3339)
	j := &restoreJob{RestoreJobStatus: RestoreJobStatus{
		ID: id, FileID: fileID, State: "queued", Stage: "queued", Percent: 0,
		Message: "В очереди", CreatedAt: now, UpdatedAt: now,
	}}
	h.jobs[id] = j
	h.active = id
	return j.RestoreJobStatus, nil
}

func (h *RestoreHub) Get(id string) (RestoreJobStatus, bool) {
	h.mu.Lock()
	j, ok := h.jobs[id]
	h.mu.Unlock()
	if !ok {
		return RestoreJobStatus{}, false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.RestoreJobStatus, true
}

func (h *RestoreHub) TakeResult(id string) (name string, data []byte, err error) {
	h.mu.Lock()
	j, ok := h.jobs[id]
	h.mu.Unlock()
	if !ok {
		return "", nil, errors.New("задание не найдено")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.State != "done" {
		return "", nil, errors.New("результат ещё не готов")
	}
	out := append([]byte(nil), j.data...)
	j.data = nil
	return j.FileName, out, nil
}

func (h *RestoreHub) update(id, state, stage string, pct int, message, errMsg string) {
	h.mu.Lock()
	j := h.jobs[id]
	h.mu.Unlock()
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.State = state
	j.Stage = stage
	j.Percent = pct
	j.Message = message
	j.Error = errMsg
	j.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
}

func (h *RestoreHub) finishOK(id, name string, data []byte) {
	h.mu.Lock()
	j := h.jobs[id]
	if h.active == id {
		h.active = ""
	}
	h.mu.Unlock()
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.State = "done"
	j.Stage = "done"
	j.Percent = 100
	j.Message = "Готово"
	j.FileName = name
	j.data = data
	j.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
}

func (h *RestoreHub) finishErr(id string, err error) {
	h.mu.Lock()
	j := h.jobs[id]
	if h.active == id {
		h.active = ""
	}
	h.mu.Unlock()
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.State = "error"
	j.Stage = "error"
	j.Message = "Ошибка"
	j.Error = err.Error()
	j.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
}

// RunRestoreJob выполняет Download в фоне с обновлением прогресса.
func (s *Service) RunRestoreJob(ctx context.Context, hub *RestoreHub, jobID, fileID string) {
	if hub == nil || s == nil {
		return
	}
	progress := func(stage string, pct int, message string) {
		state := "preparing"
		if pct >= 75 {
			state = "restoring"
		}
		userMsg := userFacingProgress(stage, message)
		hub.update(jobID, state, stage, pct, userMsg, "")
	}

	if pw, ok := s.retriever.(*retrieval.Playwright); ok {
		pw.OnProgress = progress
		defer func() { pw.OnProgress = nil }()
	}

	hub.update(jobID, "preparing", "retrieve", 15, "Подготовка файла…", "")
	name, data, err := s.DownloadWithProgress(ctx, fileID, progress)
	if err != nil {
		hub.finishErr(jobID, err)
		return
	}
	hub.finishOK(jobID, name, data)
}

func userFacingProgress(stage, message string) string {
	switch stage {
	case "launch", "open_site", "submit_url", "wait_formats", "download", "retrieve":
		if message != "" {
			return message
		}
		return "Подготовка файла…"
	case "probe", "decode", "decrypt", "verify":
		return "Восстановление…"
	case "done":
		return "Готово"
	default:
		if message != "" {
			return message
		}
		return "Обработка…"
	}
}

func newJobID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
