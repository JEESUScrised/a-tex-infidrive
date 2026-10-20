package video

import (
	"bytes"
	"errors"
	"testing"

	"ytdrive/internal/format"
)

func testHeader(layout Layout, payloadLen int) FrameHeader {
	return FrameHeader{
		Version:      format.Version,
		PixelMode:    format.PixelLuma1,
		Flags:        format.WithFEC(0, format.FECXOR),
		FileID:       0x11223344,
		FrameIndex:   3,
		FrameCount:   10,
		PayloadLen:   uint16(payloadLen),
		ChunkSize:    360,
		BlockSize:    uint16(layout.Block),
		Cols:         uint16(layout.Cols),
		Rows:         uint16(layout.Rows),
		ParityGroup:  8,
		PacketRepeat: 1,
	}
}

func TestLayoutCapacity(t *testing.T) {
	l, err := NewLayout(1280, 720, 16)
	if err != nil {
		t.Fatal(err)
	}
	if l.Cols != 80 || l.Rows != 45 {
		t.Fatalf("grid %dx%d", l.Cols, l.Rows)
	}
	if l.HeaderRows() != 4 {
		t.Fatalf("header rows %d", l.HeaderRows())
	}
	if l.PayloadCap() != 400 {
		t.Fatalf("cap %d", l.PayloadCap())
	}
}

func TestRenderExtract(t *testing.T) {
	l, err := NewLayout(1280, 720, 16)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{0xA5, 0x00, 0xFF, 0x5A}, 90)
	hdr := testHeader(l, len(payload))
	img, err := Render(l, hdr, payload, 16, 235)
	if err != nil {
		t.Fatal(err)
	}
	gotH, gotP, err := Extract(img, l)
	if err != nil {
		t.Fatal(err)
	}
	if gotH.FileID != hdr.FileID || gotH.FrameIndex != hdr.FrameIndex || gotH.PayloadLen != hdr.PayloadLen {
		t.Fatalf("header %+v", gotH)
	}
	if !bytes.Equal(gotP, payload) {
		t.Fatal("payload mismatch")
	}
}

func TestLevelShiftStillDecodes(t *testing.T) {
	l, _ := NewLayout(1280, 720, 16)
	payload := []byte{0x0F, 0xF0, 0x55, 0xAA}
	hdr := testHeader(l, len(payload))
	img, err := Render(l, hdr, payload, 16, 235)
	if err != nil {
		t.Fatal(err)
	}
	for i := range img.Pix {
		v := int(img.Pix[i]) + 18
		if v > 255 {
			v = 255
		}
		img.Pix[i] = byte(v)
	}
	_, got, err := Extract(img, l)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("shift broke decode")
	}
}

func TestDownscaleTo640x360(t *testing.T) {
	l, _ := NewLayout(1280, 720, 16)
	payload := bytes.Repeat([]byte("YTDrive-v1"), 20)
	hdr := testHeader(l, len(payload))
	img, err := Render(l, hdr, payload, 16, 235)
	if err != nil {
		t.Fatal(err)
	}
	small := boxDownscale(img, 640, 360)
	_, got, err := Extract(small, l)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("downscale broke decode")
	}
}

func TestDamagedHeaderSkipped(t *testing.T) {
	l, _ := NewLayout(1280, 720, 16)
	payload := []byte{1, 2, 3, 4}
	hdr := testHeader(l, len(payload))
	img, err := Render(l, hdr, payload, 16, 235)
	if err != nil {
		t.Fatal(err)
	}
	// Инвертируем один блок заголовка, иначе можно случайно записать ту же яркость.
	origin := (1*l.Block)*img.Width*3 + (10*l.Block)*3
	flipped := byte(235)
	if img.Pix[origin] > 128 {
		flipped = 16
	}
	fillBlock(img, l, 1, 10, flipped)
	_, _, err = Extract(img, l)
	if !errors.Is(err, ErrFrame) {
		t.Fatalf("expected bad frame, got %v", err)
	}
}

func TestUnsupportedPixelMode(t *testing.T) {
	l, _ := NewLayout(1280, 720, 16)
	hdr := testHeader(l, 0)
	hdr.PixelMode = format.PixelDCT
	img, err := Render(l, hdr, nil, 16, 235)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = Extract(img, l)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("got %v", err)
	}
}

func boxDownscale(src Image, dw, dh int) Image {
	dst := Image{Width: dw, Height: dh, Pix: make([]byte, dw*dh*3)}
	for y := 0; y < dh; y++ {
		y0 := y * src.Height / dh
		y1 := (y + 1) * src.Height / dh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < dw; x++ {
			x0 := x * src.Width / dw
			x1 := (x + 1) * src.Width / dw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, n int
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					o := (yy*src.Width + xx) * 3
					r += int(src.Pix[o])
					g += int(src.Pix[o+1])
					b += int(src.Pix[o+2])
					n++
				}
			}
			o := (y*dw + x) * 3
			dst.Pix[o] = byte(r / n)
			dst.Pix[o+1] = byte(g / n)
			dst.Pix[o+2] = byte(b / n)
		}
	}
	return dst
}
