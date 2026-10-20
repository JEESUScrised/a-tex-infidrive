package jobs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ytdrive/internal/codec"
	"ytdrive/internal/storage"
	"ytdrive/internal/youtube"
)

func TestYouTubeUploadJobCompletes(t *testing.T) {
	st := openStore(t)
	fake := &youtube.FakeProvider{VideoID: "abc123"}
	fake.SetConnected(true)
	opt := codec.DefaultOptions()
	m := New(st, opt, 1, 2, fake)
	m.Start()
	defer m.Stop()

	f := save(t, st, "clip.txt", []byte("yt-upload"))
	video := filepath.Join(filepath.Dir(f.StoredPath), "video.mp4")
	if err := os.WriteFile(video, []byte("fake-mp4-bytes"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := st.SetVideo(f.ID, video); err != nil {
		t.Fatal(err)
	}

	job, err := m.EnqueueYouTubeUpload(f.ID)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var got storage.Job
	for time.Now().Before(deadline) {
		got, err = st.GetJob(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == storage.StatusCompleted || got.Status == storage.StatusFailed || got.Status == storage.StatusCancelled {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got.Status != storage.StatusCompleted {
		t.Fatalf("status=%s err=%s msg=%s", got.Status, got.Error, got.Message)
	}
	file, err := st.GetFile(f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if file.YouTubeVideoID != "abc123" || file.YouTubePrivacyStatus != "private" {
		t.Fatalf("%+v", file)
	}
	if fake.UploadCount() != 1 {
		t.Fatalf("uploads %d", fake.UploadCount())
	}
	_, err = m.EnqueueYouTubeUpload(f.ID)
	if err == nil {
		t.Fatal("duplicate upload should fail")
	}
}

func TestYouTubeUploadRequiresConnection(t *testing.T) {
	st := openStore(t)
	fake := &youtube.FakeProvider{}
	m := New(st, codec.DefaultOptions(), 1, 2, fake)
	f := save(t, st, "x.txt", []byte("x"))
	_ = st.SetVideo(f.ID, f.StoredPath)
	_, err := m.EnqueueYouTubeUpload(f.ID)
	if !errors.Is(err, youtube.ErrNotConnected) {
		t.Fatalf("%v", err)
	}
}

func TestFailInterruptedJobs(t *testing.T) {
	st := openStore(t)
	f := save(t, st, "i.txt", []byte("i"))
	job, err := st.CreateJob(storage.KindYouTubeUpload, f.ID, f.StoredPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateJob(job.ID, storage.StatusUploading, 40, "upload", ""); err != nil {
		t.Fatal(err)
	}
	n, err := st.FailInterruptedJobs()
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	got, err := st.GetJob(job.ID)
	if err != nil || got.Status != storage.StatusFailed {
		t.Fatalf("%+v %v", got, err)
	}
}
