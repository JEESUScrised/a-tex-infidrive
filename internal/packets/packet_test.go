package packets

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"testing"

	"ytdrive/internal/format"
)

func TestPacketRoundTripAndCorruption(t *testing.T) {
	original := Packet{
		Type:    TypeData,
		Flags:   format.WithFEC(format.FlagGzip, format.FECRepeatXOR),
		FileID:  0xA1B2C3D4,
		Index:   7,
		Total:   20,
		Replica: 1,
		Group:   3,
		Payload: []byte{0x00, 0xFF, 0x10, 0x80},
	}
	raw := original.Marshal()
	got, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != original.Type || got.Flags != original.Flags || got.FileID != original.FileID ||
		got.Index != original.Index || got.Total != original.Total || got.Replica != original.Replica ||
		got.Group != original.Group || !bytes.Equal(got.Payload, original.Payload) {
		t.Fatalf("parsed packet mismatch: %+v", got)
	}

	raw[PrefixSize] ^= 0xFF
	_, err = Parse(raw)
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("expected CRC error, got %v", err)
	}
}

func TestSplitStopsOnCorruptPacket(t *testing.T) {
	a := Packet{Type: TypeData, Index: 0, Total: 2, Payload: []byte("ok")}.Marshal()
	b := Packet{Type: TypeData, Index: 1, Total: 2, Payload: []byte("bad")}.Marshal()
	b[len(b)-1] ^= 0x01
	buf := append(append([]byte{}, a...), b...)
	pkts, corrupt := Split(buf)
	if len(pkts) != 1 || corrupt != 1 {
		t.Fatalf("pkts=%d corrupt=%d", len(pkts), corrupt)
	}
	if string(pkts[0].Payload) != "ok" {
		t.Fatalf("payload %q", pkts[0].Payload)
	}
}

func TestSplitIgnoresZeroPadding(t *testing.T) {
	a := Packet{Type: TypeMeta, Index: 0, Total: 1, Payload: []byte("m")}.Marshal()
	buf := append(a, 0, 0, 0, 0, 0, 0, 0, 0)
	pkts, corrupt := Split(buf)
	if len(pkts) != 1 || corrupt != 0 {
		t.Fatalf("pkts=%d corrupt=%d", len(pkts), corrupt)
	}
}

func TestMetadataRoundTrip(t *testing.T) {
	sum := sha256.Sum256([]byte("привет"))
	meta := Metadata{
		Flags:        format.FlagGzip,
		ChunkSize:    360,
		Name:         "отчёт.txt",
		MIME:         "text/plain",
		OriginalSize: 12,
		StoredSize:   40,
		SHA256:       sum,
	}
	raw, err := meta.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseMeta(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != meta.Name || got.MIME != meta.MIME || got.Flags != meta.Flags ||
		got.ChunkSize != meta.ChunkSize || got.OriginalSize != meta.OriginalSize ||
		got.StoredSize != meta.StoredSize || got.SHA256 != meta.SHA256 {
		t.Fatalf("%+v", got)
	}
}

func TestEmptyPayloadPacket(t *testing.T) {
	raw := Packet{Type: TypeData, Index: 0, Total: 1}.Marshal()
	got, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Payload) != 0 {
		t.Fatalf("len=%d", len(got.Payload))
	}
}
