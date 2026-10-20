// Package jobs выполняет кодирование и восстановление в ограниченном пуле воркеров.
// Число одновременных процессов FFmpeg равно числу воркеров.
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
)

var (
	// ErrBusy — для файла уже есть задание в очереди или в работе.
	ErrBusy = errors.New("для этого файла уже есть активное задание")
	// ErrQueueFull — очередь не принимает новые задания.
	ErrQueueFull = errors.New("очередь заданий заполнена")
	// ErrStopped — сервер завершает работу.
	ErrStopped = errors.New("сервер останавливается")
)

// Manager — очередь заданий и пул воркеров.
type Manager struct {
	store     *storage.Store
	opt       codec.Options
	queue     chan storage.Job
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.Mutex
	active    map[string]struct{}
	workers   int
	startOnce sync.Once
	stopOnce  sync.Once
}

// New создаёт менеджер. Start запускает воркеры, Stop дожидается текущих задач.
func New(store *storage.Store, opt codec.Options, workers, queueSize int) *Manager {
	if workers < 1 {
		workers = 1
	}
	if queueSize < 1 {
		queueSize = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		store:   store,
		opt:     opt,
		queue:   make(chan storage.Job, queueSize),
		ctx:     ctx,
		cancel:  cancel,
		active:  map[string]struct{}{},
		workers: workers,
	}
}

// Start поднимает воркеры. Повторный вызов ничего не делает.
func (m *Manager) Start() {
	m.startOnce.Do(func() {
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
				_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", "остановлено")
				m.clear(job.FileID)
			default:
				return
			}
		}
	})
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
		if job.Kind == storage.KindEncode {
			m.clear(job.FileID)
		}
		if rec := recover(); rec != nil {
			_ = m.store.UpdateJob(job.ID, storage.StatusFailed, job.Progress, "", fmt.Sprintf("внутренняя ошибка: %v", rec))
		}
	}()
	if err := m.ctx.Err(); err != nil {
		_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", "остановлено")
		return
	}
	_ = m.store.UpdateJob(job.ID, storage.StatusRunning, 0, "выполняется", "")
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
	switch job.Kind {
	case storage.KindEncode:
		m.runEncode(job, progress)
	case storage.KindDecode:
		m.runDecode(job, progress)
	default:
		_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", "неизвестный тип задания")
	}
}

func (m *Manager) runEncode(job storage.Job, progress codec.ProgressFunc) {
	out := filepath.Join(m.store.Root(), "objects", job.FileID, "video.mp4")
	opt := m.opt
	if file, err := m.store.GetFile(job.FileID); err == nil && file.OriginalName != "" {
		opt.DisplayName = file.OriginalName
	}
	res, err := codec.EncodeFile(m.ctx, job.InputPath, out, opt, progress)
	if err != nil {
		_ = os.Remove(out)
		_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", err.Error())
		return
	}
	if err := m.store.SetVideo(job.FileID, out); err != nil {
		_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", err.Error())
		return
	}
	_ = m.store.SetJobOutput(job.ID, out, videoDownloadName(opt.DisplayName))
	_ = m.store.UpdateJob(job.ID, storage.StatusSucceeded, 100, fmt.Sprintf("готово, кадров: %d, sha256 %s", res.Frames, res.SHA256Hex), "")
}

func (m *Manager) runDecode(job storage.Job, progress codec.ProgressFunc) {
	out := filepath.Join(filepath.Dir(job.InputPath), "restored")
	res, err := codec.DecodeFile(m.ctx, job.InputPath, out, codec.Options{FFmpeg: m.opt.FFmpeg, FFProbe: m.opt.FFProbe}, progress)
	if err != nil {
		_ = os.Remove(out)
		_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", err.Error())
		return
	}
	if err := m.store.SetJobOutput(job.ID, out, res.Name); err != nil {
		_ = m.store.UpdateJob(job.ID, storage.StatusFailed, 0, "", err.Error())
		return
	}
	_ = m.store.UpdateJob(job.ID, storage.StatusSucceeded, 100, fmt.Sprintf("восстановлен %s, sha256 %s", res.Name, res.SHA256Hex), "")
}

func videoDownloadName(display string) string {
	base := strings.TrimSuffix(filepath.Base(display), filepath.Ext(display))
	base = strings.Trim(base, ". ")
	if base == "" || base == "." || base == ".." {
		return "video.mp4"
	}
	return base + ".mp4"
}
