package codec

import (
	"context"
	"fmt"
	"io"

	"ytdrive/internal/ffmpeg"
	"ytdrive/internal/packets"
	"ytdrive/internal/video"
)

// DiagnoseStats — подробная диагностика без публикации файла.
type DiagnoseStats struct {
	Label            string  `json:"label"`
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	FramesRead       int     `json:"frames_read"`
	FramesSampled    int     `json:"frames_sampled"`
	MarkersFound     int     `json:"markers_found"`
	HeaderCRCOk      int     `json:"header_crc_ok"`
	HeaderCRCFail    int     `json:"header_crc_fail"`
	CalibrationFail  int     `json:"calibration_fail"`
	ConfidentBlocks  int     `json:"confident_blocks"`
	SampledBlocks    int     `json:"sampled_blocks"`
	BlockConfidence  float64 `json:"block_confidence_pct"`
	PacketsOK        int     `json:"packets_ok"`
	PacketsCRCFail   int     `json:"packets_crc_fail"`
	BestLayout       string  `json:"best_layout"`
	BestOffsetX      int     `json:"best_offset_x"`
	BestOffsetY      int     `json:"best_offset_y"`
	ExtractedFrames  int     `json:"extracted_frames"`
	CorruptFromSplit int     `json:"corrupt_from_split"`
}

const diagnoseSampleFrames = 30

// DiagnoseFile анализирует MP4 (опционально со scale flags) по кандидатам сеток и сдвигам.
func DiagnoseFile(ctx context.Context, input string, opt Options, scaleW, scaleH int, scaleFlags, label string) (DiagnoseStats, error) {
	opt.PrepareDecode()
	runner := ffmpeg.New(opt.FFmpeg, opt.FFProbe)
	diagCtx, diagCancel := context.WithCancel(ctx)
	defer diagCancel()
	var (
		w, h   int
		frames <-chan video.Image
		errc   <-chan error
		err    error
	)
	if scaleW > 0 && scaleH > 0 {
		w, h, frames, errc, err = runner.DecodeVideoScaled(diagCtx, input, scaleW, scaleH, scaleFlags)
	} else {
		w, h, frames, errc, err = runner.DecodeVideo(diagCtx, input)
	}
	if err != nil {
		return DiagnoseStats{}, err
	}
	st := DiagnoseStats{Label: label, Width: w, Height: h}
	hint := geometryHint(opt)
	bestScore := -1

	for {
		if err := diagCtx.Err(); err != nil {
			drain(frames)
			_ = <-errc
			break
		}
		img, ok := <-frames
		if !ok {
			break
		}
		st.FramesRead++
		if st.FramesSampled >= diagnoseSampleFrames {
			diagCancel()
			drain(frames)
			_ = <-errc
			break
		}
		st.FramesSampled++
		layouts := candidateLayouts(img, hint)
		frameBest := -1
		var frameLayout video.Layout
		var fox, foy int
		var framePayload []byte
		calFailThis := false

		for _, layout := range layouts {
			for _, oy := range []int{0, -1, 1, -2, 2} {
				for _, ox := range []int{0, -1, 1, -2, 2} {
					_, _, thr, magicOK, crcOK, _, _ := video.ProbeHeader(img, layout, ox, oy)
					if thr == 0 && !magicOK {
						calFailThis = true
						continue
					}
					conf, total := video.BlockConfidence(img, layout, thr, ox, oy, 0)
					st.ConfidentBlocks += conf
					st.SampledBlocks += total
					if magicOK {
						st.MarkersFound++
					}
					if crcOK {
						st.HeaderCRCOk++
					} else if magicOK {
						st.HeaderCRCFail++
					}
					score := 0
					if magicOK {
						score += 10
					}
					if crcOK {
						score += 100
					}
					score += conf
					if score > frameBest {
						frameBest = score
						frameLayout = layout
						fox, foy = ox, oy
						if crcOK {
							if _, payload, err := video.ExtractOffset(img, layout, ox, oy); err == nil {
								framePayload = payload
							}
						}
					}
				}
			}
		}
		if calFailThis && frameBest < 0 {
			st.CalibrationFail++
		}
		if frameBest > bestScore {
			bestScore = frameBest
			st.BestLayout = fmt.Sprintf("%dx%d/b%d cols=%d rows=%d", frameLayout.Width, frameLayout.Height, frameLayout.Block, frameLayout.Cols, frameLayout.Rows)
			st.BestOffsetX = fox
			st.BestOffsetY = foy
		}
		if len(framePayload) > 0 {
			st.ExtractedFrames++
			pkts, corrupt := packets.Split(framePayload)
			st.PacketsOK += len(pkts)
			st.CorruptFromSplit += corrupt
			st.PacketsCRCFail += corrupt
		}
	}
	// stream may already be drained; ignore cancel-induced errors
	select {
	case <-errc:
	default:
	}
	if st.SampledBlocks > 0 {
		st.BlockConfidence = 100 * float64(st.ConfidentBlocks) / float64(st.SampledBlocks)
	}
	return st, nil
}

func drain(frames <-chan video.Image) {
	for range frames {
	}
}

// DiagnoseAndMaybeDecode — диагностика + попытка полного decode при найденном сигнале.
func DiagnoseAndMaybeDecode(ctx context.Context, input, output string, opt Options, scaleW, scaleH int, scaleFlags string) (DiagnoseStats, DecodeResult, error) {
	label := "native"
	if scaleW > 0 {
		label = fmt.Sprintf("scale_%dx%d_%s", scaleW, scaleH, scaleFlags)
	}
	st, err := DiagnoseFile(ctx, input, opt, scaleW, scaleH, scaleFlags, label)
	if err != nil {
		return st, DecodeResult{}, err
	}
	if st.HeaderCRCOk == 0 || st.ExtractedFrames == 0 {
		return st, DecodeResult{}, ErrNoSignal
	}
	opt.PrepareDecode()
	runner := ffmpeg.New(opt.FFmpeg, opt.FFProbe)
	var (
		frames <-chan video.Image
		errc   <-chan error
	)
	if scaleW > 0 && scaleH > 0 {
		_, _, frames, errc, err = runner.DecodeVideoScaled(ctx, input, scaleW, scaleH, scaleFlags)
	} else {
		_, _, frames, errc, err = runner.DecodeVideo(ctx, input)
	}
	if err != nil {
		return st, DecodeResult{}, err
	}
	res, err := consume(ctx, func() (video.Image, error) {
		img, ok := <-frames
		if !ok {
			return video.Image{}, io.EOF
		}
		return img, nil
	}, geometryHint(opt), output, nil, errc)
	return st, res, err
}
