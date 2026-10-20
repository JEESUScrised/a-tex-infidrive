package ffmpeg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMissingFFmpeg(t *testing.T) {
	r := New("ytdrive-ffmpeg-missing", "ytdrive-ffprobe-missing")
	err := r.Available()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	_, _, _, _, err = r.DecodeVideo(context.Background(), "none.mp4")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("decode got %v", err)
	}
}

func TestFFmpegRejectsGarbage(t *testing.T) {
	ff, fp, _ := Resolve("", "")
	r := New(ff, fp)
	if err := r.Available(); err != nil {
		t.Skip(err.Error())
	}
	path := filepath.Join(t.TempDir(), "garbage.mp4")
	if err := os.WriteFile(path, []byte("this is not a video"), 0o640); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, err := r.DecodeVideo(context.Background(), path)
	if err == nil {
		t.Fatal("expected ffmpeg/probe error")
	}
	out := filepath.Join(t.TempDir(), "out.mp4")
	if err := r.Transcode(context.Background(), path, out, 23, ""); err == nil {
		t.Fatal("expected transcode error")
	}
}
