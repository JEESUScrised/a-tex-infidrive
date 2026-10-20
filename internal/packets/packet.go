// Package packets реализует пакеты YTDrive v1: заголовок, CRC32 и разбор
// полезной нагрузки кадра. Повреждённый пакет отбрасывается целиком.
package packets

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
)

const (
	// Magic — четыре байта "YTP1".
	Magic uint32 = 0x59545031

	PrefixSize = 32
	CRCSize    = 4
	Overhead   = PrefixSize + CRCSize

	TypeMeta   uint8 = 1
	TypeData   uint8 = 2
	TypeParity uint8 = 3
)

var (
	ErrMagic    = errors.New("неверная сигнатура пакета")
	ErrVersion  = errors.New("неподдерживаемая версия пакета")
	ErrChecksum = errors.New("CRC32 пакета не совпадает")
	ErrShort    = errors.New("пакет обрезан")
	ErrType     = errors.New("неизвестный тип пакета")
)

// Packet — один самодостаточный блок данных файла, его метаданных или XOR-чётности.
type Packet struct {
	Version byte
	Type    uint8
	Flags   uint16
	FileID  uint32
	Index   uint32
	Total   uint32
	Replica uint16
	Group   uint32
	Payload []byte
}

// EncodedSize возвращает размер пакета на диске и в кадре.
func EncodedSize(payloadLen int) int {
	return Overhead + payloadLen
}

// Marshal сериализует пакет. CRC32 (IEEE) считается по префиксу и полезной нагрузке.
func (p Packet) Marshal() []byte {
	payload := p.Payload
	buf := make([]byte, PrefixSize+len(payload)+CRCSize)
	binary.BigEndian.PutUint32(buf[0:], Magic)
	ver := p.Version
	if ver == 0 {
		ver = 1
	}
	buf[4] = ver
	buf[5] = p.Type
	binary.BigEndian.PutUint16(buf[6:], p.Flags)
	binary.BigEndian.PutUint32(buf[8:], p.FileID)
	binary.BigEndian.PutUint32(buf[12:], p.Index)
	binary.BigEndian.PutUint32(buf[16:], p.Total)
	binary.BigEndian.PutUint16(buf[20:], uint16(len(payload)))
	binary.BigEndian.PutUint16(buf[22:], p.Replica)
	binary.BigEndian.PutUint32(buf[24:], p.Group)
	copy(buf[PrefixSize:], payload)
	sum := crc32.ChecksumIEEE(buf[:PrefixSize+len(payload)])
	binary.BigEndian.PutUint32(buf[PrefixSize+len(payload):], sum)
	return buf
}

// Parse проверяет сигнатуру, версию, тип и CRC32.
// Лишние байты после пакета игнорируются: вызывающий сам знает границу.
func Parse(b []byte) (Packet, error) {
	if len(b) < PrefixSize+CRCSize {
		return Packet{}, ErrShort
	}
	if binary.BigEndian.Uint32(b[0:]) != Magic {
		return Packet{}, ErrMagic
	}
	if b[4] != 1 {
		return Packet{}, ErrVersion
	}
	length := int(binary.BigEndian.Uint16(b[20:]))
	total := PrefixSize + length + CRCSize
	if len(b) < total {
		return Packet{}, ErrShort
	}
	sum := crc32.ChecksumIEEE(b[:PrefixSize+length])
	got := binary.BigEndian.Uint32(b[PrefixSize+length:])
	if sum != got {
		return Packet{}, ErrChecksum
	}
	switch b[5] {
	case TypeMeta, TypeData, TypeParity:
	default:
		return Packet{}, ErrType
	}
	payload := make([]byte, length)
	copy(payload, b[PrefixSize:PrefixSize+length])
	return Packet{
		Version: b[4],
		Type:    b[5],
		Flags:   binary.BigEndian.Uint16(b[6:]),
		FileID:  binary.BigEndian.Uint32(b[8:]),
		Index:   binary.BigEndian.Uint32(b[12:]),
		Total:   binary.BigEndian.Uint32(b[16:]),
		Replica: binary.BigEndian.Uint16(b[22:]),
		Group:   binary.BigEndian.Uint32(b[24:]),
		Payload: payload,
	}, nil
}

// Split достаёт из буфера кадра идущие подряд пакеты.
// Нулевая сигнатура считается выравнивающим хвостом, а не ошибкой.
// Первая ошибка CRC или обрезка прекращает разбор: кадр дальше не синхронизирован.
func Split(payload []byte) (pkts []Packet, corrupt int) {
	i := 0
	for i+PrefixSize+CRCSize <= len(payload) {
		magic := binary.BigEndian.Uint32(payload[i:])
		if magic == 0 {
			break
		}
		if magic != Magic {
			corrupt++
			break
		}
		length := int(binary.BigEndian.Uint16(payload[i+20:]))
		total := PrefixSize + length + CRCSize
		if length < 0 || i+total > len(payload) {
			corrupt++
			break
		}
		pkt, err := Parse(payload[i : i+total])
		if err != nil {
			corrupt++
			break
		}
		pkts = append(pkts, pkt)
		i += total
	}
	return pkts, corrupt
}
