// Command codecdiag: диагностика Studio 720p MP4 / upscale → YTDrive signal.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ytdrive/internal/codec"
	"ytdrive/internal/ffmpeg"
)

func main() {
	in := flag.String("in", "tmp/studio-out/infidrive-a38ce031-c00.mp4", "входной MP4")
	outDir := flag.String("out", "tmp/codecdiag", "каталог отчётов")
	maxFrames := flag.Int("max-frames", 0, "лимит кадров (0 = все, diag сам ограничивает сэмпл)")
	tryDecode := flag.Bool("try-decode", true, "пробный полный decode при найденном CRC")
	flag.Parse()

	ff, fp, _ := ffmpeg.Resolve("", "")
	if ff == "" {
		fatal("ffmpeg не найден")
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}
	opt := codec.Options{FFmpeg: ff, FFProbe: fp, Width: 1920, Height: 1080, Block: 8}

	type run struct {
		label string
		w, h  int
		flags string
	}
	runs := []run{
		{label: "native_720p", w: 0, h: 0, flags: ""},
		{label: "up_nn_1080", w: 1920, h: 1080, flags: "neighbor"},
		{label: "up_bilinear_1080", w: 1920, h: 1080, flags: "bilinear"},
		{label: "up_bicubic_1080", w: 1920, h: 1080, flags: "bicubic"},
		{label: "up_lanczos_1080", w: 1920, h: 1080, flags: "lanczos"},
	}
	_ = maxFrames

	var all []codec.DiagnoseStats
	for _, r := range runs {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
		fmt.Printf("=== %s ===\n", r.label)
		st, err := codec.DiagnoseFile(ctx, *in, opt, r.w, r.h, r.flags, r.label)
		cancel()
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}
		printStats(st)
		all = append(all, st)

		if *tryDecode && st.HeaderCRCOk > 0 && st.ExtractedFrames > 0 {
			outPath := filepath.Join(*outDir, r.label+".bin")
			ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Minute)
			_, res, derr := codec.DiagnoseAndMaybeDecode(ctx2, *in, outPath, opt, r.w, r.h, r.flags)
			cancel2()
			if derr != nil {
				fmt.Printf("decode: FAIL %v\n", derr)
			} else {
				fmt.Printf("decode: OK sha=%s frames=%d size=%d\n", res.SHA256Hex, res.UniqueFrames, res.Size)
			}
		} else {
			fmt.Printf("decode: skipped (no valid headers)\n")
		}
		fmt.Println()
	}

	rep, _ := json.MarshalIndent(all, "", "  ")
	_ = os.WriteFile(filepath.Join(*outDir, "diagnose.json"), rep, 0o644)
	fmt.Printf("wrote %s\n", filepath.Join(*outDir, "diagnose.json"))
}

func printStats(st codec.DiagnoseStats) {
	fmt.Printf("frames=%d markers=%d hdr_crc_ok=%d hdr_crc_fail=%d cal_fail=%d\n",
		st.FramesRead, st.MarkersFound, st.HeaderCRCOk, st.HeaderCRCFail, st.CalibrationFail)
	fmt.Printf("block_confidence=%.1f%% (%d/%d) packets_ok=%d packets_crc_fail=%d extracted=%d\n",
		st.BlockConfidence, st.ConfidentBlocks, st.SampledBlocks, st.PacketsOK, st.PacketsCRCFail, st.ExtractedFrames)
	fmt.Printf("best_layout=%s offset=(%d,%d)\n", st.BestLayout, st.BestOffsetX, st.BestOffsetY)
}

func fatal(v any) {
	fmt.Fprintln(os.Stderr, v)
	os.Exit(1)
}
