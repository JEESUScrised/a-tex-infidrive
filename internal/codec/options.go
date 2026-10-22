package codec

import (
	"fmt"

	"ytdrive/internal/ffmpeg"
	"ytdrive/internal/format"
	"ytdrive/internal/packets"
	"ytdrive/internal/video"
)

// Options задаёт параметры кодирования. Для разбора видео геометрия не обязательна:
// декодер читает сетку из заголовка кадра.
type Options struct {
	Width        int
	Height       int
	Block        int
	FPS          int
	CRF          int
	Preset       string
	BitrateK     int
	GOP          int
	FrameRepeat  int
	PacketRepeat int
	ParityGroup  int
	ChunkSize    int
	Gzip         bool
	Luma0        byte
	Luma1        byte
	FFmpeg       string
	FFProbe      string
	// FormatVersion — версия контейнера кадра (1 = классика, 2 = плотный профиль).
	FormatVersion byte
	// DisplayName попадает в метаданные кадра. Пустое значение — имя входного файла.
	// Веб-сервер хранит байты под безопасным именем и передаёт сюда исходное имя пользователя.
	DisplayName string
}

// ProgressFunc сообщает, сколько уникальных кадров уже обработано.
type ProgressFunc func(done, total int)

// EncodeResult — итог кодирования.
type EncodeResult struct {
	OriginalSize int64
	StoredSize   int64
	SHA256Hex    string
	Frames       int
	Packets      int
	Width        int
	Height       int
	FileID       uint32
}

// DecodeResult — итог восстановления. Файл записан только если SHA-256 совпал.
type DecodeResult struct {
	Name             string
	MIME             string
	SHA256Hex        string
	Size             int64
	FramesSeen       int
	UniqueFrames     int
	DuplicateFrames  int
	SkippedFrames    int
	CorruptPackets   int
	RecoveredPackets int
	DataPackets      int
}

// DefaultOptions — устойчивые значения для локального MP4 720p (YTDrive v1).
func DefaultOptions() Options {
	return Options{
		Width:         1280,
		Height:        720,
		Block:         16,
		FPS:           10,
		CRF:           16,
		Preset:        "veryfast",
		GOP:           1,
		FrameRepeat:   2,
		PacketRepeat:  2,
		ParityGroup:   8,
		ChunkSize:     360,
		Luma0:         16,
		Luma1:         235,
		FFmpeg:        "ffmpeg",
		FFProbe:       "ffprobe",
		FormatVersion: format.Version,
	}
}

// DefaultOptionsV2 — плотный экспериментальный профиль (~1–2 МБ/мин полезных данных).
// Измерено (crypto/rand): 1 МиБ → ~30 с / ~8.5 МиБ MP4 (CRF 23); 5 МиБ → ~151 с / ~42 МиБ.
// PacketRepeat=2 чуть снижает плотность, но повышает устойчивость encrypted/high-entropy payload.
// Studio Download отдаёт 720p: декодер проецирует сетку 240×135 на фактический кадр.
func DefaultOptionsV2() Options {
	return Options{
		Width:         1920,
		Height:        1080,
		Block:         8,
		FPS:           10,
		CRF:           23,
		Preset:        "veryfast",
		GOP:           1,
		FrameRepeat:   1,
		PacketRepeat:  2,
		ParityGroup:   8,
		ChunkSize:     3924, // PayloadCap(1080p/b8)=3960 − packet overhead
		Luma0:         16,
		Luma1:         235,
		FFmpeg:        "ffmpeg",
		FFProbe:       "ffprobe",
		FormatVersion: format.Version2,
	}
}

// DefaultOptionsV3 — нативные 1280×720 под YouTube Studio Download.
// Block 8: баланс плотности и запаса к H.264; FR=2/PR=2 для устойчивости после recompress.
// PayloadCap(720p/b8)=1740 → ChunkSize=1704. Block 12 не делится на 1280×720.
func DefaultOptionsV3() Options {
	return Options{
		Width:         1280,
		Height:        720,
		Block:         8,
		FPS:           10,
		CRF:           18,
		Preset:        "veryfast",
		GOP:           1,
		FrameRepeat:   2,
		PacketRepeat:  2,
		ParityGroup:   8,
		ChunkSize:     1704,
		Luma0:         16,
		Luma1:         235,
		FFmpeg:        "ffmpeg",
		FFProbe:       "ffprobe",
		FormatVersion: format.Version3,
	}
}

var presets = map[string]struct{}{
	"ultrafast": {},
	"superfast": {},
	"veryfast":  {},
	"faster":    {},
	"fast":      {},
	"medium":    {},
	"slow":      {},
	"slower":    {},
	"veryslow":  {},
}

// PrepareEncode проверяет параметры и подставляет путь к FFmpeg.
func (o *Options) PrepareEncode() error {
	o.FFmpeg, o.FFProbe, _ = ffmpeg.Resolve(o.FFmpeg, o.FFProbe)
	if o.Width < 64 || o.Height < 64 || o.Width > 3840 || o.Height > 2160 {
		return fmt.Errorf("разрешение %dx%d вне диапазона", o.Width, o.Height)
	}
	if o.Block < 4 || o.Block > 64 {
		return fmt.Errorf("размер блока %d недопустим", o.Block)
	}
	if o.FPS < 1 || o.FPS > 60 {
		return fmt.Errorf("fps должен быть от 1 до 60")
	}
	if o.BitrateK == 0 && (o.CRF < 0 || o.CRF > 51) {
		return fmt.Errorf("crf должен быть от 0 до 51")
	}
	if o.BitrateK != 0 && (o.BitrateK < 100 || o.BitrateK > 50000) {
		return fmt.Errorf("битрейт должен быть от 100 до 50000 кбит/с")
	}
	if _, ok := presets[o.Preset]; !ok {
		return fmt.Errorf("неизвестный preset %q", o.Preset)
	}
	if o.GOP < 1 || o.GOP > 300 {
		return fmt.Errorf("gop должен быть от 1 до 300")
	}
	if o.FrameRepeat < 1 || o.FrameRepeat > 8 {
		return fmt.Errorf("frame-repeat должен быть от 1 до 8")
	}
	if o.PacketRepeat < 1 || o.PacketRepeat > 8 {
		return fmt.Errorf("packet-repeat должен быть от 1 до 8")
	}
	if o.ParityGroup < 0 || o.ParityGroup > 64 {
		return fmt.Errorf("parity-group должен быть от 0 до 64")
	}
	if o.ChunkSize < 16 || o.ChunkSize > 16384 {
		return fmt.Errorf("размер пакета данных %d недопустим", o.ChunkSize)
	}
	if o.FormatVersion == 0 {
		o.FormatVersion = format.Version
	}
	if !format.SupportedFrameVersion(o.FormatVersion) {
		return fmt.Errorf("неподдерживаемая версия формата %d", o.FormatVersion)
	}
	if int(o.Luma1) < int(o.Luma0)+32 {
		return fmt.Errorf("контраст уровней яркости слишком мал")
	}
	layout, err := video.NewLayout(o.Width, o.Height, o.Block)
	if err != nil {
		return err
	}
	if packets.EncodedSize(o.ChunkSize) > layout.PayloadCap() {
		maxPayload := layout.PayloadCap() - packets.Overhead
		return fmt.Errorf("блок данных %d байт не помещается в кадр (максимум %d)", o.ChunkSize, maxPayload)
	}
	return nil
}

// PrepareDecode заполняет только пути к внешним утилитам.
func (o *Options) PrepareDecode() {
	o.FFmpeg, o.FFProbe, _ = ffmpeg.Resolve(o.FFmpeg, o.FFProbe)
}
