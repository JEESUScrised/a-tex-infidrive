// Command codecbench измеряет encode/decode YTDrive на crypto/rand данных.
// Пример: go run ./cmd/codecbench -sizes 64k,256k,1m -crf 16,23,28
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"ytdrive/internal/codec"
	"ytdrive/internal/ffmpeg"
	"ytdrive/internal/packets"
	"ytdrive/internal/video"
)

type sizeSpec struct {
	label string
	bytes int64
}

func main() {
	sizesFlag := flag.String("sizes", "64k,256k,1m,2m", "список размеров: 64k,256k,1m,2m,5m")
	crfFlag := flag.String("crf", "16,23,28", "список CRF")
	outDir := flag.String("out", "tmp/codecbench", "каталог артефактов")
	scale := flag.String("scale", "720p", "720p|1080p")
	block := flag.Int("block", 16, "размер блока пикселей")
	frameRepeat := flag.Int("frame-repeat", 2, "повторы кадра")
	packetRepeat := flag.Int("packet-repeat", 2, "повторы пакета")
	preset := flag.String("preset", "veryfast", "FFmpeg preset")
	skipDecode := flag.Bool("skip-decode", false, "только encode")
	flag.Parse()

	ff, fp, src := ffmpeg.Resolve("", "")
	if ff == "" || fp == "" {
		fmt.Fprintf(os.Stderr, "ffmpeg не найден\n")
		os.Exit(1)
	}
	fmt.Printf("ffmpeg=%s (%s)\n", ff, src)
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}

	w, h := 1280, 720
	switch strings.ToLower(*scale) {
	case "1080p":
		w, h = 1920, 1080
	case "720p":
	default:
		fatal(fmt.Errorf("неизвестный scale %q", *scale))
	}

	layout, err := video.NewLayout(w, h, *block)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("layout %dx%d block=%d payload_cap=%d B/frame\n", w, h, *block, layout.PayloadCap())

	sizes, err := parseSizes(*sizesFlag)
	if err != nil {
		fatal(err)
	}
	crfs, err := parseInts(*crfFlag)
	if err != nil {
		fatal(err)
	}

	csvPath := filepath.Join(*outDir, "results.csv")
	cf, err := os.Create(csvPath)
	if err != nil {
		fatal(err)
	}
	defer cf.Close()
	cw := csv.NewWriter(cf)
	_ = cw.Write([]string{
		"size_label", "size_bytes", "scale", "block", "crf", "frame_repeat", "packet_repeat",
		"payload_cap", "encode_sec", "decode_sec", "mp4_bytes", "duration_sec",
		"unique_frames", "data_mb_per_min", "sha_ok", "peak_alloc_mib", "sys_mib",
	})

	for _, sz := range sizes {
		inPath := filepath.Join(*outDir, "in_"+sz.label+".bin")
		if err := writeRand(inPath, sz.bytes); err != nil {
			fatal(err)
		}
		sumIn := fileSHA(inPath)
		for _, crf := range crfs {
			opt := codec.DefaultOptions()
			opt.Width = w
			opt.Height = h
			opt.Block = *block
			opt.CRF = crf
			opt.FrameRepeat = *frameRepeat
			opt.PacketRepeat = *packetRepeat
			opt.Preset = *preset
			opt.FFmpeg = ff
			opt.FFProbe = fp
			maxChunk := layout.PayloadCap() - packets.Overhead
			if maxChunk > 4096 {
				maxChunk = 4096
			}
			if maxChunk < 16 {
				fatal(fmt.Errorf("chunk too small for layout cap=%d", layout.PayloadCap()))
			}
			opt.ChunkSize = maxChunk

			mp4Path := filepath.Join(*outDir, fmt.Sprintf("%s_b%d_crf%d.mp4", sz.label, *block, crf))
			_ = os.Remove(mp4Path)

			runtime.GC()
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			baseAlloc := ms.Alloc

			t0 := time.Now()
			enc, err := codec.EncodeFile(context.Background(), inPath, mp4Path, opt, nil)
			encDur := time.Since(t0)
			runtime.ReadMemStats(&ms)
			peakAlloc := ms.TotalAlloc - baseAlloc
			sysMiB := float64(ms.Sys) / (1024 * 1024)

			if err != nil {
				fmt.Printf("FAIL encode %s crf=%d: %v\n", sz.label, crf, err)
				_ = cw.Write([]string{sz.label, itoa(sz.bytes), *scale, itoa(int64(*block)), itoa(int64(crf)),
					itoa(int64(*frameRepeat)), itoa(int64(*packetRepeat)), itoa(int64(layout.PayloadCap())),
					fmt.Sprintf("%.3f", encDur.Seconds()), "", "", "", "", "", "encode_err",
					fmt.Sprintf("%.1f", float64(peakAlloc)/(1024*1024)), fmt.Sprintf("%.1f", sysMiB)})
				cw.Flush()
				continue
			}

			st, _ := os.Stat(mp4Path)
			mp4Bytes := int64(0)
			if st != nil {
				mp4Bytes = st.Size()
			}
			durSec := float64(enc.Frames) / float64(opt.FPS)
			if opt.FrameRepeat > 0 {
				// EncodeResult.Frames is unique logical frames; wall video frames = Frames*FrameRepeat
				durSec = float64(enc.Frames*opt.FrameRepeat) / float64(opt.FPS)
			}
			mbMin := 0.0
			if durSec > 0 {
				mbMin = (float64(sz.bytes) / (1024 * 1024)) / (durSec / 60.0)
			}

			shaOK := ""
			decSec := ""
			if !*skipDecode {
				outPath := filepath.Join(*outDir, fmt.Sprintf("out_%s_crf%d.bin", sz.label, crf))
				_ = os.Remove(outPath)
				t1 := time.Now()
				dec, err := codec.DecodeFile(context.Background(), mp4Path, outPath, opt, nil)
				d := time.Since(t1)
				decSec = fmt.Sprintf("%.3f", d.Seconds())
				if err != nil {
					shaOK = "decode_err:" + err.Error()
				} else if dec.SHA256Hex != sumIn {
					shaOK = "mismatch"
				} else {
					shaOK = "ok"
				}
			}

			row := []string{
				sz.label, itoa(sz.bytes), *scale, itoa(int64(*block)), itoa(int64(crf)),
				itoa(int64(*frameRepeat)), itoa(int64(*packetRepeat)), itoa(int64(layout.PayloadCap())),
				fmt.Sprintf("%.3f", encDur.Seconds()), decSec, itoa(mp4Bytes),
				fmt.Sprintf("%.2f", durSec), itoa(int64(enc.Frames)),
				fmt.Sprintf("%.3f", mbMin), shaOK,
				fmt.Sprintf("%.1f", float64(peakAlloc)/(1024*1024)), fmt.Sprintf("%.1f", sysMiB),
			}
			_ = cw.Write(row)
			cw.Flush()
			fmt.Printf("%s crf=%d encode=%.1fs mp4=%s dur=%.1fs rate=%.3f MB/min sha=%s\n",
				sz.label, crf, encDur.Seconds(), human(mp4Bytes), durSec, mbMin, shaOK)
		}
	}
	fmt.Println("CSV:", csvPath)
}

func writeRand(path string, n int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, 256<<10)
	var left = n
	for left > 0 {
		chunk := buf
		if int64(len(chunk)) > left {
			chunk = buf[:left]
		}
		if _, err := rand.Read(chunk); err != nil {
			return err
		}
		if _, err := f.Write(chunk); err != nil {
			return err
		}
		left -= int64(len(chunk))
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

func parseSizes(s string) ([]sizeSpec, error) {
	var out []sizeSpec
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" {
			continue
		}
		mult := int64(1)
		num := p
		switch {
		case strings.HasSuffix(p, "kib"), strings.HasSuffix(p, "kb"), strings.HasSuffix(p, "k"):
			mult = 1024
			num = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(p, "kib"), "kb"), "k")
		case strings.HasSuffix(p, "mib"), strings.HasSuffix(p, "mb"), strings.HasSuffix(p, "m"):
			mult = 1024 * 1024
			num = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(p, "mib"), "mb"), "m")
		}
		v, err := strconv.ParseInt(num, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("size %q: %w", p, err)
		}
		out = append(out, sizeSpec{label: p, bytes: v * mult})
	}
	return out, nil
}

func parseInts(s string) ([]int, error) {
	var out []int
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func human(n int64) string {
	const u = 1024.0
	f := float64(n)
	switch {
	case f >= u*u:
		return fmt.Sprintf("%.1fMiB", f/(u*u))
	case f >= u:
		return fmt.Sprintf("%.1fKiB", f/u)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
