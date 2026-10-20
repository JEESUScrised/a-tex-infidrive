package codec

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ytdrive/internal/packets"
	"ytdrive/internal/video"
)

func fastOpt() Options {
	opt := DefaultOptions()
	opt.FrameRepeat = 1
	opt.PacketRepeat = 1
	opt.ParityGroup = 8
	opt.FPS = 20
	opt.CRF = 18
	opt.Preset = "ultrafast"
	opt.GOP = 1
	return opt
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatal(err)
	}
	return path
}

func roundTrip(t *testing.T, name string, data []byte, opt Options) DecodeResult {
	t.Helper()
	in := writeTemp(t, name, data)
	frames, enc, err := EncodeFrames(context.Background(), in, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) == 0 {
		t.Fatal("no frames")
	}
	out := filepath.Join(t.TempDir(), "restored"+filepath.Ext(name))
	dec, err := DecodeFrames(context.Background(), frames, out, Options{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("content mismatch: got %d bytes, want %d", len(got), len(data))
	}
	sum := sha256.Sum256(data)
	want := hex.EncodeToString(sum[:])
	if enc.SHA256Hex != want || dec.SHA256Hex != want {
		t.Fatalf("sha encode=%s decode=%s want=%s", enc.SHA256Hex, dec.SHA256Hex, want)
	}
	hash, err := Verify(in, out)
	if err != nil {
		t.Fatal(err)
	}
	if hash != want {
		t.Fatalf("verify %s", hash)
	}
	if dec.Name != name {
		t.Fatalf("name %q", dec.Name)
	}
	return dec
}

func TestDisplayNameOverridesStoredPath(t *testing.T) {
	opt := fastOpt()
	opt.DisplayName = "отчёт.txt"
	in := writeTemp(t, "original", []byte("named"))
	frames, _, err := EncodeFrames(context.Background(), in, opt)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "restored")
	dec, err := DecodeFrames(context.Background(), frames, out, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if dec.Name != "отчёт.txt" {
		t.Fatalf("name %q", dec.Name)
	}
	if dec.MIME != "text/plain" {
		t.Fatalf("mime %s", dec.MIME)
	}
}

func TestRoundTripText(t *testing.T) {
	roundTrip(t, "note.txt", []byte("Привет, YTDrive\nвторая строка\n"), fastOpt())
}

func TestRoundTripBinary(t *testing.T) {
	data := make([]byte, 512)
	for i := range data {
		data[i] = byte(i)
	}
	roundTrip(t, "blob.bin", data, fastOpt())
}

func TestRoundTripEmpty(t *testing.T) {
	dec := roundTrip(t, "empty.bin", nil, fastOpt())
	if dec.Size != 0 || dec.DataPackets != 0 {
		t.Fatalf("%+v", dec)
	}
}

func TestRoundTripRandomAndSHA256(t *testing.T) {
	data := make([]byte, 4096)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	roundTrip(t, "random.bin", data, fastOpt())
}

func TestRoundTripGzip(t *testing.T) {
	opt := fastOpt()
	opt.Gzip = true
	data := bytes.Repeat([]byte("сжатие-"), 300)
	roundTrip(t, "plain.txt", data, opt)
}

func TestDuplicateFrames(t *testing.T) {
	opt := fastOpt()
	in := writeTemp(t, "dup.txt", []byte("duplicate-frames"))
	frames, _, err := EncodeFrames(context.Background(), in, opt)
	if err != nil {
		t.Fatal(err)
	}
	frames = append(frames, frames...)
	out := filepath.Join(t.TempDir(), "out.txt")
	dec, err := DecodeFrames(context.Background(), frames, out, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if dec.DuplicateFrames == 0 {
		t.Fatal("duplicates were not counted")
	}
	got, _ := os.ReadFile(out)
	if string(got) != "duplicate-frames" {
		t.Fatalf("%q", got)
	}
}

func TestSkippedForeignFrame(t *testing.T) {
	opt := fastOpt()
	in := writeTemp(t, "a.txt", []byte("skip-me"))
	frames, _, err := EncodeFrames(context.Background(), in, opt)
	if err != nil {
		t.Fatal(err)
	}
	black := video.Image{Width: opt.Width, Height: opt.Height, Pix: make([]byte, opt.Width*opt.Height*3)}
	frames = append([]video.Image{black}, frames...)
	out := filepath.Join(t.TempDir(), "out.txt")
	dec, err := DecodeFrames(context.Background(), frames, out, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if dec.SkippedFrames < 1 {
		t.Fatalf("skipped=%d", dec.SkippedFrames)
	}
}

func TestCorruptPacketDetectedAndNotPublished(t *testing.T) {
	opt := fastOpt()
	opt.ParityGroup = 0
	opt.PacketRepeat = 1
	data := bytes.Repeat([]byte{0x5A}, 800)
	in := writeTemp(t, "secret.bin", data)
	frames, _, err := EncodeFrames(context.Background(), in, opt)
	if err != nil {
		t.Fatal(err)
	}
	idx := firstDataFrame(t, frames, opt)
	wipePayload(frames[idx])
	out := filepath.Join(t.TempDir(), "out.bin")
	_, err = DecodeFrames(context.Background(), frames, out, Options{})
	if err == nil {
		t.Fatal("expected failure")
	}
	if !errors.Is(err, ErrIncomplete) && !errors.Is(err, ErrChecksum) {
		t.Fatalf("unexpected error %v", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatal("damaged file was published")
	}
}

func TestParityRecoversMissingFrame(t *testing.T) {
	opt := fastOpt()
	opt.ParityGroup = 4
	opt.PacketRepeat = 1
	data := bytes.Repeat([]byte("parity"), 200)
	in := writeTemp(t, "p.bin", data)
	frames, _, err := EncodeFrames(context.Background(), in, opt)
	if err != nil {
		t.Fatal(err)
	}
	idx := firstDataFrame(t, frames, opt)
	frames = append(frames[:idx], frames[idx+1:]...)
	out := filepath.Join(t.TempDir(), "out.bin")
	dec, err := DecodeFrames(context.Background(), frames, out, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if dec.RecoveredPackets < 1 {
		t.Fatalf("recovered=%d", dec.RecoveredPackets)
	}
	got, _ := os.ReadFile(out)
	if !bytes.Equal(got, data) {
		t.Fatal("restored bytes differ")
	}
}

func TestTwoMissingFramesFail(t *testing.T) {
	opt := fastOpt()
	opt.ParityGroup = 8
	opt.PacketRepeat = 1
	data := bytes.Repeat([]byte("xyz"), 500)
	in := writeTemp(t, "m.bin", data)
	frames, _, err := EncodeFrames(context.Background(), in, opt)
	if err != nil {
		t.Fatal(err)
	}
	var dataIdx []int
	for i, img := range frames {
		if frameType(t, img, opt) == packets.TypeData {
			dataIdx = append(dataIdx, i)
		}
	}
	if len(dataIdx) < 2 {
		t.Fatalf("need two data frames, got %d", len(dataIdx))
	}
	drop := map[int]bool{dataIdx[0]: true, dataIdx[1]: true}
	var kept []video.Image
	for i, img := range frames {
		if !drop[i] {
			kept = append(kept, img)
		}
	}
	out := filepath.Join(t.TempDir(), "out.bin")
	_, err = DecodeFrames(context.Background(), kept, out, Options{})
	if !errors.Is(err, ErrIncomplete) {
		t.Fatalf("got %v", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatal("incomplete file was published")
	}
}

func TestReplicaSurvivesCorruptedCopy(t *testing.T) {
	opt := fastOpt()
	opt.PacketRepeat = 2
	opt.ParityGroup = 0
	data := bytes.Repeat([]byte("R"), 400)
	in := writeTemp(t, "r.bin", data)
	frames, _, err := EncodeFrames(context.Background(), in, opt)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[uint32]int{}
	layout, _ := video.NewLayout(opt.Width, opt.Height, opt.Block)
	corrupted := false
	for i := range frames {
		_, payload, err := video.Extract(frames[i], layout)
		if err != nil {
			t.Fatal(err)
		}
		pkts, bad := packets.Split(payload)
		if bad != 0 || len(pkts) != 1 || pkts[0].Type != packets.TypeData {
			continue
		}
		if first, ok := seen[pkts[0].Index]; ok {
			wipePayload(frames[first])
			corrupted = true
			break
		}
		seen[pkts[0].Index] = i
	}
	if !corrupted {
		t.Fatal("no duplicated data packet")
	}
	out := filepath.Join(t.TempDir(), "out.txt")
	if _, err := DecodeFrames(context.Background(), frames, out, Options{}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != string(data) {
		t.Fatalf("%q", got)
	}
}

func TestVerifyMismatch(t *testing.T) {
	a := writeTemp(t, "a.bin", []byte("one"))
	b := writeTemp(t, "b.bin", []byte("two"))
	if _, err := Verify(a, b); err == nil {
		t.Fatal("expected mismatch")
	}
}

func TestRejectsBadOptions(t *testing.T) {
	opt := fastOpt()
	opt.Preset = "nope"
	if err := opt.PrepareEncode(); err == nil {
		t.Fatal("expected preset error")
	}
	opt = fastOpt()
	opt.ChunkSize = 5000
	if err := opt.PrepareEncode(); err == nil {
		t.Fatal("expected chunk error")
	}
}

func firstDataFrame(t *testing.T, frames []video.Image, opt Options) int {
	t.Helper()
	for i, img := range frames {
		if frameType(t, img, opt) == packets.TypeData {
			return i
		}
	}
	t.Fatal("no data frame")
	return -1
}

func frameType(t *testing.T, img video.Image, opt Options) byte {
	t.Helper()
	layout, err := video.NewLayout(opt.Width, opt.Height, opt.Block)
	if err != nil {
		t.Fatal(err)
	}
	_, payload, err := video.Extract(img, layout)
	if err != nil {
		t.Fatal(err)
	}
	pkts, _ := packets.Split(payload)
	if len(pkts) == 0 {
		t.Fatal("frame without packets")
	}
	return pkts[0].Type
}

func wipePayload(img video.Image) {
	y0 := 5 * 16
	for y := y0; y < img.Height; y++ {
		for x := 0; x < img.Width; x++ {
			o := (y*img.Width + x) * 3
			img.Pix[o] = 16
			img.Pix[o+1] = 16
			img.Pix[o+2] = 16
		}
	}
}
