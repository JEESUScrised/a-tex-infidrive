// Package format хранит константы самостоятельного формата YTDrive v1.
//
// Идея хранения файла в видео подсмотрена у проекта PulseBeat02/yt-media-storage
// (GPL-3.0): пакеты, контрольные суммы, один бит на блок. Раскладка байтов,
// коды и алгоритм декодирования здесь другие и написаны заново: исходный
// проект рассчитан на lossless FFV1, а YTDrive v1 должен переживать H.264.
package format

// Version — YTDrive v1 (720p, block 16, высокая избыточность).
const Version byte = 1

// Version2 — экспериментальный плотный профиль (1080p, block 8, меньше повторов).
// Раскладка бит/блок та же (PixelLuma1); отличается геометрией и FEC-параметрами в заголовке.
const Version2 byte = 2

// Version3 — Studio-ориентированный профиль (нативные 1280×720).
// Та же раскладка PixelLuma1; геометрия совпадает с официальным Download YouTube Studio (720p/360p).
const Version3 byte = 3

// SupportedFrameVersion — true для версий, которые умеет декодировать текущий бинарник.
func SupportedFrameVersion(v byte) bool {
	return v == Version || v == Version2 || v == Version3
}

// Флаги кадра и пакета. Младший бит — gzip полезной нагрузки.
// Биты 1–3 кодируют режим избыточности и оставлены расширяемыми:
// значения 4 и 5 зарезервированы под Reed-Solomon и fountain codes.
const (
	FlagGzip uint16 = 1 << 0

	FECShift = 1
	FECMask  = uint16(0x7) << FECShift

	FECNone        = 0
	FECRepeat      = 1
	FECXOR         = 2
	FECRepeatXOR   = 3
	FECReedSolomon = 4
	FECFountain    = 5
)

// Режим пикселей. Сейчас реализован только чёрно-белый блок = 1 бит.
// Значение 1 зарезервировано под будущее DCT-кодирование.
const (
	PixelLuma1 byte = 0
	PixelDCT   byte = 1
)

// FEC возвращает режим избыточности из поля флагов.
func FEC(flags uint16) int {
	return int((flags & FECMask) >> FECShift)
}

// WithFEC записывает режим избыточности, не трогая остальные флаги.
func WithFEC(flags uint16, mode int) uint16 {
	flags &^= FECMask
	flags |= (uint16(mode) << FECShift) & FECMask
	return flags
}
