// Package video рисует и читает кадры YTDrive v1.
// Бит хранится яркостью целого блока. При чтении берётся средняя яркость
// внутренней части блока: после H.264 края блока размываются.
package video

import "fmt"

const (
	// Magic — сигнатура заголовка кадра "YT1\x01".
	Magic uint32 = 0x59543101
	// HeaderBytes — размер заголовка кадра до полезной нагрузки.
	HeaderBytes = 40
)

// Layout — логическая сетка блоков. Видео после перекодирования может иметь
// другой размер в пикселях; важно число столбцов и строк.
type Layout struct {
	Width  int
	Height int
	Block  int
	Cols   int
	Rows   int
}

// NewLayout проверяет, что разрешение делится на блок и в кадре хватает места.
func NewLayout(width, height, block int) (Layout, error) {
	if block < 4 || width < block || height < block || width%block != 0 || height%block != 0 {
		return Layout{}, fmt.Errorf("разрешение %dx%d не делится на блок %d", width, height, block)
	}
	l := Layout{
		Width:  width,
		Height: height,
		Block:  block,
		Cols:   width / block,
		Rows:   height / block,
	}
	if err := l.Validate(); err != nil {
		return Layout{}, err
	}
	return l, nil
}

// HeaderRows — сколько строк сетки занимает заголовок кадра (строка 0 — калибровка).
func (l Layout) HeaderRows() int {
	bits := HeaderBytes * 8
	return (bits + l.Cols - 1) / l.Cols
}

// PayloadOffsetBits — индекс первого бита полезной нагрузки.
func (l Layout) PayloadOffsetBits() int {
	return l.HeaderRows() * l.Cols
}

// PayloadCap — сколько байт данных помещается в один кадр.
func (l Layout) PayloadCap() int {
	dataRows := l.Rows - 1 - l.HeaderRows()
	if dataRows < 1 || l.Cols < 16 {
		return 0
	}
	return dataRows * l.Cols / 8
}

// Validate проверяет минимальный размер сетки.
func (l Layout) Validate() error {
	if l.Cols < 16 || l.Rows < l.HeaderRows()+2 || l.PayloadCap() < 64 {
		return fmt.Errorf("сетка %dx%d слишком мала для формата v1", l.Cols, l.Rows)
	}
	if l.PayloadCap() > 65535 {
		return fmt.Errorf("полезная нагрузка кадра не помещается в uint16")
	}
	return nil
}
