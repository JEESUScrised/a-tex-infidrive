package codec

import "ytdrive/internal/packets"

// item — один логический пакет (включая копии) без полезной нагрузки.
type item struct {
	Type    byte
	Index   uint32
	Total   uint32
	Replica uint16
	Group   uint32
	Length  int
}

// iterate обходит пакеты в том же порядке, в каком они попадут в кадры.
// Повторы идут подряд, XOR-пакет закрывает группу, в том числе неполную последнюю.
func iterate(metaLen int, stored int64, chunk, group, repeat int, fn func(item) error) error {
	if chunk <= 0 || repeat <= 0 {
		return errf("некорректные параметры пакетов")
	}
	frags := fragLens(metaLen, chunk)
	for i, n := range frags {
		for r := 0; r < repeat; r++ {
			if err := fn(item{
				Type:    packets.TypeMeta,
				Index:   uint32(i),
				Total:   uint32(len(frags)),
				Replica: uint16(r),
				Length:  n,
			}); err != nil {
				return err
			}
		}
	}
	if stored == 0 {
		return nil
	}
	total := int((stored + int64(chunk) - 1) / int64(chunk))
	groups := 0
	if group > 0 {
		groups = (total + group - 1) / group
	}
	for i := 0; i < total; i++ {
		plen := chunk
		if i == total-1 {
			if rem := int(stored % int64(chunk)); rem != 0 {
				plen = rem
			}
		}
		gid := uint32(0)
		if group > 0 {
			gid = uint32(i / group)
		}
		for r := 0; r < repeat; r++ {
			if err := fn(item{
				Type:    packets.TypeData,
				Index:   uint32(i),
				Total:   uint32(total),
				Replica: uint16(r),
				Group:   gid,
				Length:  plen,
			}); err != nil {
				return err
			}
		}
		end := group > 0 && ((i+1)%group == 0 || i == total-1)
		if end {
			for r := 0; r < repeat; r++ {
				if err := fn(item{
					Type:    packets.TypeParity,
					Index:   gid,
					Total:   uint32(groups),
					Replica: uint16(r),
					Group:   gid,
					Length:  chunk,
				}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func fragLens(n, chunk int) []int {
	if n <= 0 {
		return nil
	}
	var out []int
	for n > 0 {
		l := chunk
		if l > n {
			l = n
		}
		out = append(out, l)
		n -= l
	}
	return out
}

func countPlan(metaLen int, stored int64, opt Options, capacity int) (frames, pkts int, err error) {
	used := 0
	err = iterate(metaLen, stored, opt.ChunkSize, opt.ParityGroup, opt.PacketRepeat, func(it item) error {
		sz := packets.EncodedSize(it.Length)
		if sz > capacity {
			return errf("пакет %d байт больше ёмкости кадра %d", sz, capacity)
		}
		if used > 0 && used+sz > capacity {
			frames++
			used = 0
		}
		used += sz
		pkts++
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	if used > 0 {
		frames++
	}
	return frames, pkts, nil
}
