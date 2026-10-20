package blobstore

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func TestMemoryRoundTrip(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	payload := []byte("hello-infidrive")
	meta, err := m.Put(ctx, "users/a/obj1", bytes.NewReader(payload), PutOptions{ContentType: "text/plain", Size: int64(len(payload))})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Size != int64(len(payload)) {
		t.Fatalf("size %d", meta.Size)
	}
	rc, got, err := m.Get(ctx, "users/a/obj1")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if !bytes.Equal(b, payload) || got.ContentType != "text/plain" {
		t.Fatalf("%q %+v", b, got)
	}
	if err := m.Delete(ctx, "users/a/obj1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Get(ctx, "users/a/obj1"); err != ErrNotFound {
		t.Fatalf("want not found, got %v", err)
	}
}
