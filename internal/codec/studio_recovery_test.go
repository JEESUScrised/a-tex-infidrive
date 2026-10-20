package codec

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestStudioDownloadRecoveryScenario моделирует сценарий из документации:
// encode → «повторное сжатие как после YouTube» → ручной импорт MP4 → decode → SHA-256.
// Официальный Data API не отдаёт видеопоток; пользователь скачивает ролик из Studio.
func TestStudioDownloadRecoveryScenario(t *testing.T) {
	runner, base := requireFFmpeg(t)
	opt := DefaultOptions()
	opt.FFmpeg = base.FFmpeg
	opt.FFProbe = base.FFProbe
	opt.CRF = 14
	opt.Preset = "veryfast"
	opt.FPS = 15
	opt.GOP = 1

	data := bytes.Repeat([]byte("studio-recovery-check-"), 30)
	in := writeTemp(t, "payload.bin", data)
	src := filepath.Join(t.TempDir(), "upload.mp4")
	if _, err := EncodeFile(context.Background(), in, src, opt, nil); err != nil {
		t.Fatal(err)
	}
	// Имитация файла, скачанного пользователем из YouTube Studio после обработки.
	studio := filepath.Join(t.TempDir(), "from-studio.mp4")
	if err := runner.Transcode(context.Background(), src, studio, 23, ""); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "restored.bin")
	dec, err := DecodeFile(context.Background(), studio, out, Options{FFmpeg: opt.FFmpeg, FFProbe: opt.FFProbe}, nil)
	if err != nil {
		t.Fatalf("восстановление после «Studio» не удалось: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("SHA/байты не совпали")
	}
	if dec.SHA256Hex == "" {
		t.Fatal("empty sha")
	}
}
