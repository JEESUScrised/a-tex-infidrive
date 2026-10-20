package youtube

import (
	"context"
	"fmt"
	"os"
	"sync"
)

// FakeProvider — тестовый провайдер без сети.
type FakeProvider struct {
	mu          sync.Mutex
	connected   bool
	uploads     int
	fail        error
	lastPath    string
	lastBytes   []byte
	lastPrivacy string
	VideoID     string // если задан — фиксированный id (legacy tests)
	byID        map[string][]byte
}

func (f *FakeProvider) Configured() bool { return true }

func (f *FakeProvider) Connected(context.Context) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connected
}

func (f *FakeProvider) SetConnected(v bool) {
	f.mu.Lock()
	f.connected = v
	f.mu.Unlock()
}

func (f *FakeProvider) AuthURL(string) (string, error) { return "http://example/auth", nil }

func (f *FakeProvider) HandleCallback(context.Context, string, string) error {
	f.SetConnected(true)
	return nil
}

func (f *FakeProvider) GetConnectedChannel(context.Context) (ChannelInfo, error) {
	return ChannelInfo{ID: "ch1", Title: "Test Channel"}, nil
}

func (f *FakeProvider) Disconnect(context.Context) error {
	f.SetConnected(false)
	return nil
}

func (f *FakeProvider) UploadVideo(ctx context.Context, filePath string, opt UploadOptions) (VideoRef, error) {
	if err := ctx.Err(); err != nil {
		return VideoRef{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return VideoRef{}, f.fail
	}
	if f.byID == nil {
		f.byID = map[string][]byte{}
	}
	f.uploads++
	f.lastPath = filePath
	var body []byte
	if b, err := os.ReadFile(filePath); err == nil {
		body = append([]byte(nil), b...)
		f.lastBytes = body
	}
	id := f.VideoID
	if id == "" {
		id = fmt.Sprintf("video%d", f.uploads)
	}
	f.byID[id] = body
	if opt.Progress != nil {
		opt.Progress(50, 100)
		opt.Progress(100, 100)
	}
	priv := opt.Privacy
	if priv == "" {
		priv = "private"
	}
	f.lastPrivacy = priv
	return VideoRef{
		VideoID:          id,
		URL:              "https://www.youtube.com/watch?v=" + id,
		UploadStatus:     "uploaded",
		ProcessingStatus: "processing",
		PrivacyStatus:    priv,
	}, nil
}

func (f *FakeProvider) GetVideoStatus(_ context.Context, videoID string) (VideoRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := videoID
	if id == "" {
		id = f.VideoID
	}
	if id == "" {
		id = "video123"
	}
	priv := f.lastPrivacy
	if priv == "" {
		priv = "private"
	}
	return VideoRef{
		VideoID:          id,
		URL:              "https://www.youtube.com/watch?v=" + id,
		UploadStatus:     "processed",
		ProcessingStatus: "succeeded",
		PrivacyStatus:    priv,
	}, nil
}

// LastUploadPath — путь к MP4 последней загрузки (может быть уже удалён).
func (f *FakeProvider) LastUploadPath() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastPath
}

// LastUploadBytes — копия загруженного MP4 для тестов восстановления.
func (f *FakeProvider) LastUploadBytes() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]byte, len(f.lastBytes))
	copy(out, f.lastBytes)
	return out
}

// BytesByID возвращает MP4 по video_id.
func (f *FakeProvider) BytesByID(id string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.byID[id]
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func (f *FakeProvider) Upload(ctx context.Context, path, title, desc string) (VideoRef, error) {
	return f.UploadVideo(ctx, path, UploadOptions{Title: title, Description: desc})
}

func (f *FakeProvider) ProcessingStatus(ctx context.Context, id string) (string, error) {
	ref, err := f.GetVideoStatus(ctx, id)
	return ref.ProcessingStatus, err
}

func (f *FakeProvider) UploadCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.uploads
}
