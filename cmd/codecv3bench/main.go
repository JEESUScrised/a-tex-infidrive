// Command codecv3bench: сравнение block 8/10/16 на 720p + CRF 23/28 + даунскейл 360p.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"ytdrive/internal/codec"
	"ytdrive/internal/ffmpeg"
	"ytdrive/internal/format"
	"ytdrive/internal/packets"
	"ytdrive/internal/video"
)

func main() {
	ff, fp, _ := ffmpeg.Resolve("", "")
	if ff == "" {
		fatal("ffmpeg не найден")
	}
	outDir := "tmp/codecv3bench"
	_ = os.MkdirAll(outDir, 0o755)
	runner := ffmpeg.New(ff, fp)

	sizes := []struct {
		label string
		n     int64
	}{
		{"64k", 64 << 10},
		{"256k", 256 << 10},
		{"1m", 1 << 20},
		{"5m", 5 << 20},
	}

	cf, err := os.Create(filepath.Join(outDir, "results.csv"))
	if err != nil {
		fatal(err)
	}
	defer cf.Close()
	cw := csv.NewWriter(cf)
	_ = cw.Write([]string{
		"size", "block", "phase", "crf", "encode_sec", "mp4_bytes", "duration_sec",
		"mib_per_min", "corrupt_packets", "recovered_packets", "unique_frames", "sha_ok", "peak_alloc_mib",
	})

	for _, sz := range sizes {
		inPath := filepath.Join(outDir, "in_"+sz.label+".bin")
		if err := writeRand(inPath, sz.n); err != nil {
			fatal(err)
		}
		want := fileSHA(inPath)
		for _, block := range []int{8, 10, 16} {
			layout, err := video.NewLayout(1280, 720, block)
			if err != nil {
				fmt.Printf("skip block %d: %v\n", block, err)
				continue
			}
			chunk := layout.PayloadCap() - packets.Overhead
			opt := codec.Options{
				Width: 1280, Height: 720, Block: block,
				FPS: 10, CRF: 18, Preset: "veryfast", GOP: 1,
				FrameRepeat: 2, PacketRepeat: 2, ParityGroup: 8,
				ChunkSize: chunk, Luma0: 16, Luma1: 235,
				FFmpeg: ff, FFProbe: fp, FormatVersion: format.Version3,
			}
			baseMP4 := filepath.Join(outDir, fmt.Sprintf("%s_b%d_base.mp4", sz.label, block))
			runtime.GC()
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			baseAlloc := ms.TotalAlloc
			t0 := time.Now()
			enc, err := codec.EncodeFile(context.Background(), inPath, baseMP4, opt, nil)
			encSec := time.Since(t0).Seconds()
			runtime.ReadMemStats(&ms)
			peak := float64(ms.TotalAlloc-baseAlloc) / (1 << 20)
			if err != nil {
				fmt.Printf("encode fail %s b%d: %v\n", sz.label, block, err)
				continue
			}
			st, _ := os.Stat(baseMP4)
			dur := float64(enc.Frames*opt.FrameRepeat) / float64(opt.FPS)
			mibMin := (float64(sz.n) / (1 << 20)) / (dur / 60.0)
			fmt.Printf("ENC %s b%d frames=%d mp4=%d dur=%.1fs %.2f MiB/min peak=%.0fMiB\n",
				sz.label, block, enc.Frames, st.Size(), dur, mibMin, peak)

			// base decode (no extra loss)
			writeRow(cw, sz.label, block, "base_crf18", 18, encSec, st.Size(), dur, mibMin, peak,
				decodeCheck(ff, fp, baseMP4, want))

			for _, crf := range []int{23, 28} {
				for _, ph := range []struct {
					name, scale string
				}{
					{fmt.Sprintf("reenc_crf%d", crf), ""},
					{fmt.Sprintf("reenc_crf%d_360p", crf), "640x360"},
				} {
					dst := filepath.Join(outDir, fmt.Sprintf("%s_b%d_%s.mp4", sz.label, block, ph.name))
					if err := runner.Transcode(context.Background(), baseMP4, dst, crf, ph.scale); err != nil {
						fmt.Printf("  transcode %s fail: %v\n", ph.name, err)
						continue
					}
					st2, _ := os.Stat(dst)
					d2 := dur // same timeline
					m2 := mibMin
					chk := decodeCheck(ff, fp, dst, want)
					fmt.Printf("  %s sha_ok=%v corrupt=%d recovered=%d\n", ph.name, chk.ok, chk.corrupt, chk.recovered)
					writeRow(cw, sz.label, block, ph.name, crf, encSec, st2.Size(), d2, m2, peak, chk)
				}
			}
		}
	}
	cw.Flush()
	fmt.Println("wrote", filepath.Join(outDir, "results.csv"))
}

type decCheck struct {
	ok, corrupt, recovered, frames int
	okFlag                         bool
}

func decodeCheck(ff, fp, mp4, want string) decCheck {
	out := mp4 + ".out.bin"
	_ = os.Remove(out)
	res, err := codec.DecodeFile(context.Background(), mp4, out, codec.Options{FFmpeg: ff, FFProbe: fp}, nil)
	defer os.Remove(out)
	if err != nil {
		return decCheck{}
	}
	ok := fileSHA(out) == want && res.SHA256Hex == want
	return decCheck{ok: boolToInt(ok), corrupt: res.CorruptPackets, recovered: res.RecoveredPackets, frames: res.UniqueFrames, okFlag: ok}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func writeRow(cw *csv.Writer, size string, block int, phase string, crf int, encSec float64, mp4Bytes int64, dur, mibMin, peak float64, chk decCheck) {
	_ = cw.Write([]string{
		size, fmt.Sprintf("%d", block), phase, fmt.Sprintf("%d", crf),
		fmt.Sprintf("%.3f", encSec), fmt.Sprintf("%d", mp4Bytes), fmt.Sprintf("%.3f", dur),
		fmt.Sprintf("%.3f", mibMin), fmt.Sprintf("%d", chk.corrupt), fmt.Sprintf("%d", chk.recovered),
		fmt.Sprintf("%d", chk.frames), fmt.Sprintf("%v", chk.okFlag), fmt.Sprintf("%.1f", peak),
	})
	cw.Flush()
}

func writeRand(path string, n int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, 64<<10)
	var left = n
	for left > 0 {
		chunk := int64(len(buf))
		if chunk > left {
			chunk = left
		}
		if _, err := rand.Read(buf[:chunk]); err != nil {
			return err
		}
		if _, err := f.Write(buf[:chunk]); err != nil {
			return err
		}
		left -= chunk
	}
	return nil
}

func fileSHA(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func fatal(v any) {
	fmt.Fprintln(os.Stderr, v)
	os.Exit(1)
}
