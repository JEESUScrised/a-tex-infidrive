package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"ytdrive/internal/blobstore"
	"ytdrive/internal/codec"
	"ytdrive/internal/config"
	"ytdrive/internal/drive"
	"ytdrive/internal/jobs"
	"ytdrive/internal/storage"
)

func TestDriveUploadDownloadShare(t *testing.T) {
	st, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	meta, err := drive.Open(st.DB())
	if err != nil {
		t.Fatal(err)
	}
	blobs := blobstore.NewMemory()
	svc := drive.NewService(meta, blobs, "http://127.0.0.1", 1<<20)
	m := jobs.New(st, codec.DefaultOptions(), 1, 2, nil)
	m.Start()
	t.Cleanup(m.Stop)
	cfg := config.Config{MaxUpload: 1 << 20, WebDir: t.TempDir(), AppName: "A-Tex InfiDrive", AppEnv: config.EnvDevelopment}
	srv := httptest.NewServer(NewWithDrive(st, m, cfg, nil, svc, nil).Handler())
	defer srv.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("file", "hello.txt")
	_, _ = part.Write([]byte("hello-drive"))
	_ = w.Close()
	up, err := http.Post(srv.URL+"/api/drive/upload", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer up.Body.Close()
	if up.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(up.Body)
		t.Fatalf("upload %d %s", up.StatusCode, b)
	}
	var file drive.File
	if err := json.NewDecoder(up.Body).Decode(&file); err != nil {
		t.Fatal(err)
	}

	dl, err := http.Get(srv.URL + "/api/drive/" + file.ID + "/download")
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Body.Close()
	body, _ := io.ReadAll(dl.Body)
	if dl.StatusCode != http.StatusOK || string(body) != "hello-drive" {
		t.Fatalf("download %d %q", dl.StatusCode, body)
	}

	shareReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/drive/"+file.ID+"/share", bytes.NewReader([]byte(`{}`)))
	shareReq.Header.Set("Content-Type", "application/json")
	shareRes, err := http.DefaultClient.Do(shareReq)
	if err != nil {
		t.Fatal(err)
	}
	defer shareRes.Body.Close()
	var link drive.ShareLink
	if err := json.NewDecoder(shareRes.Body).Decode(&link); err != nil {
		t.Fatal(err)
	}
	shareMeta, err := http.Get(srv.URL + "/api/public/share/" + link.Token)
	if err != nil {
		t.Fatal(err)
	}
	defer shareMeta.Body.Close()
	if shareMeta.StatusCode != http.StatusOK {
		t.Fatalf("share meta %d", shareMeta.StatusCode)
	}
	pub, err := http.Get(srv.URL + "/s/" + link.Token + "/download")
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Body.Close()
	pb, _ := io.ReadAll(pub.Body)
	if pub.StatusCode != http.StatusOK || string(pb) != "hello-drive" {
		t.Fatalf("share %d %q", pub.StatusCode, pb)
	}
}
