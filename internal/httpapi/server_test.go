package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ytdrive/internal/codec"
	"ytdrive/internal/config"
	"ytdrive/internal/jobs"
	"ytdrive/internal/storage"
)

func newTestServer(t *testing.T, max int64) *httptest.Server {
	t.Helper()
	st, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	opt := codec.DefaultOptions()
	opt.FrameRepeat = 1
	opt.PacketRepeat = 1
	opt.Preset = "ultrafast"
	m := jobs.New(st, opt, 1, 2, nil)
	m.Start()
	t.Cleanup(m.Stop)
	cfg := config.Config{MaxUpload: max, FFmpeg: "ffmpeg", FFProbe: "ffprobe", WebDir: t.TempDir()}
	return httptest.NewServer(New(st, m, cfg, nil).Handler())
}

func TestHealthUploadAndMissing(t *testing.T) {
	srv := newTestServer(t, 1024)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("health %d", res.StatusCode)
	}

	yt, err := http.Get(srv.URL + "/api/youtube")
	if err != nil {
		t.Fatal(err)
	}
	defer yt.Body.Close()
	var ybody map[string]any
	if err := json.NewDecoder(yt.Body).Decode(&ybody); err != nil {
		t.Fatal(err)
	}
	if ybody["enabled"] != false {
		t.Fatalf("youtube enabled: %#v", ybody)
	}

	body, ctype := multipartBody(t, "note.txt", []byte("hello api"))
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/files", body)
	req.Header.Set("Content-Type", ctype)
	up, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer up.Body.Close()
	if up.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(up.Body)
		t.Fatalf("upload %d %s", up.StatusCode, b)
	}
	var file storage.File
	if err := json.NewDecoder(up.Body).Decode(&file); err != nil {
		t.Fatal(err)
	}
	if file.OriginalName != "note.txt" || file.Size != 9 {
		t.Fatalf("%+v", file)
	}

	listRes, err := http.Get(srv.URL + "/api/files")
	if err != nil {
		t.Fatal(err)
	}
	defer listRes.Body.Close()
	var list []storage.File
	if err := json.NewDecoder(listRes.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("list %d", len(list))
	}

	bad, err := http.Get(srv.URL + "/api/files/not-a-valid-id")
	if err != nil {
		t.Fatal(err)
	}
	bad.Body.Close()
	if bad.StatusCode != http.StatusNotFound {
		t.Fatalf("traversal status %d", bad.StatusCode)
	}

	enc, err := http.Post(srv.URL+"/api/files/"+file.ID+"/encode", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Body.Close()
	if enc.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(enc.Body)
		t.Fatalf("encode %d %s", enc.StatusCode, b)
	}
}

func TestUploadTooLarge(t *testing.T) {
	srv := newTestServer(t, 8)
	defer srv.Close()
	body, ctype := multipartBody(t, "big.bin", bytes.Repeat([]byte("x"), 64))
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/files", body)
	req.Header.Set("Content-Type", ctype)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func multipartBody(t *testing.T, name string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

func TestWebDirConstant(t *testing.T) {
	if filepath.Base("web/index.html") != "index.html" {
		t.Fatal("path helper")
	}
}
