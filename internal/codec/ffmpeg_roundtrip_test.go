package codec

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"ytdrive/internal/ffmpeg"
)

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if err := ffmpeg.New("", "").Available(); err != nil {
		t.Skip(err.Error())
	}
}

func TestFFmpegRoundTrip(t *testing.T) {
	requireFFmpeg(t)
	opt := fastOpt()
	data := []byte("ffmpeg round trip: бинарные \x00\xff данные")
	in := writeTemp(t, "round.bin", data)
	outVideo := filepath.Join(t.TempDir(), "video.mp4")
	if _, err := EncodeFile(context.Background(), in, outVideo, opt, nil); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored.bin")
	dec, err := DecodeFile(context.Background(), outVideo, restored, Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(restored)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got %q", got)
	}
	if dec.Name != "round.bin" {
		t.Fatalf("name %s", dec.Name)
	}
}

func TestLossyReencode(t *testing.T) {
	requireFFmpeg(t)
	opt := DefaultOptions()
	opt.CRF = 14
	opt.Preset = "veryfast"
	opt.FPS = 15
	opt.GOP = 1
	data := bytes.Repeat([]byte("lossy-check-0123456789"), 40)
	in := writeTemp(t, "lossy.bin", data)
	src := filepath.Join(t.TempDir(), "src.mp4")
	if _, err := EncodeFile(context.Background(), in, src, opt, nil); err != nil {
		t.Fatal(err)
	}
	runner := ffmpeg.New("", "")
	cases := []struct {
		name  string
		crf   int
		scale string
	}{
		{name: "crf23", crf: 23},
		{name: "crf28", crf: 28},
		{name: "scale640-crf23", crf: 23, scale: "640x360"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			re := filepath.Join(t.TempDir(), "re.mp4")
			if err := runner.Transcode(context.Background(), src, re, tc.crf, tc.scale); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "out.bin")
			if _, err := DecodeFile(context.Background(), re, out, Options{}, nil); err != nil {
				t.Fatalf("ограничение: повторное сжатие %s не восстановлено: %v", tc.name, err)
			}
			got, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("ограничение: SHA/байты не совпали после %s", tc.name)
			}
		})
	}
}
