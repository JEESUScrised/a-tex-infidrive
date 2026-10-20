package jobs

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ytdrive/internal/codec"
	"ytdrive/internal/ffmpeg"
	"ytdrive/internal/storage"
)

func TestQueueRejectsDuplicateAndOverflow(t *testing.T) {
	st := openStore(t)
	opt := codec.DefaultOptions()
	m := New(st, opt, 1, 1, nil)
	defer m.Stop()

	a := save(t, st, "a.txt", []byte("a"))
	b := save(t, st, "b.txt", []byte("b"))
	if _, err := m.EnqueueEncode(a.ID); err != nil {
		t.Fatal(err)
	}
	_, err := m.EnqueueEncode(a.ID)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("busy: %v", err)
	}
	_, err = m.EnqueueEncode(b.ID)
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("queue: %v", err)
	}
}

func TestEncodeJobFinishes(t *testing.T) {
	st := openStore(t)
	opt := codec.DefaultOptions()
	opt.FFmpeg, opt.FFProbe, _ = ffmpeg.Resolve(opt.FFmpeg, opt.FFProbe)
	opt.FrameRepeat = 1
	opt.PacketRepeat = 1
	opt.Preset = "ultrafast"
	opt.CRF = 20
	opt.FPS = 15
	opt.GOP = 1
	m := New(st, opt, 1, 2, nil)
	m.Start()
	defer m.Stop()

	f := save(t, st, "job.txt", []byte("background-job"))
	job, err := m.EnqueueEncode(f.ID)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(60 * time.Second)
	var got storage.Job
	for time.Now().Before(deadline) {
		got, err = st.GetJob(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == storage.StatusSucceeded || got.Status == storage.StatusFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if ffmpeg.New(opt.FFmpeg, opt.FFProbe).Available() != nil {
		if got.Status != storage.StatusFailed {
			t.Fatalf("without ffmpeg status=%s err=%s", got.Status, got.Error)
		}
		return
	}
	if got.Status != storage.StatusSucceeded {
		t.Fatalf("status=%s err=%s", got.Status, got.Error)
	}
	if _, err := os.Stat(got.OutputPath); err != nil {
		t.Fatal(err)
	}
	file, err := st.GetFile(f.ID)
	if err != nil || !file.HasVideo {
		t.Fatalf("has video %+v %v", file, err)
	}
}

func TestCancelQueuedJob(t *testing.T) {
	st := openStore(t)
	opt := codec.DefaultOptions()
	m := New(st, opt, 1, 4, nil)
	defer m.Stop()

	f := save(t, st, "cancel.txt", []byte("cancel-me"))
	job, err := m.EnqueueEncode(f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != storage.StatusCancelled {
		t.Fatalf("status=%s", got.Status)
	}
	if err := m.Cancel(job.ID); !errors.Is(err, ErrNotCancellable) {
		t.Fatalf("second cancel: %v", err)
	}
	// После отмены тот же файл можно поставить снова.
	if _, err := m.EnqueueEncode(f.ID); err != nil {
		t.Fatal(err)
	}
}

func TestMissingFFmpegFailsJob(t *testing.T) {
	st := openStore(t)
	opt := codec.DefaultOptions()
	opt.FFmpeg = filepath.Join(t.TempDir(), "missing-ffmpeg")
	opt.FFProbe = filepath.Join(t.TempDir(), "missing-ffprobe")
	opt.FrameRepeat = 1
	opt.PacketRepeat = 1
	m := New(st, opt, 1, 2, nil)
	m.Start()
	defer m.Stop()

	f := save(t, st, "nof.txt", []byte("x"))
	job, err := m.EnqueueEncode(f.ID)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	var got storage.Job
	for time.Now().Before(deadline) {
		got, err = st.GetJob(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == storage.StatusFailed || got.Status == storage.StatusSucceeded {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got.Status != storage.StatusFailed {
		t.Fatalf("status=%s err=%s", got.Status, got.Error)
	}
}

func openStore(t *testing.T) *storage.Store {
	t.Helper()
	st, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func save(t *testing.T, st *storage.Store, name string, data []byte) storage.File {
	t.Helper()
	f, err := st.SaveUpload(bytes.NewReader(data), name, "text/plain", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Clean(f.StoredPath)); err != nil {
		t.Fatal(err)
	}
	return f
}
