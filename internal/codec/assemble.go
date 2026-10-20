package codec

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"ytdrive/internal/packets"
)

const maxStoredBytes int64 = 8 << 30

// assembler складывает пакеты на диск: в памяти только битовые карты и метаданные.
type assembler struct {
	dir        string
	data       *os.File
	parity     *os.File
	chunk      int
	group      int
	fileID     uint32
	have       []byte
	haveParity []byte
	metaFrags  map[uint32][]byte
	metaTotal  uint32
	corrupt    int
	recovered  int
}

func newAssembler(chunk, group int, fileID uint32) (*assembler, error) {
	if chunk <= 0 {
		return nil, errf("некорректный размер блока данных")
	}
	dir, err := os.MkdirTemp("", "ytdrive-dec-*")
	if err != nil {
		return nil, err
	}
	data, err := os.Create(filepath.Join(dir, "data.bin"))
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	parity, err := os.Create(filepath.Join(dir, "parity.bin"))
	if err != nil {
		data.Close()
		os.RemoveAll(dir)
		return nil, err
	}
	return &assembler{
		dir:       dir,
		data:      data,
		parity:    parity,
		chunk:     chunk,
		group:     group,
		fileID:    fileID,
		metaFrags: map[uint32][]byte{},
	}, nil
}

func (a *assembler) cleanup() {
	if a.data != nil {
		a.data.Close()
	}
	if a.parity != nil {
		a.parity.Close()
	}
	if a.dir != "" {
		os.RemoveAll(a.dir)
	}
}

func (a *assembler) add(p packets.Packet) error {
	if p.FileID != a.fileID {
		a.corrupt++
		return nil
	}
	switch p.Type {
	case packets.TypeMeta:
		a.addMeta(p)
	case packets.TypeData:
		return a.addData(p)
	case packets.TypeParity:
		return a.addParity(p)
	default:
		a.corrupt++
	}
	return nil
}

func (a *assembler) addMeta(p packets.Packet) {
	if p.Total == 0 || p.Index >= p.Total || p.Total > 4096 {
		a.corrupt++
		return
	}
	if a.metaTotal == 0 {
		a.metaTotal = p.Total
	} else if a.metaTotal != p.Total {
		a.corrupt++
		return
	}
	if _, ok := a.metaFrags[p.Index]; ok {
		return
	}
	a.metaFrags[p.Index] = append([]byte(nil), p.Payload...)
}

func (a *assembler) addData(p packets.Packet) error {
	if a.has(p.Index) {
		return nil
	}
	if int64(p.Index)*int64(a.chunk) >= maxStoredBytes || len(p.Payload) > a.chunk {
		a.corrupt++
		return nil
	}
	if _, err := a.data.WriteAt(p.Payload, int64(p.Index)*int64(a.chunk)); err != nil {
		return err
	}
	a.mark(p.Index, &a.have)
	return nil
}

func (a *assembler) addParity(p packets.Packet) error {
	if a.group <= 0 {
		return nil
	}
	if a.hasBit(a.haveParity, p.Index) {
		return nil
	}
	if len(p.Payload) != a.chunk || int64(p.Index)*int64(a.chunk) >= maxStoredBytes {
		a.corrupt++
		return nil
	}
	if _, err := a.parity.WriteAt(p.Payload, int64(p.Index)*int64(a.chunk)); err != nil {
		return err
	}
	a.mark(p.Index, &a.haveParity)
	return nil
}

func (a *assembler) finish(dest string) (packets.Metadata, error) {
	meta, err := a.loadMeta()
	if err != nil {
		return packets.Metadata{}, err
	}
	if err := a.recover(meta); err != nil {
		return packets.Metadata{}, err
	}
	if err := a.data.Truncate(int64(meta.StoredSize)); err != nil {
		return packets.Metadata{}, err
	}
	if err := a.data.Sync(); err != nil {
		return packets.Metadata{}, err
	}
	dataPath := a.data.Name()
	if err := a.data.Close(); err != nil {
		return packets.Metadata{}, err
	}
	a.data = nil
	if err := a.parity.Close(); err != nil {
		return packets.Metadata{}, err
	}
	a.parity = nil

	plain := dataPath
	if meta.Flags&1 != 0 {
		plain = filepath.Join(a.dir, "plain.bin")
		if err := gunzipFile(dataPath, plain); err != nil {
			return packets.Metadata{}, errf("распаковка gzip: %w", err)
		}
	}
	sum, err := fileSHA(plain)
	if err != nil {
		return packets.Metadata{}, err
	}
	if sum != meta.SHA256 {
		return packets.Metadata{}, ErrChecksum
	}
	if err := replaceFile(plain, dest); err != nil {
		return packets.Metadata{}, err
	}
	return meta, nil
}

func (a *assembler) loadMeta() (packets.Metadata, error) {
	if a.metaTotal == 0 {
		return packets.Metadata{}, fmt.Errorf("%w: нет метаданных", ErrIncomplete)
	}
	var buf bytes.Buffer
	for i := uint32(0); i < a.metaTotal; i++ {
		frag, ok := a.metaFrags[i]
		if !ok {
			return packets.Metadata{}, fmt.Errorf("%w: нет фрагмента метаданных %d", ErrIncomplete, i)
		}
		buf.Write(frag)
	}
	meta, err := packets.ParseMeta(buf.Bytes())
	if err != nil {
		return packets.Metadata{}, err
	}
	if int(meta.ChunkSize) != a.chunk {
		return packets.Metadata{}, errf("размер блока в метаданных не совпадает с кадром")
	}
	if meta.StoredSize > uint64(maxStoredBytes) {
		return packets.Metadata{}, errf("хранимый размер превышает ограничение")
	}
	return meta, nil
}

func (a *assembler) recover(meta packets.Metadata) error {
	total := 0
	if meta.StoredSize > 0 {
		total = int((meta.StoredSize + uint64(a.chunk) - 1) / uint64(a.chunk))
	}
	missing := make([]int, 0)
	for i := 0; i < total; i++ {
		if !a.has(uint32(i)) {
			missing = append(missing, i)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if a.group <= 0 {
		return fmt.Errorf("%w: отсутствует %d пакетов, первый %d", ErrIncomplete, len(missing), missing[0])
	}
	still := make([]int, 0)
	for _, index := range missing {
		if a.has(uint32(index)) {
			continue
		}
		gid := index / a.group
		lost := 0
		start := gid * a.group
		end := start + a.group
		if end > total {
			end = total
		}
		for j := start; j < end; j++ {
			if !a.has(uint32(j)) {
				lost++
			}
		}
		if lost == 1 && a.hasBit(a.haveParity, uint32(gid)) {
			if err := a.recoverOne(gid, index, total, meta.StoredSize); err != nil {
				return err
			}
			continue
		}
		still = append(still, index)
	}
	if len(still) > 0 {
		return fmt.Errorf("%w: отсутствует %d пакетов, первый %d", ErrIncomplete, len(still), still[0])
	}
	return nil
}

func (a *assembler) recoverOne(gid, index, total int, stored uint64) error {
	buf := make([]byte, a.chunk)
	if _, err := a.parity.ReadAt(buf, int64(gid)*int64(a.chunk)); err != nil {
		return err
	}
	start := gid * a.group
	end := start + a.group
	if end > total {
		end = total
	}
	for j := start; j < end; j++ {
		if j == index {
			continue
		}
		plen := packetLen(j, total, a.chunk, stored)
		part := make([]byte, plen)
		if _, err := a.data.ReadAt(part, int64(j)*int64(a.chunk)); err != nil {
			return err
		}
		for k := range part {
			buf[k] ^= part[k]
		}
	}
	want := packetLen(index, total, a.chunk, stored)
	if _, err := a.data.WriteAt(buf[:want], int64(index)*int64(a.chunk)); err != nil {
		return err
	}
	a.mark(uint32(index), &a.have)
	a.recovered++
	return nil
}

func packetLen(index, total, chunk int, stored uint64) int {
	if index < total-1 {
		return chunk
	}
	if rem := int(stored % uint64(chunk)); rem != 0 {
		return rem
	}
	return chunk
}

func (a *assembler) has(i uint32) bool {
	return a.hasBit(a.have, i)
}

func (a *assembler) hasBit(bits []byte, i uint32) bool {
	b := i / 8
	if int(b) >= len(bits) {
		return false
	}
	return bits[b]&(1<<(i%8)) != 0
}

func (a *assembler) mark(i uint32, bits *[]byte) {
	b := int(i / 8)
	for len(*bits) <= b {
		*bits = append(*bits, 0)
	}
	(*bits)[b] |= 1 << (i % 8)
}

func gunzipFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	zr, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer zr.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, zr)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(dst)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(dst)
		return closeErr
	}
	return nil
}
