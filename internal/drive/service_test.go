package drive

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"ytdrive/internal/blobstore"
	"ytdrive/internal/storage"
)

func TestUploadDownloadShare(t *testing.T) {
	st, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	meta, err := Open(st.DB())
	if err != nil {
		t.Fatal(err)
	}
	blobs := blobstore.NewMemory()
	svc := NewService(meta, blobs, "https://app.a-tex-infidrive.ru", 1<<20)

	payload := []byte("cloud-file-bytes")
	f, err := svc.UploadStream(context.Background(), "", "note.txt", "text/plain", bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != "ready" || f.Size != int64(len(payload)) || f.SHA256 == "" {
		t.Fatalf("%+v", f)
	}
	// нет локального objects/
	if _, ok := blobs.BytesForTest(f.ObjectKey); !ok {
		t.Fatal("object missing in blobstore")
	}

	rc, got, err := svc.Download(context.Background(), f.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(b, payload) || got.Name != "note.txt" {
		t.Fatalf("%q %+v", b, got)
	}

	link, err := svc.CreateShare(f.ID, time.Hour, 2)
	if err != nil {
		t.Fatal(err)
	}
	if link.Token == "" || link.URL == "" {
		t.Fatalf("%+v", link)
	}
	rc2, _, _, err := svc.OpenShare(context.Background(), link.Token)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := io.ReadAll(rc2)
	rc2.Close()
	if !bytes.Equal(b2, payload) {
		t.Fatalf("%q", b2)
	}

	if err := svc.SoftDelete(f.ID); err != nil {
		t.Fatal(err)
	}
	trash, err := svc.Trash()
	if err != nil || len(trash) != 1 {
		t.Fatalf("%v %d", err, len(trash))
	}
	if err := svc.Purge(context.Background(), f.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := blobs.BytesForTest(f.ObjectKey); ok {
		t.Fatal("blob should be deleted")
	}
}
