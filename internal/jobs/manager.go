// Package jobs выполняет кодирование, восстановление и загрузку на YouTube в пуле воркеров.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"ytdrive/internal/codec"
	"ytdrive/internal/storage"
	"ytdrive/internal/youtube"
)

var (
	// ErrBusy — для файла уже есть задание в очереди или в работе.
	ErrBusy = errors.New("для этого файла уже есть активное задание")
	// ErrQueueFull — очередь не принимает новые задания.
	ErrQueueFull = errors.New("очередь заданий заполнена")
	// ErrStopped — сервер завершает работу.
	ErrStopped = errors.New("сервер останавливается")
	// ErrNotCancellable — задание уже завершено или неизвестно.
	ErrNotCancellable = errors.New("задание нельзя отменить")
	// ErrNoVideo — нет локального MP4 для загрузки.
	ErrNoVideo = errors.New("сначала закодируйте файл в MP4")
)

// Manager — очередь заданий и пул воркеров.
type Manager struct {
	store     *storage.Store
	opt       codec.Options
	yt        youtube.VideoStorageProvider
	ytPrivacy string // default private; experimental public via config
	queue     chan storage.Job
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.Mutex
	active    map[string]struct{}
	cancels   map[string]context.CancelFunc
	cancelled map[string]struct{}
	workers   int
	startOnce sync.Once
	stopOnce  sync.Once
}

// New создаёт менеджер. yt может быть Unconfigured.
func New(store *storage.Store, opt codec.Options, workers, queueSize int, yt youtube.VideoStorageProvider) *Manager {
	return NewWithPrivacy(store, opt, workers, queueSize, yt, "private")
}

// NewWithPrivacy задаёт privacyStatus для YouTube upload (private|unlisted|public).
func NewWithPrivacy(store *storage.Store, opt codec.Options, workers, queueSize int, yt youtube.VideoStorageProvider, privacy string) *Manager {
	if workers < 1 {
		workers = 1
	}
	if queueSize < 1 {
		queueSize = 1
	}
	if yt == nil {
		yt = youtube.Unconfigured{}
	}
	if privacy == "" {
		privacy = "private"
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		store:     store,
		opt:       opt,
		yt:        yt,
		ytPrivacy: privacy,
		queue:     make(chan storage.Job, queueSize),
		ctx:       ctx,
		cancel:    cancel,
		active:    map[string]struct{}{},
		cancels:   map[string]context.CancelFunc{},
		cancelled: map[string]struct{}{},
		workers:   workers,
	}
}

// Start поднимает воркеры и помечает прерванные задания.
func (m *Manager) Start() {
	m.startOnce.Do(func() {
		_, _ = m.store.FailInterruptedJobs()
		for i := 0; i < m.workers; i++ {
			m.wg.Add(1)
			go m.loop()
		}
	})
}

// Stop отменяет контекст, дожидается воркеров и помечает невыполненные задания.
func (m *Manager) Stop() {
	m.stopOnce.Do(func() {
		m.cancel()
		m.wg.Wait()
		for {
			select {
			case job := <-m.queue:
				_ = m.store.UpdateJob(job.ID, storage.StatusCancelled, 0, "", "остановлено")
				m.clear(job.FileID)
			default:
				return
			}
		}
	})
}

// Cancel отменяет задание в очереди или выполняющееся задание (через context).
func (m *Manager) Cancel(jobID string) error {
	if !storage.ValidID(jobID) {
		return storage.ErrInvalidID
	}
	job, err := m.store.GetJob(jobID)
	if err != nil {
		return err
	}
	switch job.Status {
	case storage.StatusSucceeded, storage.StatusFailed, storage.StatusCancelled, storage.StatusCompleted:
		return ErrNotCancellable
	}
	m.mu.Lock()
	m.cancelled[jobID] = struct{}{}
	if cancel, ok := m.cancels[jobID]; ok {
		cancel()
	}
	m.mu.Unlock()
	if job.Status == storage.StatusQueued {
		_ = m.store.UpdateJob(jobID, storage.StatusCancelled, job.Progress, "", "отменено")
		m.clear(job.FileID)
	}
	return nil
}

// EnqueueEncode ставит кодирование файла в очередь.
func (m *Manager) EnqueueEncode(fileID string) (storage.Job, error) {
	f, err := m.store.GetFile(fileID)
	if err != nil {
		return storage.Job{}, err
	}
	if err := m.reserve(fileID); err != nil {
		return storage.Job{}, err
	}
	job, err := m.store.CreateJob(storage.KindEncode, f.ID, f.StoredPath)
	if err != nil {
		m.clear(fileID)
		return storage.Job{}, err
	}
	if err := m.push(job); err != nil {
		m.clear(fileID)
		_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", err.Error())
		return storage.Job{}, err
	}
	return job, nil
}

// EnqueueYouTubeUpload ставит загрузку готового MP4 на YouTube.
func (m *Manager) EnqueueYouTubeUpload(fileID string) (storage.Job, error) {
	if !m.yt.Configured() {
		return storage.Job{}, youtube.ErrNotConfigured
	}
	if !m.yt.Connected(m.ctx) {
		return storage.Job{}, youtube.ErrNotConnected
	}
	f, err := m.store.GetFile(fileID)
	if err != nil {
		return storage.Job{}, err
	}
	if f.VideoPath == "" {
		return storage.Job{}, ErrNoVideo
	}
	if f.YouTubeVideoID != "" {
		return storage.Job{}, youtube.ErrAlreadyUploaded
	}
	if err := m.reserve(fileID); err != nil {
		return storage.Job{}, err
	}
	job, err := m.store.CreateJob(storage.KindYouTubeUpload, f.ID, f.VideoPath)
	if err != nil {
		m.clear(fileID)
		return storage.Job{}, err
	}
	if err := m.push(job); err != nil {
		m.clear(fileID)
		_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", err.Error())
		return storage.Job{}, err
	}
	return job, nil
}

// EnqueuePrepared ставит уже созданное задание в очередь.
func (m *Manager) EnqueuePrepared(job storage.Job) error {
	if err := m.ctx.Err(); err != nil {
		return ErrStopped
	}
	return m.push(job)
}

// EnqueueDecode ставит восстановление уже сохранённого MP4.
func (m *Manager) EnqueueDecode(inputPath string) (storage.Job, error) {
	job, err := m.store.CreateJob(storage.KindDecode, "", inputPath)
	if err != nil {
		return storage.Job{}, err
	}
	if err := m.push(job); err != nil {
		_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", err.Error())
		return storage.Job{}, err
	}
	return job, nil
}

func (m *Manager) reserve(fileID string) error {
	if err := m.ctx.Err(); err != nil {
		return ErrStopped
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.active[fileID]; ok {
		return ErrBusy
	}
	m.active[fileID] = struct{}{}
	return nil
}

func (m *Manager) clear(fileID string) {
	if fileID == "" {
		return
	}
	m.mu.Lock()
	delete(m.active, fileID)
	m.mu.Unlock()
}

func (m *Manager) push(job storage.Job) error {
	select {
	case <-m.ctx.Done():
		return ErrStopped
	case m.queue <- job:
		return nil
	default:
		return ErrQueueFull
	}
}

func (m *Manager) loop() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case job := <-m.queue:
			m.process(job)
		}
	}
}

func (m *Manager) process(job storage.Job) {
	defer func() {
		if job.Kind == storage.KindEncode || job.Kind == storage.KindYouTubeUpload {
			m.clear(job.FileID)
		}
		m.mu.Lock()
		delete(m.cancels, job.ID)
		delete(m.cancelled, job.ID)
		m.mu.Unlock()
		if rec := recover(); rec != nil {
			_ = m.store.UpdateJob(job.ID, storage.StatusFailed, job.Progress, "", fmt.Sprintf("внутренняя ошибка: %v", rec))
		}
	}()
	if err := m.ctx.Err(); err != nil {
		_ = m.store.UpdateJob(job.ID, storage.StatusCancelled, 0, "", "остановлено")
		return
	}
	m.mu.Lock()
	if _, stop := m.cancelled[job.ID]; stop {
		m.mu.Unlock()
		_ = m.store.UpdateJob(job.ID, storage.StatusCancelled, 0, "", "отменено")
		return
	}
	jobCtx, cancel := context.WithCancel(m.ctx)
	m.cancels[job.ID] = cancel
	m.mu.Unlock()
	defer cancel()

	switch job.Kind {
	case storage.KindEncode:
		_ = m.store.UpdateJob(job.ID, storage.StatusRunning, 0, "кодирование", "")
		m.runEncode(jobCtx, job)
	case storage.KindDecode:
		_ = m.store.UpdateJob(job.ID, storage.StatusRunning, 0, "восстановление", "")
		m.runDecode(jobCtx, job)
	case storage.KindYouTubeUpload:
		_ = m.store.UpdateJob(job.ID, storage.StatusUploading, 0, "загрузка на YouTube", "")
		m.runYouTubeUpload(jobCtx, job)
	default:
		_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", "неизвестный тип задания")
	}
}

func (m *Manager) runEncode(ctx context.Context, job storage.Job) {
	out := filepath.Join(m.store.Root(), "objects", job.FileID, "video.mp4")
	opt := m.opt
	if file, err := m.store.GetFile(job.FileID); err == nil && file.OriginalName != "" {
		opt.DisplayName = file.OriginalName
	}
	last := -1
	progress := func(done, total int) {
		pct := 0
		if total > 0 {
			pct = done * 100 / total
		}
		if pct < 100 && pct == last {
			return
		}
		last = pct
		_ = m.store.UpdateJob(job.ID, storage.StatusRunning, pct, fmt.Sprintf("кадры %d из %d", done, total), "")
	}
	res, err := codec.EncodeFile(ctx, job.InputPath, out, opt, progress)
	if err != nil {
		_ = os.Remove(out)
		status, msg := failStatus(ctx, err)
		_ = m.store.UpdateJob(job.ID, status, 0, "", msg)
		return
	}
	if err := m.store.SetVideo(job.FileID, out); err != nil {
		_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", err.Error())
		return
	}
	_ = m.store.SetJobOutput(job.ID, out, videoDownloadName(opt.DisplayName))
	_ = m.store.UpdateJob(job.ID, storage.StatusSucceeded, 100, fmt.Sprintf("готово, кадров: %d, sha256 %s", res.Frames, res.SHA256Hex), "")
}

func (m *Manager) runDecode(ctx context.Context, job storage.Job) {
	out := filepath.Join(filepath.Dir(job.InputPath), "restored")
	last := -1
	progress := func(done, total int) {
		pct := 0
		if total > 0 {
			pct = done * 100 / total
		}
		if pct < 100 && pct == last {
			return
		}
		last = pct
		_ = m.store.UpdateJob(job.ID, storage.StatusRunning, pct, fmt.Sprintf("кадры %d из %d", done, total), "")
	}
	res, err := codec.DecodeFile(ctx, job.InputPath, out, codec.Options{FFmpeg: m.opt.FFmpeg, FFProbe: m.opt.FFProbe}, progress)
	if err != nil {
		_ = os.Remove(out)
		status, msg := failStatus(ctx, err)
		_ = m.store.UpdateJob(job.ID, status, 0, "", msg)
		return
	}
	if err := m.store.SetJobOutput(job.ID, out, res.Name); err != nil {
		_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", err.Error())
		return
	}
	_ = m.store.UpdateJob(job.ID, storage.StatusSucceeded, 100, fmt.Sprintf("восстановлен %s, sha256 %s", res.Name, res.SHA256Hex), "")
}

func (m *Manager) runYouTubeUpload(ctx context.Context, job storage.Job) {
	file, err := m.store.GetFile(job.FileID)
	if err != nil {
		_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", err.Error())
		return
	}
	if file.YouTubeVideoID != "" {
		_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", youtube.ErrAlreadyUploaded.Error())
		return
	}
	path := job.InputPath
	if path == "" {
		path = file.VideoPath
	}
	title := strings.TrimSuffix(file.OriginalName, filepath.Ext(file.OriginalName))
	if title == "" {
		title = file.OriginalName
	}
	last := -1
	privacy := m.ytPrivacy
	if privacy == "" {
		privacy = "private"
	}
	opt := youtube.UploadOptions{
		Title:       title,
		Description: "Загружено через A-Tex InfiDrive. SHA-256: " + file.SHA256,
		Privacy:     privacy,
		Progress: func(sent, total int64) {
			pct := 0
			if total > 0 {
				pct = int(sent * 100 / total)
			}
			if pct > 99 {
				pct = 99
			}
			if pct == last {
				return
			}
			last = pct
			_ = m.store.UpdateJob(job.ID, storage.StatusUploading, pct, fmt.Sprintf("загрузка %d%%", pct), "")
		},
	}
	ref, err := m.yt.UploadVideo(ctx, path, opt)
	if err != nil {
		_ = m.store.SetYouTube(job.FileID, storage.YouTubeMeta{
			UploadStatus: storage.StatusFailed,
			Error:        publicYouTubeErr(err),
		})
		status, msg := failStatus(ctx, err)
		_ = m.store.UpdateJob(job.ID, status, 0, "", msg)
		return
	}
	// Сразу фиксируем video_id, чтобы повторная публикация была невозможна.
	meta := storage.YouTubeMeta{
		VideoID:          ref.VideoID,
		URL:              ref.URL,
		UploadStatus:     storage.StatusUploaded,
		ProcessingStatus: ref.ProcessingStatus,
		UploadedAt:       storage.NowUTC(),
		PrivacyStatus:    ref.PrivacyStatus,
	}
	if meta.ProcessingStatus == "" {
		meta.ProcessingStatus = "uploaded"
	}
	_ = m.store.SetYouTube(job.FileID, meta)
	_ = m.store.UpdateJob(job.ID, storage.StatusUploaded, 100, "загружено, ожидание обработки", "")

	_ = m.store.UpdateJob(job.ID, storage.StatusProcessing, 100, "проверка статуса обработки", "")
	st, err := m.yt.GetVideoStatus(ctx, ref.VideoID)
	if err == nil {
		meta.UploadStatus = st.UploadStatus
		if meta.UploadStatus == "" {
			meta.UploadStatus = storage.StatusUploaded
		}
		meta.ProcessingStatus = st.ProcessingStatus
		if st.PrivacyStatus != "" {
			meta.PrivacyStatus = st.PrivacyStatus
		}
		_ = m.store.SetYouTube(job.FileID, meta)
	}
	_ = m.store.UpdateJob(job.ID, storage.StatusCompleted, 100,
		fmt.Sprintf("YouTube %s (%s)", ref.VideoID, meta.PrivacyStatus), "")
}

func failStatus(ctx context.Context, err error) (string, string) {
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		return storage.StatusCancelled, "отменено"
	}
	return storage.StatusFailed, publicYouTubeErr(err)
}

func publicYouTubeErr(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, youtube.ErrNotConfigured):
		return youtube.ErrNotConfigured.Error()
	case errors.Is(err, youtube.ErrNotConnected):
		return youtube.ErrNotConnected.Error()
	case errors.Is(err, youtube.ErrQuota):
		return "квота YouTube API исчерпана или ограничена"
	case errors.Is(err, youtube.ErrAuth):
		return "ошибка авторизации YouTube — подключите аккаунт снова"
	case errors.Is(err, youtube.ErrAlreadyUploaded):
		return youtube.ErrAlreadyUploaded.Error()
	case errors.Is(err, youtube.ErrTemporary):
		return "временная ошибка YouTube API"
	default:
		// Не пробрасываем сырой ответ с возможными токенами.
		msg := err.Error()
		if strings.Contains(strings.ToLower(msg), "token") {
			return "ошибка YouTube API"
		}
		if len(msg) > 300 {
			return msg[:300]
		}
		return msg
	}
}

func videoDownloadName(display string) string {
	base := strings.TrimSuffix(filepath.Base(display), filepath.Ext(display))
	base = strings.Trim(base, ". ")
	if base == "" || base == "." || base == ".." {
		return "video.mp4"
	}
	return base + ".mp4"
}
