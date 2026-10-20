package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"ytdrive/internal/codec"
	"ytdrive/internal/contentcrypto"
	"ytdrive/internal/drive"
	"ytdrive/internal/ffmpeg"
	"ytdrive/internal/retrieval"
	"ytdrive/internal/storage"
	"ytdrive/internal/youtube"
)

func TestEncryptEncodeFakeYouTubeManualRestore(t *testing.T) {
	ff, fp, _ := ffmpeg.Resolve("", "")
	if err := ffmpeg.New(ff, fp).Available(); err != nil {
		t.Skip("ffmpeg required:", err)
	}
	st, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	meta, err := drive.Open(st.DB())
	if err != nil {
		t.Fatal(err)
	}
	driveSvc := drive.NewService(meta, nil, "http://localhost", 128<<10)
	fake := &youtube.FakeProvider{}
	fake.SetConnected(true)
	key := bytes.Repeat([]byte{9}, 32)
	box, err := contentcrypto.NewBox(key)
	if err != nil {
		t.Fatal(err)
	}
	opt := codec.DefaultOptions()
	opt.FFmpeg, opt.FFProbe = ff, fp
	opt.FrameRepeat = 1
	opt.PacketRepeat = 1
	opt.Preset = "ultrafast"
	svc := New(driveSvc, meta, fake, retrieval.NewSSYouTube("", 1<<20), box, Config{
		MaxBytes:       128 << 10,
		Privacy:        "public",
		TempDir:        t.TempDir(),
		Encode:         opt,
		WaitProcessing: 10 * time.Second,
	})

	plain := []byte("e2e-college-demo-payload-32b!!")
	wantSHA := sha256.Sum256(plain)
	f, err := svc.UploadEncryptEncodePublish(context.Background(), "", "demo.bin", "application/octet-stream", bytes.NewReader(plain), int64(len(plain)))
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != "ready" || f.SHA256 != hex.EncodeToString(wantSHA[:]) {
		t.Fatalf("%+v", f)
	}
	vid, _ := meta.VideoID(f.ID)
	if vid == "" {
		t.Fatal("missing video id")
	}

	// Авто-retrieval SSYouTube недоступен.
	if _, _, err := svc.Download(context.Background(), f.ID); err == nil {
		t.Fatal("expected manual fallback")
	}

	mp4 := fake.LastUploadBytes()
	if len(mp4) == 0 {
		t.Fatal("fake did not keep mp4 bytes")
	}
	name, got, err := svc.RestoreFromStream(context.Background(), f.ID, bytes.NewReader(mp4))
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo.bin" || !bytes.Equal(got, plain) {
		t.Fatalf("restore %q %q", name, got)
	}
	// Временные каталоги эксперимента не должны остаться в TempDir сервиса как постоянные objects.
	entries, _ := os.ReadDir(t.TempDir())
	for _, e := range entries {
		if e.Name() == "objects" {
			t.Fatal("unexpected objects dir")
		}
	}
}

func TestSSYouTubeUnavailableByDefault(t *testing.T) {
	ss := retrieval.NewSSYouTube("", 1024)
	if err := ss.Available(context.Background()); err == nil {
		t.Fatal("expected unavailable")
	}
	meta := ss.Diagnose(context.Background())
	if meta.Automatic || !meta.ManualFallback {
		t.Fatalf("%+v", meta)
	}
}
