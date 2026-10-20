package pipeline

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
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

type mapRetriever struct {
	fake *youtube.FakeProvider
}

func (m mapRetriever) Name() string { return "fake-map" }
func (m mapRetriever) Available(context.Context) error { return nil }
func (m mapRetriever) Retrieve(_ context.Context, videoID string) (io.ReadCloser, error) {
	b := m.fake.BytesByID(videoID)
	if len(b) == 0 {
		return nil, retrieval.ErrUnavailable
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func TestChunkedUploadRoundtrip1MiB(t *testing.T) {
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
	driveSvc := drive.NewService(meta, nil, "http://localhost", 50<<20)
	fake := &youtube.FakeProvider{}
	fake.SetConnected(true)
	key := bytes.Repeat([]byte{3}, 32)
	box, err := contentcrypto.NewBox(key)
	if err != nil {
		t.Fatal(err)
	}
	opt := codec.DefaultOptionsV2()
	opt.FFmpeg, opt.FFProbe = ff, fp
	svc := New(driveSvc, meta, fake, mapRetriever{fake: fake}, box, Config{
		MaxBytes:       50 << 20,
		MaxChunkBytes:  512 << 10, // 512 KiB → несколько частей на 1 MiB
		MaxTempBytes:   256 << 20,
		Privacy:        "public",
		TempDir:        t.TempDir(),
		Encode:         opt,
		WaitProcessing: 30 * time.Second,
	})

	plain := make([]byte, 1<<20)
	if _, err := rand.Read(plain); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(plain)

	f, err := svc.UploadEncryptEncodePublish(context.Background(), "", "rand-1m.bin", "application/octet-stream", bytes.NewReader(plain), int64(len(plain)))
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != "ready" {
		t.Fatalf("status=%s", f.Status)
	}
	if f.SHA256 != hex.EncodeToString(want[:]) {
		t.Fatalf("sha file meta")
	}
	man, err := meta.Manifest(f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if man.ChunkTotal < 2 {
		t.Fatalf("expected multiple chunks, got %d", man.ChunkTotal)
	}
	if fake.UploadCount() != man.ChunkTotal {
		t.Fatalf("uploads=%d chunks=%d", fake.UploadCount(), man.ChunkTotal)
	}

	name, got, err := svc.Download(context.Background(), f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if name != "rand-1m.bin" {
		t.Fatalf("name=%q", name)
	}
	sum := sha256.Sum256(got)
	if hex.EncodeToString(sum[:]) != hex.EncodeToString(want[:]) {
		t.Fatal("sha mismatch after download")
	}
}

func TestChunkedUpload5MiBSingleChunk(t *testing.T) {
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
	driveSvc := drive.NewService(meta, nil, "http://localhost", 50<<20)
	fake := &youtube.FakeProvider{}
	fake.SetConnected(true)
	box, _ := contentcrypto.NewBox(bytes.Repeat([]byte{7}, 32))
	opt := codec.DefaultOptionsV2()
	opt.FFmpeg, opt.FFProbe = ff, fp
	svc := New(driveSvc, meta, fake, mapRetriever{fake: fake}, box, Config{
		MaxBytes:       50 << 20,
		MaxChunkBytes:  5 << 20,
		MaxTempBytes:   512 << 20,
		Privacy:        "public",
		TempDir:        t.TempDir(),
		Encode:         opt,
		WaitProcessing: 60 * time.Second,
	})
	plain := make([]byte, 5<<20)
	_, _ = rand.Read(plain)
	want := sha256.Sum256(plain)
	f, err := svc.UploadEncryptEncodePublish(context.Background(), "", "rand-5m.bin", "application/octet-stream", bytes.NewReader(plain), int64(len(plain)))
	if err != nil {
		t.Fatal(err)
	}
	if f.SHA256 != hex.EncodeToString(want[:]) {
		t.Fatal("meta sha")
	}
	_, got, err := svc.Download(context.Background(), f.ID)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(got)
	if hex.EncodeToString(sum[:]) != hex.EncodeToString(want[:]) {
		t.Fatal("roundtrip sha")
	}
}
