package youtube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

func newYouTubeService(ctx context.Context, ts oauth2.TokenSource) (*youtube.Service, error) {
	return youtube.NewService(ctx, option.WithTokenSource(ts))
}

func fetchChannel(ctx context.Context, svc *youtube.Service) (ChannelInfo, error) {
	call := svc.Channels.List([]string{"snippet"}).Mine(true).MaxResults(1)
	call.Context(ctx)
	resp, err := call.Do()
	if err != nil {
		return ChannelInfo{}, mapAPIError(err)
	}
	if len(resp.Items) == 0 {
		return ChannelInfo{}, fmt.Errorf("%w: канал не найден", ErrNotConnected)
	}
	ch := resp.Items[0]
	title := ""
	if ch.Snippet != nil {
		title = ch.Snippet.Title
	}
	return ChannelInfo{ID: ch.Id, Title: title}, nil
}

func fetchVideo(ctx context.Context, svc *youtube.Service, videoID string) (VideoRef, error) {
	call := svc.Videos.List([]string{"status", "processingDetails", "snippet"}).Id(videoID)
	call.Context(ctx)
	resp, err := call.Do()
	if err != nil {
		return VideoRef{}, mapAPIError(err)
	}
	if len(resp.Items) == 0 {
		return VideoRef{}, fmt.Errorf("видео %s не найдено", videoID)
	}
	return refFromVideo(resp.Items[0]), nil
}

func refFromVideo(v *youtube.Video) VideoRef {
	ref := VideoRef{
		VideoID: v.Id,
		URL:     "https://www.youtube.com/watch?v=" + v.Id,
	}
	if v.Status != nil {
		ref.UploadStatus = v.Status.UploadStatus
		ref.PrivacyStatus = v.Status.PrivacyStatus
	}
	if v.ProcessingDetails != nil {
		ref.ProcessingStatus = v.ProcessingDetails.ProcessingStatus
	}
	if v.Snippet != nil {
		ref.ChannelID = v.Snippet.ChannelId
		ref.ChannelTitle = v.Snippet.ChannelTitle
	}
	return ref
}

func uploadFile(ctx context.Context, svc *youtube.Service, path string, opt UploadOptions) (VideoRef, error) {
	privacy := strings.TrimSpace(opt.Privacy)
	if privacy == "" {
		privacy = "private"
	}
	title := strings.TrimSpace(opt.Title)
	if title == "" {
		title = "YTDrive upload"
	}
	f, err := os.Open(path)
	if err != nil {
		return VideoRef{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return VideoRef{}, err
	}
	if st.Size() <= 0 {
		return VideoRef{}, fmt.Errorf("пустой MP4 нельзя загрузить на YouTube")
	}

	video := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       title,
			Description: opt.Description,
			CategoryId:  "22",
		},
		Status: &youtube.VideoStatus{
			PrivacyStatus:           privacy,
			SelfDeclaredMadeForKids: false,
			ForceSendFields:         []string{"SelfDeclaredMadeForKids"},
		},
	}

	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return VideoRef{}, err
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return VideoRef{}, err
		}
		call := svc.Videos.Insert([]string{"snippet", "status"}, video)
		call.Context(ctx)
		call.Media(f,
			googleapi.ContentType("video/mp4"),
			googleapi.ChunkSize(8<<20),
		)
		if opt.Progress != nil {
			total := st.Size()
			call.ProgressUpdater(func(current, totalHint int64) {
				t := totalHint
				if t <= 0 {
					t = total
				}
				opt.Progress(current, t)
			})
		}
		resp, err := call.Do()
		if err == nil {
			ref := refFromVideo(resp)
			if ref.PrivacyStatus == "" {
				ref.PrivacyStatus = privacy
			}
			if ref.UploadStatus == "" {
				ref.UploadStatus = "uploaded"
			}
			return ref, nil
		}
		last = mapAPIError(err)
		// Повтор videos.insert после неопределённого ответа опасен — только явные временные сбои.
		if errors.Is(last, ErrAuth) || errors.Is(last, ErrQuota) {
			return VideoRef{}, last
		}
		if !errors.Is(last, ErrTemporary) {
			return VideoRef{}, last
		}
		select {
		case <-ctx.Done():
			return VideoRef{}, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Second):
		}
	}
	return VideoRef{}, last
}
