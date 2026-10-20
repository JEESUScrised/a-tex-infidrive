package codec

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"ytdrive/internal/ffmpeg"
	"ytdrive/internal/format"
	"ytdrive/internal/packets"
	"ytdrive/internal/video"
)

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// EncodeFile кодирует файл в MP4. Запись в output происходит только после успешного FFmpeg.
func EncodeFile(ctx context.Context, input, output string, opt Options, progress ProgressFunc) (EncodeResult, error) {
	if err := opt.PrepareEncode(); err != nil {
		return EncodeResult{}, err
	}
	runner := ffmpeg.New(opt.FFmpeg, opt.FFProbe)
	spec := ffmpeg.Spec{
		Width:    opt.Width,
		Height:   opt.Height,
		FPS:      opt.FPS,
		CRF:      opt.CRF,
		GOP:      opt.GOP,
		BitrateK: opt.BitrateK,
		Preset:   opt.Preset,
	}
	var result EncodeResult
	err := runner.EncodeVideo(ctx, output, spec, func(write func([]byte) error) error {
		var err error
		result, err = encode(ctx, input, opt, progress, func(img video.Image) error {
			for i := 0; i < opt.FrameRepeat; i++ {
				if err := write(img.Pix); err != nil {
					return err
				}
			}
			return nil
		})
		return err
	})
	if err != nil {
		return EncodeResult{}, err
	}
	return result, nil
}

// EncodeFrames строит кадры в памяти. Так тесты проверяют формат без FFmpeg.
func EncodeFrames(ctx context.Context, input string, opt Options) ([]video.Image, EncodeResult, error) {
	if err := opt.PrepareEncode(); err != nil {
		return nil, EncodeResult{}, err
	}
	var frames []video.Image
	result, err := encode(ctx, input, opt, nil, func(img video.Image) error {
		frames = append(frames, img)
		return nil
	})
	if err != nil {
		return nil, EncodeResult{}, err
	}
	return frames, result, nil
}

func encode(ctx context.Context, input string, opt Options, progress ProgressFunc, emit func(video.Image) error) (EncodeResult, error) {
	src, err := os.Open(input)
	if err != nil {
		return EncodeResult{}, err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return EncodeResult{}, err
	}
	if info.IsDir() {
		return EncodeResult{}, errf("%s является каталогом", input)
	}

	tmp, err := os.CreateTemp("", "ytdrive-payload-*.bin")
	if err != nil {
		return EncodeResult{}, err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	hasher := sha256.New()
	reader := io.TeeReader(ctxReader{ctx: ctx, r: src}, hasher)
	var original int64
	if opt.Gzip {
		gz, gzErr := gzip.NewWriterLevel(tmp, gzip.DefaultCompression)
		if gzErr != nil {
			return EncodeResult{}, gzErr
		}
		original, err = io.Copy(gz, reader)
		closeErr := gz.Close()
		if err == nil {
			err = closeErr
		}
	} else {
		original, err = io.Copy(tmp, reader)
	}
	if err != nil {
		return EncodeResult{}, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return EncodeResult{}, err
	}
	st, err := tmp.Stat()
	if err != nil {
		return EncodeResult{}, err
	}
	stored := st.Size()
	var sum [32]byte
	copy(sum[:], hasher.Sum(nil))

	rawName := opt.DisplayName
	if strings.TrimSpace(rawName) == "" {
		rawName = filepath.Base(input)
	}
	name := cleanName(rawName)
	flags := uint16(0)
	if opt.Gzip {
		flags |= format.FlagGzip
	}
	flags = format.WithFEC(flags, fecMode(opt))
	meta := packets.Metadata{
		Flags:        flags,
		ChunkSize:    uint16(opt.ChunkSize),
		Name:         name,
		MIME:         mimeByName(name),
		OriginalSize: uint64(original),
		StoredSize:   uint64(stored),
		SHA256:       sum,
	}
	metaBytes, err := meta.Marshal()
	if err != nil {
		return EncodeResult{}, err
	}
	layout, err := video.NewLayout(opt.Width, opt.Height, opt.Block)
	if err != nil {
		return EncodeResult{}, err
	}
	frameCount, packetCount, err := countPlan(len(metaBytes), stored, opt, layout.PayloadCap())
	if err != nil {
		return EncodeResult{}, err
	}
	if frameCount == 0 {
		return EncodeResult{}, errf("не удалось собрать ни одного кадра")
	}
	fileID, err := newFileID()
	if err != nil {
		return EncodeResult{}, err
	}

	fw := &frameWriter{
		ctx:        ctx,
		layout:     layout,
		opt:        opt,
		flags:      flags,
		fileID:     fileID,
		frameCount: uint32(frameCount),
		emit:       emit,
		progress:   progress,
	}
	var group [][]byte
	var lastData, lastParity []byte
	err = iterate(len(metaBytes), stored, opt.ChunkSize, opt.ParityGroup, opt.PacketRepeat, func(it item) error {
		var payload []byte
		switch it.Type {
		case packets.TypeMeta:
			start := int(it.Index) * opt.ChunkSize
			payload = metaBytes[start : start+it.Length]
		case packets.TypeData:
			if it.Replica == 0 {
				buf := make([]byte, it.Length)
				if _, err := io.ReadFull(tmp, buf); err != nil {
					return errf("чтение данных: %w", err)
				}
				lastData = buf
				if opt.ParityGroup > 0 {
					group = append(group, buf)
				}
			}
			payload = lastData
		case packets.TypeParity:
			if it.Replica == 0 {
				lastParity = xorAll(group, opt.ChunkSize)
				group = nil
			}
			payload = lastParity
		default:
			return errf("внутренний тип пакета %d", it.Type)
		}
		return fw.add(packets.Packet{
			Type:    it.Type,
			Flags:   flags,
			FileID:  fileID,
			Index:   it.Index,
			Total:   it.Total,
			Replica: it.Replica,
			Group:   it.Group,
			Payload: append([]byte(nil), payload...),
		})
	})
	if err != nil {
		return EncodeResult{}, err
	}
	if err := fw.flush(); err != nil {
		return EncodeResult{}, err
	}
	if int(fw.frameIndex) != frameCount {
		return EncodeResult{}, errf("внутренний рассинхрон кадров: %d и %d", fw.frameIndex, frameCount)
	}
	return EncodeResult{
		OriginalSize: original,
		StoredSize:   stored,
		SHA256Hex:    hex.EncodeToString(sum[:]),
		Frames:       frameCount,
		Packets:      packetCount,
		Width:        opt.Width,
		Height:       opt.Height,
		FileID:       fileID,
	}, nil
}

type frameWriter struct {
	ctx        context.Context
	layout     video.Layout
	opt        Options
	flags      uint16
	fileID     uint32
	frameCount uint32
	frameIndex uint32
	buf        []byte
	emit       func(video.Image) error
	progress   ProgressFunc
}

func (w *frameWriter) add(p packets.Packet) error {
	raw := p.Marshal()
	if len(raw) > w.layout.PayloadCap() {
		return errf("пакет не помещается в кадр")
	}
	if len(w.buf) > 0 && len(w.buf)+len(raw) > w.layout.PayloadCap() {
		if err := w.flush(); err != nil {
			return err
		}
	}
	w.buf = append(w.buf, raw...)
	return nil
}

func (w *frameWriter) flush() error {
	if len(w.buf) == 0 {
		return nil
	}
	if err := w.ctx.Err(); err != nil {
		return err
	}
	ver := w.opt.FormatVersion
	if ver == 0 {
		ver = format.Version
	}
	hdr := video.FrameHeader{
		Version:      ver,
		PixelMode:    format.PixelLuma1,
		Flags:        w.flags,
		FileID:       w.fileID,
		FrameIndex:   w.frameIndex,
		FrameCount:   w.frameCount,
		PayloadLen:   uint16(len(w.buf)),
		ChunkSize:    uint16(w.opt.ChunkSize),
		BlockSize:    uint16(w.layout.Block),
		Cols:         uint16(w.layout.Cols),
		Rows:         uint16(w.layout.Rows),
		ParityGroup:  uint16(w.opt.ParityGroup),
		PacketRepeat: uint16(w.opt.PacketRepeat),
	}
	img, err := video.Render(w.layout, hdr, w.buf, w.opt.Luma0, w.opt.Luma1)
	if err != nil {
		return err
	}
	if err := w.emit(img); err != nil {
		return err
	}
	w.frameIndex++
	if w.progress != nil {
		w.progress(int(w.frameIndex), int(w.frameCount))
	}
	w.buf = w.buf[:0]
	return nil
}

func fecMode(opt Options) int {
	repeat := opt.PacketRepeat > 1 || opt.FrameRepeat > 1
	xor := opt.ParityGroup > 0
	switch {
	case repeat && xor:
		return format.FECRepeatXOR
	case xor:
		return format.FECXOR
	case repeat:
		return format.FECRepeat
	default:
		return format.FECNone
	}
}

func xorAll(parts [][]byte, chunk int) []byte {
	out := make([]byte, chunk)
	for _, p := range parts {
		for i := range p {
			out[i] ^= p[i]
		}
	}
	return out
}

func newFileID() (uint32, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	id := binary.BigEndian.Uint32(b[:])
	if id == 0 {
		id = 1
	}
	return id, nil
}

func cleanName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	name = strings.Trim(name, ". ")
	if name == "" || name == "." || name == ".." {
		return "file.bin"
	}
	if len(name) > 180 {
		name = name[:180]
	}
	return name
}

func mimeByName(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return "application/octet-stream"
	}
	kind := mime.TypeByExtension(ext)
	if kind == "" {
		return "application/octet-stream"
	}
	if i := strings.Index(kind, ";"); i >= 0 {
		kind = strings.TrimSpace(kind[:i])
	}
	return kind
}
