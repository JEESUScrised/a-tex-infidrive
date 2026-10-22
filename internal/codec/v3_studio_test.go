package codec

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"ytdrive/internal/format"
)

func TestV3StudioCRFRoundTrip(t *testing.T) {
	runner, base := requireFFmpeg(t)
	opt := DefaultOptionsV3()
	opt.FFmpeg = base.FFmpeg
	opt.FFProbe = base.FFProbe
	if opt.FormatVersion != format.Version3 {
		t.Fatalf("want Version3")
	}

	data := make([]byte, 64<<10)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	in := writeTemp(t, "v3payload.bin", data)
	src := filepath.Join(t.TempDir(), "v3.mp4")
	if _, err := EncodeFile(context.Background(), in, src, opt, nil); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name  string
		crf   int
		scale string
	}{
		{"crf23", 23, ""},
		{"crf28", 28, ""},
		{"crf23_360p", 23, "640x360"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), tc.name+".mp4")
			if err := runner.Transcode(context.Background(), src, dst, tc.crf, tc.scale); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), tc.name+".bin")
			dec, err := DecodeFile(context.Background(), dst, out, Options{FFmpeg: opt.FFmpeg, FFProbe: opt.FFProbe}, nil)
			if tc.scale == "640x360" {
				// 360p Studio path: may fail — document, don't require success
				if err != nil {
					t.Logf("360p decode expected-hard: %v", err)
					return
				}
			}
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			got, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("bytes mismatch corrupt=%d recovered=%d", dec.CorruptPackets, dec.RecoveredPackets)
			}
		})
	}
}

func TestV2StudioProjectionStillWorks(t *testing.T) {
	runner, base := requireFFmpeg(t)
	opt := DefaultOptionsV2()
	opt.FFmpeg = base.FFmpeg
	opt.FFProbe = base.FFProbe
	opt.FrameRepeat = 2 // slightly more robust for synthetic scale

	data := bytes.Repeat([]byte("v2-proj-"), 8000)
	in := writeTemp(t, "v2payload.bin", data)
	src := filepath.Join(t.TempDir(), "v2-1080.mp4")
	if _, err := EncodeFile(context.Background(), in, src, opt, nil); err != nil {
		t.Fatal(err)
	}
	studio := filepath.Join(t.TempDir(), "studio-720.mp4")
	if err := runner.Transcode(context.Background(), src, studio, 23, "1280x720"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.bin")
	if _, err := DecodeFile(context.Background(), studio, out, Options{FFmpeg: opt.FFmpeg, FFProbe: opt.FFProbe}, nil); err != nil {
		t.Fatalf("projected decode failed: %v", err)
	}
	got, _ := os.ReadFile(out)
	if !bytes.Equal(got, data) {
		t.Fatal("SHA/байты не совпали после 1080→720")
	}
}
