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

// ErrCaptcha — сайт запросил CAPTCHA; обход запрещён.
var ErrCaptcha = errors.New("требуется CAPTCHA стороннего сервиса")

// ProgressFunc — колбэк прогресса подготовки файла.
type ProgressFunc func(stage string, pct int, message string)

// Playwright — VideoRetriever через Node.js Playwright worker (browser-retriever).
// Go передаёт только video_id; произвольные URL от пользователя не принимаются.
type Playwright struct {
	NodeBin    string // default "node"
	WorkerJS   string // path to browser-retriever/src/cli.mjs
	TempDir    string // локально: os.TempDir; на VPS: tmpfs (/dev/shm/infidrive)
	Timeout    time.Duration
	Headless   bool
	MaxBytes   int64
	OnProgress ProgressFunc
}

type workerResult struct {
	OK         bool     `json:"ok"`
	Path       string   `json:"path"`
	Bytes      int64    `json:"bytes"`
	Quality    string   `json:"quality"`
	Format     string   `json:"format"`
	DurationMS int64    `json:"duration_ms"`
	Error      string   `json:"error"`
	Message    string   `json:"message"`
	Logs       []string `json:"logs"`
}

func NewPlaywright(workerJS, tempDir string, timeout time.Duration, maxBytes int64) *Playwright {
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	if maxBytes <= 0 {
		maxBytes = 64 << 20
	}
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	return &Playwright{
		NodeBin:  "node",
		WorkerJS: workerJS,
		TempDir:  tempDir,
		Timeout:  timeout,
		Headless: true,
		MaxBytes: maxBytes,
	}
}

func (p *Playwright) Name() string { return "playwright_ssyoutube" }

func (p *Playwright) Available(ctx context.Context) error {
	if p == nil || strings.TrimSpace(p.WorkerJS) == "" {
		return fmt.Errorf("%w: worker не настроен (BROWSER_RETRIEVER_JS)", ErrUnavailable)
	}
	if _, err := os.Stat(p.WorkerJS); err != nil {
		return fmt.Errorf("%w: worker файл не найден: %v", ErrUnavailable, err)
	}
	node := p.NodeBin
	if node == "" {
		node = "node"
	}
	cmd := exec.CommandContext(ctx, node, "-e", "process.exit(0)")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: node недоступен: %v", ErrUnavailable, err)
	}
	return nil
}

func (p *Playwright) Diagnose(ctx context.Context) Meta {
	err := p.Available(ctx)
	msg := "Playwright SSYouTube worker готов"
	if err != nil {
		msg = err.Error()
	}
	return Meta{
		Name:           p.Name(),
		Available:      err == nil,
		Message:        msg,
		Automatic:      err == nil,
		ManualFallback: true,
	}
}

// Retrieve запускает Chromium через worker и возвращает поток скачанного MP4.
// Временный файл удаляется при Close().
func (p *Playwright) Retrieve(ctx context.Context, videoID string) (io.ReadCloser, error) {
	if err := p.Available(ctx); err != nil {
		return nil, err
	}
	videoID = strings.TrimSpace(videoID)
	if !validYouTubeID(videoID) {
		return nil, fmt.Errorf("%w: некорректный video_id", ErrBadContent)
	}

	workDir, err := os.MkdirTemp(p.TempDir, "infidrive-pw-*")
	if err != nil {
		return nil, err
	}
	cleanup := func() { _ = os.RemoveAll(workDir) }

	res, err := p.runWorker(ctx, videoID, workDir)
	if err != nil {
		cleanup()
		return nil, err
	}
	if !res.OK {
		cleanup()
		return nil, mapWorkerError(res)
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
	if st.Size() > p.MaxBytes {
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

func (p *Playwright) runWorker(ctx context.Context, videoID, outDir string) (workerResult, error) {
	node := p.NodeBin
	if node == "" {
		node = "node"
	}
	args := []string{
		p.WorkerJS,
		"--video-id=" + videoID,
		"--out-dir=" + outDir,
		fmt.Sprintf("--timeout-ms=%d", int(p.Timeout.Milliseconds())),
	}
	if !p.Headless {
		args = append(args, "--headed")
	}
	ctx, cancel := context.WithTimeout(ctx, p.Timeout+15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, node, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_BROWSERS_PATH="+os.Getenv("PLAYWRIGHT_BROWSERS_PATH"))

	if p.OnProgress != nil {
		p.OnProgress("launch", 5, "Запуск браузера")
	}

	err := cmd.Run()
	// Parse progress lines from stderr even on failure.
	parseProgress(stderr.Bytes(), p.OnProgress)

	var res workerResult
	out := bytes.TrimSpace(stdout.Bytes())
	if len(out) == 0 {
		if err != nil {
			return res, fmt.Errorf("%w: worker: %v; stderr=%s", ErrUnavailable, err, truncate(stderr.String(), 800))
		}
		return res, fmt.Errorf("%w: пустой ответ worker", ErrUnavailable)
	}
	if jerr := json.Unmarshal(out, &res); jerr != nil {
		return res, fmt.Errorf("%w: JSON worker: %v; out=%s", ErrUnavailable, jerr, truncate(string(out), 400))
	}
	return res, nil
}

func mapWorkerError(res workerResult) error {
	msg := res.Message
	if msg == "" {
		msg = res.Error
	}
	switch res.Error {
	case "captcha_required":
		return fmt.Errorf("%w: %s", ErrCaptcha, msg)
	case "formats_timeout", "download_timeout", "no_mp4_option", "browser_launch_failed":
		return fmt.Errorf("%w: %s", ErrUnavailable, msg)
	default:
		return fmt.Errorf("%w: %s", ErrUnavailable, msg)
	}
}

func parseProgress(stderr []byte, cb ProgressFunc) {
	if cb == nil {
		return
	}
	for _, line := range bytes.Split(stderr, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var ev struct {
			Type    string `json:"type"`
			Stage   string `json:"stage"`
			Pct     int    `json:"pct"`
			Message string `json:"message"`
		}
		if json.Unmarshal(line, &ev) != nil || ev.Type != "progress" {
			continue
		}
		cb(ev.Stage, ev.Pct, ev.Message)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

type tempFileCloser struct {
	*os.File
	cleanup func()
	closed  bool
}

func (t *tempFileCloser) Close() error {
	var err error
	if !t.closed {
		t.closed = true
		err = t.File.Close()
		if t.cleanup != nil {
			t.cleanup()
		}
	}
	return err
}

// ResolveWorkerJS ищет browser-retriever/src/cli.mjs относительно cwd / exeе.
func ResolveWorkerJS(hint string) string {
	if hint != "" {
		if st, err := os.Stat(hint); err == nil && !st.IsDir() {
			return hint
		}
	}
	candidates := []string{
		filepath.Join("browser-retriever", "src", "cli.mjs"),
		filepath.Join("..", "browser-retriever", "src", "cli.mjs"),
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "browser-retriever", "src", "cli.mjs"),
			filepath.Join(dir, "..", "browser-retriever", "src", "cli.mjs"),
		)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			abs, err := filepath.Abs(c)
			if err == nil {
				return abs
			}
			return c
		}
	}
	return hint
}
