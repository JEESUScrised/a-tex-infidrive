package contentcrypto

import (
	"bytes"
	"testing"
)

func TestSealOpen(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	box, err := NewBox(key)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("college-demo-payload")
	sealed, err := box.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(sealed, plain) {
		t.Fatal("must encrypt")
	}
	out, err := box.Open(sealed)
	if err != nil || !bytes.Equal(out, plain) {
		t.Fatalf("%q %v", out, err)
	}
}
