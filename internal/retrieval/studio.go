package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrLoginRequired — нужна ручная авторизация в YouTube Studio (persistent profile).
var ErrLoginRequired = errors.New("требуется ручной вход в YouTube Studio")

// ErrAutomationBlocked — Google ограничил автоматизацию; ручной Download сохраняется.
var ErrAutomationBlocked = errors.New("автоматизация Studio ограничена Google")

// Studio — VideoRetriever через официальный Download владельца в YouTube Studio.
// Сессия браузера: изолированный Playwright profile; пароль/2FA только вручную.
type Studio struct {
	NodeBin    string
	WorkerJS   string // browser-retriever/src/studio-cli.mjs
	ProfileDir string // persistent Chromium profile (0600-ish via umask)
	TempDir    string
	Timeout    time.Duration
	Headless   bool
	MaxBytes   int64
	OnProgress ProgressFunc
}

func NewStudio(workerJS, profileDir, tempDir string, timeout time.Duration, maxBytes int64) *Studio {
	if timeout <= 0 {
		timeout = 4 * time.Minute
	}
	if maxBytes <= 0 {
		maxBytes = 256 << 20
	}
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	return &Studio{
		NodeBin:    "node",
		WorkerJS:   workerJS,
		ProfileDir: profileDir,
		TempDir:    tempDir,
		Timeout:    timeout,
		Headless:   true,
		MaxBytes:   maxBytes,
	}
}

func (s *Studio) Name() string { return "playwright_youtube_studio" }

func (s *Studio) Available(ctx context.Context) error {
	if s == nil || strings.TrimSpace(s.WorkerJS) == "" {
		return fmt.Errorf("%w: Studio worker не настроен (STUDIO_RETRIEVER_JS)", ErrUnavailable)
	}
	if _, err := os.Stat(s.WorkerJS); err != nil {
		return fmt.Errorf("%w: Studio worker не найден: %v", ErrUnavailable, err)
	}
	if strings.TrimSpace(s.ProfileDir) == "" {
		return fmt.Errorf("%w: STUDIO_BROWSER_PROFILE не задан", ErrUnavailable)
	}
	// Profile may be empty before first --login; still "configured".
	node := s.NodeBin
	if node == "" {
		node = "node"
	}
	if err := exec.CommandContext(ctx, node, "-e", "process.exit(0)").Run(); err != nil {
		return fmt.Errorf("%w: node недоступен: %v", ErrUnavailable, err)
	}
	return nil
}

func (s *Studio) Diagnose(ctx context.Context) Meta {
	err := s.Available(ctx)
	msg := "Playwright YouTube Studio worker настроен"
	if err != nil {
		msg = err.Error()
	} else if st, e := os.Stat(s.ProfileDir); e != nil || !st.IsDir() {
		msg = "профиль Studio ещё не создан — выполните studio-cli --login"
		return Meta{Name: s.Name(), Available: false, Message: msg, Automatic: false, ManualFallback: true}
	}
	return Meta{
		Name:           s.Name(),
		Available:      err == nil,
		Message:        msg,
		Automatic:      err == nil,
		ManualFallback: true,
	}
}

func (s *Studio) Retrieve(ctx context.Context, videoID string) (io.ReadCloser, error) {
	if err := s.Available(ctx); err != nil {
		return nil, err
	}
	videoID = strings.TrimSpace(videoID)
	if !validYouTubeID(videoID) {
		return nil, fmt.Errorf("%w: некорректный video_id", ErrBadContent)
	}
	workDir, err := os.MkdirTemp(s.TempDir, "infidrive-studio-*")
	if err != nil {
		return nil, err
	}
	cleanup := func() { _ = os.RemoveAll(workDir) }

	res, err := s.runWorker(ctx, videoID, workDir)
	if err != nil {
		cleanup()
		return nil, err
	}
	if !res.OK {
		cleanup()
		return nil, mapStudioError(res)
	}
	if res.Path == "" {
		cleanup()
		return nil, fmt.Errorf("%w: worker не вернул path", ErrUnavailable)
	}
	st, err := os.Stat(res.Path)
	if err != nil || st.Size() == 0 {
		cleanup()
		return nil, fmt.Errorf("%w: пустой MP4", ErrBadContent)
	}
	if st.Size() > s.MaxBytes {
		cleanup()
		return nil, ErrTooLarge
	}
	f, err := os.Open(res.Path)
	if err != nil {
		cleanup()
		return nil, err
	}
	return &tempFileCloser{File: f, cleanup: cleanup}, nil
}

type studioResult struct {
	OK         bool     `json:"ok"`
	Path       string   `json:"path"`
	Bytes      int64    `json:"bytes"`
	Quality    string   `json:"quality"`
	Format     string   `json:"format"`
	Width      int      `json:"width"`
	Height     int      `json:"height"`
	DurationMS int64    `json:"duration_ms"`
	Error      string   `json:"error"`
	Message    string   `json:"message"`
	Logs       []string `json:"logs"`
}

func (s *Studio) runWorker(ctx context.Context, videoID, outDir string) (studioResult, error) {
	node := s.NodeBin
	if node == "" {
		node = "node"
	}
	args := []string{
		s.WorkerJS,
		"--video-id=" + videoID,
		"--out-dir=" + outDir,
		"--profile-dir=" + s.ProfileDir,
		fmt.Sprintf("--timeout-ms=%d", int(s.Timeout.Milliseconds())),
	}
	if !s.Headless {
		args = append(args, "--headed")
	}
	ctx, cancel := context.WithTimeout(ctx, s.Timeout+20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, node, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if s.OnProgress != nil {
		s.OnProgress("launch", 5, "Запуск Studio Chromium")
	}
	_ = cmd.Run()
	parseProgress(stderr.Bytes(), s.OnProgress)

	var res studioResult
	out := bytes.TrimSpace(stdout.Bytes())
	if len(out) == 0 {
		return res, fmt.Errorf("%w: пустой ответ Studio worker; stderr=%s", ErrUnavailable, truncate(stderr.String(), 600))
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return res, fmt.Errorf("%w: JSON Studio worker: %v", ErrUnavailable, err)
	}
	return res, nil
}

func mapStudioError(res studioResult) error {
	msg := res.Message
	if msg == "" {
		msg = res.Error
	}
	switch res.Error {
	case "login_required", "login_timeout":
		return fmt.Errorf("%w: %s", ErrLoginRequired, msg)
	case "automation_blocked":
		return fmt.Errorf("%w: %s", ErrAutomationBlocked, msg)
	case "captcha_required":
		return fmt.Errorf("%w: %s", ErrCaptcha, msg)
	case "download_limit", "download_action_missing", "video_not_found", "download_timeout":
		return fmt.Errorf("%w: %s", ErrUnavailable, msg)
	default:
		return fmt.Errorf("%w: %s", ErrUnavailable, msg)
	}
}

// ResolveStudioWorkerJS ищет studio-cli.mjs.
func ResolveStudioWorkerJS(hint string) string {
	if hint != "" {
		if st, err := os.Stat(hint); err == nil && !st.IsDir() {
			return hint
		}
	}
	candidates := []string{
		filepath.Join("browser-retriever", "src", "studio-cli.mjs"),
		filepath.Join("..", "browser-retriever", "src", "studio-cli.mjs"),
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "browser-retriever", "src", "studio-cli.mjs"),
			filepath.Join(dir, "..", "browser-retriever", "src", "studio-cli.mjs"),
		)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			if abs, err := filepath.Abs(c); err == nil {
				return abs
			}
			return c
		}
	}
	return hint
}
