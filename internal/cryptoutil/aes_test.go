package cryptoutil

import (
	"bytes"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	box, err := NewBox(key)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Seal("refresh-token-secret")
	if err != nil {
		t.Fatal(err)
	}
	if sealed == "refresh-token-secret" || sealed == "" {
		t.Fatalf("expected ciphertext, got %q", sealed)
	}
	out, err := box.Open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if out != "refresh-token-secret" {
		t.Fatalf("got %q", out)
	}
}

func TestLegacyPlaintext(t *testing.T) {
	box, err := NewBox(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	out, err := box.Open("legacy-plain-token")
	if err != nil || out != "legacy-plain-token" {
		t.Fatalf("%q %v", out, err)
	}
}

func TestNilKeyPassthrough(t *testing.T) {
	box, err := NewBox(nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := box.Seal("x")
	if err != nil || s != "x" {
		t.Fatalf("%q %v", s, err)
	}
}
