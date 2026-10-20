package codec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"ytdrive/internal/ffmpeg"
	"ytdrive/internal/packets"
	"ytdrive/internal/video"
)

// DecodeFile восстанавливает файл из MP4. Повреждённый результат не публикуется.
func DecodeFile(ctx context.Context, input, output string, opt Options, progress ProgressFunc) (DecodeResult, error) {
	opt.PrepareDecode()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	runner := ffmpeg.New(opt.FFmpeg, opt.FFProbe)
	_, _, frames, errc, err := runner.DecodeVideo(ctx, input)
	if err != nil {
		return DecodeResult{}, err
	}
	return consume(ctx, func() (video.Image, error) {
		img, ok := <-frames
		if !ok {
			return video.Image{}, io.EOF
		}
		return img, nil
	}, geometryHint(opt), output, progress, errc)
}

// DecodeFrames восстанавливает файл из уже извлечённых кадров.
func DecodeFrames(ctx context.Context, frames []video.Image, output string, opt Options) (DecodeResult, error) {
	opt.PrepareDecode()
	i := 0
	return consume(ctx, func() (video.Image, error) {
		if i >= len(frames) {
			return video.Image{}, io.EOF
		}
		img := frames[i]
		i++
		return img, nil
	}, geometryHint(opt), output, nil, nil)
}

func geometryHint(opt Options) *video.Layout {
	if opt.Width <= 0 || opt.Height <= 0 || opt.Block <= 0 {
		return nil
	}
	layout, err := video.NewLayout(opt.Width, opt.Height, opt.Block)
	if err != nil {
		return nil
	}
	return &layout
}

func consume(ctx context.Context, next func() (video.Image, error), hint *video.Layout, output string, progress ProgressFunc, errc <-chan error) (DecodeResult, error) {
	var result DecodeResult
	var locked *video.Layout
	var asm *assembler
	seen := map[uint32]struct{}{}
	frameCount := 0

	finishStream := func() error {
		if errc == nil {
			return nil
		}
		return <-errc
	}

	for {
		if err := ctx.Err(); err != nil {
			if asm != nil {
				asm.cleanup()
			}
			_ = finishStream()
			return result, err
		}
		img, err := next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if asm != nil {
				asm.cleanup()
			}
			_ = finishStream()
			return result, err
		}

		var (
			layout  video.Layout
			hdr     video.FrameHeader
			payload []byte
		)
		if locked == nil {
			layout, hdr, payload, err = tryExtract(img, candidateLayouts(img, hint))
		} else {
			layout = *locked
			hdr, payload, err = video.Extract(img, layout)
		}
		if err != nil {
			if errors.Is(err, video.ErrUnsupported) {
				if asm != nil {
					asm.cleanup()
				}
				_ = finishStream()
				return result, err
			}
			result.SkippedFrames++
			continue
		}
		if locked == nil {
			created, cerr := newAssembler(int(hdr.ChunkSize), int(hdr.ParityGroup), hdr.FileID)
			if cerr != nil {
				_ = finishStream()
				return result, cerr
			}
			asm = created
			locked = &layout
			frameCount = int(hdr.FrameCount)
		} else if hdr.FileID != asm.fileID || int(hdr.ChunkSize) != asm.chunk || int(hdr.ParityGroup) != asm.group {
			result.SkippedFrames++
			continue
		}

		result.FramesSeen++
		if _, ok := seen[hdr.FrameIndex]; ok {
			result.DuplicateFrames++
		} else {
			seen[hdr.FrameIndex] = struct{}{}
			result.UniqueFrames++
		}
		pkts, corrupt := packets.Split(payload)
		result.CorruptPackets += corrupt
		for _, pkt := range pkts {
			if err := asm.add(pkt); err != nil {
				asm.cleanup()
				_ = finishStream()
				return result, err
			}
		}
		result.CorruptPackets += asm.corrupt
		asm.corrupt = 0
		if progress != nil && frameCount > 0 {
			progress(result.UniqueFrames, frameCount)
		}
	}

	if streamErr := finishStream(); streamErr != nil {
		if asm != nil {
			asm.cleanup()
		}
		return result, streamErr
	}
	if asm == nil {
		return result, ErrNoSignal
	}
	defer asm.cleanup()
	meta, err := asm.finish(output)
	result.RecoveredPackets = asm.recovered
	result.CorruptPackets += asm.corrupt
	if err != nil {
		return result, err
	}
	result.Name = meta.Name
	result.MIME = meta.MIME
	result.SHA256Hex = hex.EncodeToString(meta.SHA256[:])
	result.Size = int64(meta.OriginalSize)
	if meta.StoredSize > 0 {
		result.DataPackets = int((meta.StoredSize + uint64(meta.ChunkSize) - 1) / uint64(meta.ChunkSize))
	}
	return result, nil
}

func tryExtract(img video.Image, layouts []video.Layout) (video.Layout, video.FrameHeader, []byte, error) {
	var last error
	for _, layout := range layouts {
		hdr, payload, err := video.Extract(img, layout)
		if err != nil {
			if errors.Is(err, video.ErrUnsupported) {
				return video.Layout{}, video.FrameHeader{}, nil, err
			}
			last = err
			continue
		}
		if int(hdr.Cols) != layout.Cols || int(hdr.Rows) != layout.Rows {
			continue
		}
		return layout, hdr, payload, nil
	}
	if last == nil {
		last = video.ErrFrame
	}
	return video.Layout{}, video.FrameHeader{}, nil, last
}

func candidateLayouts(img video.Image, hint *video.Layout) []video.Layout {
	seen := map[video.Layout]struct{}{}
	var out []video.Layout
	add := func(layout video.Layout) {
		if layout.Validate() != nil {
			return
		}
		if _, ok := seen[layout]; ok {
			return
		}
		seen[layout] = struct{}{}
		out = append(out, layout)
	}
	if hint != nil {
		add(*hint)
	}
	if layout, err := video.NewLayout(1280, 720, 16); err == nil {
		add(layout)
	}
	for _, block := range []int{16, 8, 32, 4} {
		if img.Width%block == 0 && img.Height%block == 0 {
			if layout, err := video.NewLayout(img.Width, img.Height, block); err == nil {
				add(layout)
			}
		}
	}
	return out
}

// Verify сравнивает SHA-256 двух файлов потоково.
func Verify(original, restored string) (string, error) {
	a, err := fileSHA(original)
	if err != nil {
		return "", err
	}
	b, err := fileSHA(restored)
	if err != nil {
		return "", err
	}
	ah := hex.EncodeToString(a[:])
	bh := hex.EncodeToString(b[:])
	if a != b {
		return "", errf("файлы различаются: %s и %s", ah, bh)
	}
	return ah, nil
}

func fileSHA(path string) ([32]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, err
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

func replaceFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".partial"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		os.Remove(tmp)
		if copyErr != nil {
			return copyErr
		}
		if syncErr != nil {
			return syncErr
		}
		return closeErr
	}
	_ = os.Remove(dst)
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	_ = os.Remove(src)
	return nil
}
