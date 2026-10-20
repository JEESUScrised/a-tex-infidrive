package packets

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// MagicMeta — сигнатура блока метаданных "YTM1".
const MagicMeta uint32 = 0x59544D31

// MetaHeaderSize — фиксированная часть метаданных до имени и MIME.
const MetaHeaderSize = 64

var (
	ErrMetaMagic   = errors.New("неверная сигнатура метаданных")
	ErrMetaVersion = errors.New("неподдерживаемая версия метаданных")
	ErrMetaShort   = errors.New("метаданные обрезаны")
	ErrMetaSize    = errors.New("некорректный размер в метаданных")
)

// Metadata описывает исходный файл. SHA-256 всегда считается до gzip.
type Metadata struct {
	Flags        uint16
	ChunkSize    uint16
	Name         string
	MIME         string
	OriginalSize uint64
	StoredSize   uint64
	SHA256       [32]byte
}

// Marshal кодирует метаданные в самоописываемый буфер.
func (m Metadata) Marshal() ([]byte, error) {
	if m.Name == "" {
		m.Name = "file.bin"
	}
	if m.MIME == "" {
		m.MIME = "application/octet-stream"
	}
	if m.ChunkSize == 0 {
		return nil, ErrMetaSize
	}
	name := []byte(m.Name)
	mime := []byte(m.MIME)
	if len(name) > 1024 || len(mime) > 256 {
		return nil, fmt.Errorf("имя или MIME слишком длинные")
	}
	buf := make([]byte, MetaHeaderSize+len(name)+len(mime))
	binary.BigEndian.PutUint32(buf[0:], MagicMeta)
	buf[4] = 1
	binary.BigEndian.PutUint16(buf[6:], m.Flags)
	binary.BigEndian.PutUint16(buf[8:], m.ChunkSize)
	binary.BigEndian.PutUint16(buf[10:], uint16(len(name)))
	binary.BigEndian.PutUint16(buf[12:], uint16(len(mime)))
	binary.BigEndian.PutUint64(buf[16:], m.OriginalSize)
	binary.BigEndian.PutUint64(buf[24:], m.StoredSize)
	copy(buf[32:64], m.SHA256[:])
	copy(buf[64:], name)
	copy(buf[64+len(name):], mime)
	return buf, nil
}

// ParseMeta разбирает буфер, собранный из мета-пакетов.
func ParseMeta(b []byte) (Metadata, error) {
	if len(b) < MetaHeaderSize {
		return Metadata{}, ErrMetaShort
	}
	if binary.BigEndian.Uint32(b[0:]) != MagicMeta {
		return Metadata{}, ErrMetaMagic
	}
	if b[4] != 1 {
		return Metadata{}, ErrMetaVersion
	}
	nameLen := int(binary.BigEndian.Uint16(b[10:]))
	mimeLen := int(binary.BigEndian.Uint16(b[12:]))
	if len(b) < MetaHeaderSize+nameLen+mimeLen {
		return Metadata{}, ErrMetaShort
	}
	chunk := binary.BigEndian.Uint16(b[8:])
	if chunk == 0 {
		return Metadata{}, ErrMetaSize
	}
	var sum [32]byte
	copy(sum[:], b[32:64])
	return Metadata{
		Flags:        binary.BigEndian.Uint16(b[6:]),
		ChunkSize:    chunk,
		Name:         string(b[64 : 64+nameLen]),
		MIME:         string(b[64+nameLen : 64+nameLen+mimeLen]),
		OriginalSize: binary.BigEndian.Uint64(b[16:]),
		StoredSize:   binary.BigEndian.Uint64(b[24:]),
		SHA256:       sum,
	}, nil
}
