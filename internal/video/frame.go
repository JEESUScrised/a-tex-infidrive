package video

import (
	"encoding/binary"
	"errors"
	"hash/crc32"

	"ytdrive/internal/format"
)

var (
	// ErrFrame означает, что картинка не похожа на целый кадр YTDrive.
	// Декодер пропускает такие кадры: это может быть чёрный кадр кодека.
	ErrFrame = errors.New("кадр не является кадром YTDrive")
	// ErrUnsupported означает, что кадр наш, но режим пикселей ещё не реализован.
	ErrUnsupported = errors.New("неподдерживаемый режим кадра")
)

// Image — один несжатый кадр RGB24.
type Image struct {
	Width  int
	Height int
	Pix    []byte
}

// FrameHeader — служебные поля, которые повторяются в каждом кадре.
type FrameHeader struct {
	Version      byte
	PixelMode    byte
	Flags        uint16
	FileID       uint32
	FrameIndex   uint32
	FrameCount   uint32
	PayloadLen   uint16
	ChunkSize    uint16
	BlockSize    uint16
	Cols         uint16
	Rows         uint16
	ParityGroup  uint16
	PacketRepeat uint16
}

// Marshal кодирует заголовок и дописывает CRC32 по первым 36 байтам.
func (h FrameHeader) Marshal() []byte {
	buf := make([]byte, HeaderBytes)
	binary.BigEndian.PutUint32(buf[0:], Magic)
	ver := h.Version
	if ver == 0 {
		ver = format.Version
	}
	buf[4] = ver
	buf[5] = h.PixelMode
	binary.BigEndian.PutUint16(buf[6:], h.Flags)
	binary.BigEndian.PutUint32(buf[8:], h.FileID)
	binary.BigEndian.PutUint32(buf[12:], h.FrameIndex)
	binary.BigEndian.PutUint32(buf[16:], h.FrameCount)
	binary.BigEndian.PutUint16(buf[20:], h.PayloadLen)
	binary.BigEndian.PutUint16(buf[22:], h.ChunkSize)
	binary.BigEndian.PutUint16(buf[24:], h.BlockSize)
	binary.BigEndian.PutUint16(buf[26:], h.Cols)
	binary.BigEndian.PutUint16(buf[28:], h.Rows)
	binary.BigEndian.PutUint16(buf[30:], h.ParityGroup)
	binary.BigEndian.PutUint16(buf[32:], h.PacketRepeat)
	sum := crc32.ChecksumIEEE(buf[:36])
	binary.BigEndian.PutUint32(buf[36:], sum)
	return buf
}

func parseHeader(b []byte) (FrameHeader, error) {
	if len(b) < HeaderBytes {
		return FrameHeader{}, ErrFrame
	}
	if binary.BigEndian.Uint32(b[0:]) != Magic {
		return FrameHeader{}, ErrFrame
	}
	sum := crc32.ChecksumIEEE(b[:36])
	if sum != binary.BigEndian.Uint32(b[36:]) {
		return FrameHeader{}, ErrFrame
	}
	h := FrameHeader{
		Version:      b[4],
		PixelMode:    b[5],
		Flags:        binary.BigEndian.Uint16(b[6:]),
		FileID:       binary.BigEndian.Uint32(b[8:]),
		FrameIndex:   binary.BigEndian.Uint32(b[12:]),
		FrameCount:   binary.BigEndian.Uint32(b[16:]),
		PayloadLen:   binary.BigEndian.Uint16(b[20:]),
		ChunkSize:    binary.BigEndian.Uint16(b[22:]),
		BlockSize:    binary.BigEndian.Uint16(b[24:]),
		Cols:         binary.BigEndian.Uint16(b[26:]),
		Rows:         binary.BigEndian.Uint16(b[28:]),
		ParityGroup:  binary.BigEndian.Uint16(b[30:]),
		PacketRepeat: binary.BigEndian.Uint16(b[32:]),
	}
	if h.Version != format.Version || (h.PixelMode != format.PixelLuma1 && h.PixelMode != format.PixelDCT) {
		return FrameHeader{}, ErrUnsupported
	}
	if h.PixelMode != format.PixelLuma1 {
		return FrameHeader{}, ErrUnsupported
	}
	return h, nil
}

// Render рисует кадр логического размера. Биты 0 и 1 — два контрастных уровня яркости.
func Render(layout Layout, hdr FrameHeader, payload []byte, luma0, luma1 byte) (Image, error) {
	if err := layout.Validate(); err != nil {
		return Image{}, err
	}
	if int(hdr.PayloadLen) != len(payload) {
		return Image{}, errors.New("длина полезной нагрузки не совпадает с заголовком")
	}
	if len(payload) > layout.PayloadCap() {
		return Image{}, errors.New("полезная нагрузка не помещается в кадр")
	}
	if uint16(layout.Cols) != hdr.Cols || uint16(layout.Rows) != hdr.Rows || uint16(layout.Block) != hdr.BlockSize {
		return Image{}, errors.New("заголовок не совпадает с сеткой кадра")
	}
	img := Image{
		Width:  layout.Width,
		Height: layout.Height,
		Pix:    make([]byte, layout.Width*layout.Height*3),
	}
	fill(img, luma0)
	drawCalibration(img, layout, luma0, luma1)
	writeBits(img, layout, 0, hdr.Marshal(), luma0, luma1)
	if len(payload) > 0 {
		writeBits(img, layout, layout.PayloadOffsetBits(), payload, luma0, luma1)
	}
	return img, nil
}

// Extract читает заголовок и полезную нагрузку.
// Фактический размер картинки может отличаться от логической сетки:
// блоки равномерно проецируются на img.Width x img.Height.
func Extract(img Image, layout Layout) (FrameHeader, []byte, error) {
	if err := layout.Validate(); err != nil {
		return FrameHeader{}, nil, err
	}
	if len(img.Pix) != img.Width*img.Height*3 || img.Width < layout.Cols || img.Height < layout.Rows {
		return FrameHeader{}, nil, ErrFrame
	}
	black := meanBlocks(img, layout, 0, 0, 4)
	white := meanBlocks(img, layout, 0, 4, 4)
	if white < black+24 {
		return FrameHeader{}, nil, ErrFrame
	}
	threshold := (black + white) / 2
	headerBytes := readBits(img, layout, 0, HeaderBytes, threshold)
	hdr, err := parseHeader(headerBytes)
	if err != nil {
		return FrameHeader{}, nil, err
	}
	if int(hdr.Cols) != layout.Cols || int(hdr.Rows) != layout.Rows {
		return FrameHeader{}, nil, ErrFrame
	}
	if int(hdr.PayloadLen) > layout.PayloadCap() || hdr.ChunkSize == 0 {
		return FrameHeader{}, nil, ErrFrame
	}
	payload := readBits(img, layout, layout.PayloadOffsetBits(), int(hdr.PayloadLen), threshold)
	return hdr, payload, nil
}

func fill(img Image, luma byte) {
	for i := 0; i < len(img.Pix); i++ {
		img.Pix[i] = luma
	}
}

func drawCalibration(img Image, layout Layout, luma0, luma1 byte) {
	for col := 0; col < layout.Cols; col++ {
		luma := luma0
		switch {
		case col < 4:
			luma = luma0
		case col < 8:
			luma = luma1
		default:
			if ((col-8)/4)%2 == 0 {
				luma = luma1
			}
		}
		fillBlock(img, layout, 0, col, luma)
	}
}

func writeBits(img Image, layout Layout, bit0 int, data []byte, luma0, luma1 byte) {
	bit := bit0
	for _, b := range data {
		for shift := 7; shift >= 0; shift-- {
			luma := luma0
			if b&(1<<uint(shift)) != 0 {
				luma = luma1
			}
			row := 1 + bit/layout.Cols
			col := bit % layout.Cols
			fillBlock(img, layout, row, col, luma)
			bit++
		}
	}
}

func readBits(img Image, layout Layout, bit0, nbytes int, threshold float64) []byte {
	out := make([]byte, nbytes)
	bit := bit0
	for i := 0; i < nbytes; i++ {
		var value byte
		for shift := 7; shift >= 0; shift-- {
			row := 1 + bit/layout.Cols
			col := bit % layout.Cols
			if blockMean(img, layout, row, col) >= threshold {
				value |= 1 << uint(shift)
			}
			bit++
		}
		out[i] = value
	}
	return out
}

func fillBlock(img Image, layout Layout, row, col int, luma byte) {
	x0, y0, x1, y1 := blockRect(img.Width, img.Height, layout.Cols, layout.Rows, col, row)
	for y := y0; y < y1; y++ {
		rowOff := y * img.Width * 3
		for x := x0; x < x1; x++ {
			o := rowOff + x*3
			img.Pix[o] = luma
			img.Pix[o+1] = luma
			img.Pix[o+2] = luma
		}
	}
}

func meanBlocks(img Image, layout Layout, row, col, count int) float64 {
	var sum float64
	for i := 0; i < count; i++ {
		sum += blockMean(img, layout, row, col+i)
	}
	return sum / float64(count)
}

func blockMean(img Image, layout Layout, row, col int) float64 {
	x0, y0, x1, y1 := blockRect(img.Width, img.Height, layout.Cols, layout.Rows, col, row)
	w := x1 - x0
	h := y1 - y0
	ix := w / 4
	iy := h / 4
	if w-2*ix < 1 {
		ix = 0
	}
	if h-2*iy < 1 {
		iy = 0
	}
	var sum float64
	var n int
	for y := y0 + iy; y < y1-iy; y++ {
		rowOff := y * img.Width * 3
		for x := x0 + ix; x < x1-ix; x++ {
			o := rowOff + x*3
			sum += 0.299*float64(img.Pix[o]) + 0.587*float64(img.Pix[o+1]) + 0.114*float64(img.Pix[o+2])
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func blockRect(width, height, cols, rows, col, row int) (x0, y0, x1, y1 int) {
	x0 = col * width / cols
	x1 = (col + 1) * width / cols
	y0 = row * height / rows
	y1 = (row + 1) * height / rows
	if x1 <= x0 {
		x1 = x0 + 1
		if x1 > width {
			x1 = width
		}
	}
	if y1 <= y0 {
		y1 = y0 + 1
		if y1 > height {
			y1 = height
		}
	}
	return x0, y0, x1, y1
}
