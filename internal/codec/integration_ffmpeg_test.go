package codec

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"ytdrive/internal/ffmpeg"
	"ytdrive/internal/video"
)

func TestIntegrationFullCycleSHA256(t *testing.T) {
	_, opt := requireFFmpeg(t)

	zipPath := filepath.Join(t.TempDir(), "sample.zip")
	if err := writeZip(zipPath, map[string][]byte{
		"a.txt": []byte("внутри архива"),
		"b.bin": {0x00, 0xff, 0x7f},
	}); err != nil {
		t.Fatal(err)
	}
	zipBytes, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	random := make([]byte, 8192)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		file string
		data []byte
	}{
		{name: "text", file: "note.txt", data: []byte("полный цикл YTDrive\n")},
		{name: "zip", file: "sample.zip", data: zipBytes},
		{name: "binary", file: "pattern.bin", data: bytes.Repeat([]byte{0x00, 0x7f, 0xff, 0x10}, 200)},
		{name: "random", file: "rand.bin", data: random},
		{name: "empty", file: "empty.dat", data: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := writeTemp(t, tc.file, tc.data)
			wantSum := sha256.Sum256(tc.data)
			wantHex := hex.EncodeToString(wantSum[:])
			videoPath := filepath.Join(t.TempDir(), tc.name+".mp4")
			enc, err := EncodeFile(context.Background(), in, videoPath, opt, nil)
			if err != nil {
				t.Fatal(err)
			}
			if enc.SHA256Hex != wantHex {
				t.Fatalf("encode sha %s want %s", enc.SHA256Hex, wantHex)
			}
			out := filepath.Join(t.TempDir(), "restored-"+tc.file)
			dec, err := DecodeFile(context.Background(), videoPath, out, Options{FFmpeg: opt.FFmpeg, FFProbe: opt.FFProbe}, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			if int64(len(got)) != enc.OriginalSize || !bytes.Equal(got, tc.data) {
				t.Fatalf("size/content mismatch: got %d want %d", len(got), len(tc.data))
			}
			if dec.SHA256Hex != wantHex {
				t.Fatalf("decode sha %s want %s", dec.SHA256Hex, wantHex)
			}
			if sum, err := Verify(in, out); err != nil || sum != wantHex {
				t.Fatalf("verify %s %v", sum, err)
			}
		})
	}
}

func TestIntegrationResilienceTable(t *testing.T) {
	runner, base := requireFFmpeg(t)
	opt := DefaultOptions()
	opt.FFmpeg = base.FFmpeg
	opt.FFProbe = base.FFProbe
	opt.CRF = 14
	opt.Preset = "veryfast"
	opt.FPS = 15
	opt.GOP = 1

	data := bytes.Repeat([]byte("resilience-0123456789"), 50)
	in := writeTemp(t, "resilience.bin", data)
	src := filepath.Join(t.TempDir(), "src.mp4")
	if _, err := EncodeFile(context.Background(), in, src, opt, nil); err != nil {
		t.Fatal(err)
	}

	memOpt := fastOpt()
	type row struct {
		name string
		run  func(t *testing.T)
	}
	rows := []row{
		{
			name: "reencode-crf23",
			run: func(t *testing.T) {
				mustRoundTripTranscode(t, runner, opt, src, data, 23, "")
			},
		},
		{
			name: "reencode-crf28",
			run: func(t *testing.T) {
				mustRoundTripTranscode(t, runner, opt, src, data, 28, "")
			},
		},
		{
			name: "scale-640x360-crf23",
			run: func(t *testing.T) {
				mustRoundTripTranscode(t, runner, opt, src, data, 23, "640x360")
			},
		},
		{
			name: "damaged-payload-frame-with-parity",
			run: func(t *testing.T) {
				frames, _, err := EncodeFrames(context.Background(), in, memOpt)
				if err != nil {
					t.Fatal(err)
				}
				idx := firstDataFrame(t, frames, memOpt)
				wipePayload(frames[idx])
				out := filepath.Join(t.TempDir(), "out.bin")
				dec, err := DecodeFrames(context.Background(), frames, out, Options{})
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(out)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, data) {
					t.Fatalf("байты не совпали")
				}
				if dec.RecoveredPackets < 1 && dec.CorruptPackets < 1 {
					t.Fatalf("ожидалось восстановление или отбраковка копии: recovered=%d corrupt=%d", dec.RecoveredPackets, dec.CorruptPackets)
				}
			},
		},
		{
			name: "missing-frame-with-parity",
			run: func(t *testing.T) {
				frames, _, err := EncodeFrames(context.Background(), in, memOpt)
				if err != nil {
					t.Fatal(err)
				}
				idx := firstDataFrame(t, frames, memOpt)
				cut := append([]video.Image{}, frames[:idx]...)
				cut = append(cut, frames[idx+1:]...)
				out := filepath.Join(t.TempDir(), "out.bin")
				dec, err := DecodeFrames(context.Background(), cut, out, Options{})
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(out)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, data) {
					t.Fatalf("байты не совпали")
				}
				if dec.RecoveredPackets < 1 {
					t.Fatalf("recovered=%d", dec.RecoveredPackets)
				}
			},
		},
	}

	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			r.run(t)
			t.Log("PASS")
		})
	}
}

func mustRoundTripTranscode(t *testing.T, runner ffmpeg.Runner, opt Options, src string, data []byte, crf int, scale string) {
	t.Helper()
	re := filepath.Join(t.TempDir(), "re.mp4")
	if err := runner.Transcode(context.Background(), src, re, crf, scale); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.bin")
	if _, err := DecodeFile(context.Background(), re, out, Options{FFmpeg: opt.FFmpeg, FFProbe: opt.FFProbe}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("байты не совпали после перекодирования")
	}
}

func writeZip(path string, files map[string][]byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write(body); err != nil {
			return err
		}
	}
	return zw.Close()
}
