// Package format хранит константы самостоятельного формата YTDrive v1.
//
// Идея хранения файла в видео подсмотрена у проекта PulseBeat02/yt-media-storage
// (GPL-3.0): пакеты, контрольные суммы, один бит на блок. Раскладка байтов,
// коды и алгоритм декодирования здесь другие и написаны заново: исходный
// проект рассчитан на lossless FFV1, а YTDrive v1 должен переживать H.264.
package format

// Version — единственная реализованная версия контейнера.
const Version byte = 1

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
