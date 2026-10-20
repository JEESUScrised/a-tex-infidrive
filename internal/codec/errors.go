package codec

import "errors"

var (
	// ErrIncomplete — после проверки копий и XOR не хватает пакетов.
	ErrIncomplete = errors.New("недостаточно данных для восстановления файла")
	// ErrChecksum — восстановленные байты не совпали с SHA-256 из метаданных.
	ErrChecksum = errors.New("SHA-256 восстановленного файла не совпадает")
	// ErrNoSignal — ни один кадр не распознан как YTDrive.
	ErrNoSignal = errors.New("в видео не найдены кадры YTDrive")
)
