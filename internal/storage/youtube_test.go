package storage

import (
	"bytes"
	"context"
	"testing"

	"ytdrive/internal/youtube"
)

func TestYouTubeMigrationAndTokens(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.SaveUpload(bytes.NewReader([]byte("v")), "a.bin", "application/octet-stream", 100)
	if err != nil {
		t.Fatal(err)
	}
	meta := YouTubeMeta{
		VideoID:          "vid1",
		URL:              "https://www.youtube.com/watch?v=vid1",
		UploadStatus:     StatusUploaded,
		ProcessingStatus: "processing",
		UploadedAt:       NowUTC(),
		PrivacyStatus:    "private",
	}
	if err := st.SetYouTube(f.ID, meta); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetFile(f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.YouTubeVideoID != "vid1" || got.YouTubePrivacyStatus != "private" {
		t.Fatalf("%+v", got)
	}
	st.Close()

	st2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	got, err = st2.GetFile(f.ID)
	if err != nil || got.YouTubeVideoID != "vid1" {
		t.Fatalf("%+v %v", got, err)
	}

	rec := youtube.TokenRecord{AccessToken: "a", RefreshToken: "r", TokenType: "Bearer"}
	if err := st2.SaveToken(context.Background(), "google_youtube", rec); err != nil {
		t.Fatal(err)
	}
	loaded, err := st2.LoadToken(context.Background(), "google_youtube")
	if err != nil || loaded.RefreshToken != "r" {
		t.Fatalf("%+v %v", loaded, err)
	}
	if err := st2.DeleteToken(context.Background(), "google_youtube"); err != nil {
		t.Fatal(err)
	}
	_, err = st2.LoadToken(context.Background(), "google_youtube")
	if err != ErrNotFound {
		t.Fatalf("%v", err)
	}
}
