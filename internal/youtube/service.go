package youtube

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/oauth2"
	ytapi "google.golang.org/api/youtube/v3"
)

// Service — рабочая реализация VideoStorageProvider.
type Service struct {
	creds  Credentials
	tokens TokenPersister
	states *StateStore
}

// NewService создаёт провайдер. При пустых credentials возвращает ошибку — вызывайте Unconfigured.
func NewService(creds Credentials, tokens TokenPersister) (*Service, error) {
	if !creds.valid() {
		return nil, ErrNotConfigured
	}
	if tokens == nil {
		return nil, fmt.Errorf("не задано хранилище токенов")
	}
	return &Service{
		creds:  creds,
		tokens: tokens,
		states: NewStateStore(0),
	}, nil
}

func (s *Service) Configured() bool { return s != nil && s.creds.valid() }

func (s *Service) Connected(ctx context.Context) bool {
	tok, err := s.tokens.LoadToken(ctx, providerGoogle)
	return err == nil && (tok.AccessToken != "" || tok.RefreshToken != "")
}

// BeginAuth выдаёт URL авторизации и CSRF-state.
func (s *Service) BeginAuth() (authURL, state string, err error) {
	if !s.Configured() {
		return "", "", ErrNotConfigured
	}
	state, err = s.states.Issue()
	if err != nil {
		return "", "", err
	}
	url := s.creds.config().AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	)
	return url, state, nil
}

func (s *Service) AuthURL(state string) (string, error) {
	if !s.Configured() {
		return "", ErrNotConfigured
	}
	if state == "" {
		return "", ErrInvalidState
	}
	return s.creds.config().AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	), nil
}

func (s *Service) HandleCallback(ctx context.Context, code, state string) error {
	if !s.Configured() {
		return ErrNotConfigured
	}
	if !s.states.Consume(state) {
		return ErrInvalidState
	}
	if code == "" {
		return fmt.Errorf("%w: пустой code", ErrAuth)
	}
	tok, err := s.creds.config().Exchange(ctx, code)
	if err != nil {
		return errors.Join(ErrAuth, err)
	}
	rec := tokenFromOAuth(tok)
	if rec.RefreshToken == "" {
		if old, err := s.tokens.LoadToken(ctx, providerGoogle); err == nil {
			rec.RefreshToken = old.RefreshToken
		}
	}
	return s.tokens.SaveToken(ctx, providerGoogle, rec)
}

func (s *Service) Disconnect(ctx context.Context) error {
	if !s.Configured() {
		return ErrNotConfigured
	}
	return s.tokens.DeleteToken(ctx, providerGoogle)
}

func (s *Service) GetConnectedChannel(ctx context.Context) (ChannelInfo, error) {
	svc, err := s.api(ctx)
	if err != nil {
		return ChannelInfo{}, err
	}
	return fetchChannel(ctx, svc)
}

func (s *Service) UploadVideo(ctx context.Context, filePath string, opt UploadOptions) (VideoRef, error) {
	svc, err := s.api(ctx)
	if err != nil {
		return VideoRef{}, err
	}
	return uploadFile(ctx, svc, filePath, opt)
}

func (s *Service) GetVideoStatus(ctx context.Context, videoID string) (VideoRef, error) {
	if videoID == "" {
		return VideoRef{}, fmt.Errorf("пустой video_id")
	}
	svc, err := s.api(ctx)
	if err != nil {
		return VideoRef{}, err
	}
	return fetchVideo(ctx, svc, videoID)
}

func (s *Service) Upload(ctx context.Context, filePath, title, description string) (VideoRef, error) {
	return s.UploadVideo(ctx, filePath, UploadOptions{Title: title, Description: description})
}

func (s *Service) ProcessingStatus(ctx context.Context, videoID string) (string, error) {
	ref, err := s.GetVideoStatus(ctx, videoID)
	if err != nil {
		return "", err
	}
	if ref.ProcessingStatus != "" {
		return ref.ProcessingStatus, nil
	}
	return ref.UploadStatus, nil
}

func (s *Service) api(ctx context.Context) (*ytapi.Service, error) {
	ts, err := s.tokenSource(ctx)
	if err != nil {
		return nil, err
	}
	svc, err := newYouTubeService(ctx, ts)
	if err != nil {
		return nil, mapAPIError(err)
	}
	return svc, nil
}

func (s *Service) tokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	rec, err := s.tokens.LoadToken(ctx, providerGoogle)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotConnected, err)
	}
	if rec.AccessToken == "" && rec.RefreshToken == "" {
		return nil, ErrNotConnected
	}
	base := s.creds.config().TokenSource(ctx, rec.toOAuth())
	return &savingTokenSource{base: s.tokens, src: base, tok: rec.toOAuth()}, nil
}
