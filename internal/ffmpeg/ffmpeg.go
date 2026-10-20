// Package ffmpeg запускает внешний FFmpeg без shell и без CGO.
// Команда собирается списком аргументов, процессы ограничены context.Context.
package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"ytdrive/internal/video"
)

var (
	// ErrNotFound — в PATH нет исполняемого файла.
	ErrNotFound = errors.New("ffmpeg не найден")
	// ErrRun — FFmpeg завершился с ошибкой или вернул непригодные данные.
	ErrRun = errors.New("ошибка ffmpeg")
)

// Runner хранит пути к ffmpeg и ffprobe. Пустая строка означает имя из PATH.
type Runner struct {
	FFmpeg  string
	FFProbe string
}

// Spec описывает выходной MP4.
type Spec struct {
	Width    int
	Height   int
	FPS      int
	CRF      int
	GOP      int
	BitrateK int
	Preset   string
}

// New подставляет имена по умолчанию.
func New(ffmpegPath, ffprobePath string) Runner {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}
	return Runner{FFmpeg: ffmpegPath, FFProbe: ffprobePath}
}

// Available проверяет, что ffmpeg запускается.
func (r Runner) Available() error {
	_, err := r.look(r.FFmpeg)
	return err
}

func (r Runner) look(name string) (string, error) {
	if strings.ContainsAny(name, `/\`) {
		if _, err := os.Stat(name); err != nil {
			return "", fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return name, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return path, nil
}

// Probe возвращает размеры первого видеопотока.
func (r Runner) Probe(ctx context.Context, path string) (int, int, error) {
	if probe, err := r.look(r.FFProbe); err == nil {
		cmd := exec.CommandContext(ctx, probe,
			"-v", "error",
			"-select_streams", "v:0",
			"-show_entries", "stream=width,height",
			"-of", "csv=s=x:p=0",
			path,
		)
		hideWindow(cmd)
		out, err := cmd.Output()
		if err == nil {
			if w, h, ok := parseSize(string(out)); ok {
				return w, h, nil
			}
		}
	}
	bin, err := r.look(r.FFmpeg)
	if err != nil {
		return 0, 0, err
	}
	cmd := exec.CommandContext(ctx, bin, "-hide_banner", "-i", path)
	hideWindow(cmd)
	out, _ := cmd.CombinedOutput()
	if w, h, ok := parseSize(string(out)); ok {
		return w, h, nil
	}
	return 0, 0, fmt.Errorf("%w: не удалось определить размер видео", ErrRun)
}

var sizeRE = regexp.MustCompile(`(\d{2,5})x(\d{2,5})`)

func parseSize(s string) (int, int, bool) {
	m := sizeRE.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(m[1])
	h, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil || w < 16 || h < 16 {
		return 0, 0, false
	}
	return w, h, true
}

// EncodeVideo читает кадры RGB24 из callback и пишет H.264 MP4.
// Файл публикуется через временное имя в том же каталоге.
func (r Runner) EncodeVideo(ctx context.Context, output string, spec Spec, frames func(write func([]byte) error) error) error {
	bin, err := r.look(r.FFmpeg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dirOf(output), 0o750); err != nil {
		return err
	}
	partial := output + ".partial.mp4"
	_ = os.Remove(partial)
	args := []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"-s", fmt.Sprintf("%dx%d", spec.Width, spec.Height),
		"-r", strconv.Itoa(spec.FPS),
		"-i", "pipe:0",
		"-an",
		"-c:v", "libx264",
		"-preset", spec.Preset,
		"-pix_fmt", "yuv420p",
		"-g", strconv.Itoa(spec.GOP),
		"-keyint_min", strconv.Itoa(spec.GOP),
		"-bf", "0",
		"-tune", "stillimage",
		"-movflags", "+faststart",
	}
	if spec.BitrateK > 0 {
		br := strconv.Itoa(spec.BitrateK) + "k"
		args = append(args, "-b:v", br, "-maxrate", br, "-bufsize", strconv.Itoa(spec.BitrateK*2)+"k")
	} else {
		args = append(args, "-crf", strconv.Itoa(spec.CRF))
	}
	args = append(args, partial)

	cmd := exec.CommandContext(ctx, bin, args...)
	hideWindow(cmd)
	cmd.Stdout = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: %v", ErrRun, err)
	}
	errBuf := &bytes.Buffer{}
	errDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(errBuf, io.LimitReader(stderr, 64<<10))
		close(errDone)
	}()

	frameSize := spec.Width * spec.Height * 3
	writeErr := frames(func(pix []byte) error {
		if len(pix) != frameSize {
			return fmt.Errorf("%w: кадр %d байт, ожидалось %d", ErrRun, len(pix), frameSize)
		}
		_, err := stdin.Write(pix)
		return err
	})
	closeErr := stdin.Close()
	waitErr := cmd.Wait()
	<-errDone
	if writeErr != nil {
		_ = os.Remove(partial)
		return wrapErr(writeErr, errBuf.String())
	}
	if closeErr != nil {
		_ = os.Remove(partial)
		return wrapErr(closeErr, errBuf.String())
	}
	if waitErr != nil {
		_ = os.Remove(partial)
		return wrapErr(waitErr, errBuf.String())
	}
	if err := replace(partial, output); err != nil {
		_ = os.Remove(partial)
		return err
	}
	return nil
}

// DecodeVideo декодирует видео в поток RGB24-кадров.
// После закрытия канала кадров нужно прочитать errc.
// Отмена ctx останавливает процесс.
func (r Runner) DecodeVideo(ctx context.Context, path string) (int, int, <-chan video.Image, <-chan error, error) {
	bin, err := r.look(r.FFmpeg)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	w, h, err := r.Probe(ctx, path)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	cmd := exec.CommandContext(ctx, bin,
		"-hide_banner", "-loglevel", "error",
		"-i", path,
		"-an",
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"pipe:1",
	)
	hideWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, 0, nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return 0, 0, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return 0, 0, nil, nil, fmt.Errorf("%w: %v", ErrRun, err)
	}
	frames := make(chan video.Image, 2)
	errc := make(chan error, 1)
	go func() {
		defer close(frames)
		errBuf := &bytes.Buffer{}
		errDone := make(chan struct{})
		go func() {
			_, _ = io.Copy(errBuf, io.LimitReader(stderr, 64<<10))
			close(errDone)
		}()
		readErr := readFrames(ctx, stdout, w, h, frames)
		_ = stdout.Close()
		waitErr := cmd.Wait()
		<-errDone
		switch {
		case readErr != nil:
			errc <- wrapErr(readErr, errBuf.String())
		case waitErr != nil:
			errc <- wrapErr(waitErr, errBuf.String())
		default:
			errc <- nil
		}
	}()
	return w, h, frames, errc, nil
}

func readFrames(ctx context.Context, r io.Reader, w, h int, out chan<- video.Image) error {
	size := w * h * 3
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		buf := make([]byte, size)
		_, err := io.ReadFull(r, buf)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- video.Image{Width: w, Height: h, Pix: buf}:
		}
	}
}

// Transcode перекодирует готовый MP4 с потерями. scale — выражение FFmpeg или пустая строка.
// Используется тестами устойчивости и не принимает произвольную строку от пользователя API:
// вызывающий код передаёт фиксированные значения.
func (r Runner) Transcode(ctx context.Context, input, output string, crf int, scale string) error {
	bin, err := r.look(r.FFmpeg)
	if err != nil {
		return err
	}
	args := []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-i", input,
		"-an",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", strconv.Itoa(crf),
		"-bf", "0",
		"-pix_fmt", "yuv420p",
	}
	switch scale {
	case "":
	case "640x360":
		args = append(args, "-vf", "scale=640:360")
	default:
		return fmt.Errorf("%w: неподдерживаемый масштаб %q", ErrRun, scale)
	}
	args = append(args, output)
	cmd := exec.CommandContext(ctx, bin, args...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		return fmt.Errorf("%w: %v (%s)", ErrRun, err, msg)
	}
	return nil
}

func wrapErr(err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if len(stderr) > 400 {
		stderr = stderr[len(stderr)-400:]
	}
	if stderr == "" {
		return fmt.Errorf("%w: %v", ErrRun, err)
	}
	return fmt.Errorf("%w: %v (%s)", ErrRun, err, stderr)
}

func dirOf(path string) string {
	dir := strings.ReplaceAll(path, "\\", "/")
	if i := strings.LastIndex(dir, "/"); i >= 0 {
		return path[:i]
	}
	return "."
}

func replace(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	_ = os.Remove(dst)
	return os.Rename(src, dst)
}
