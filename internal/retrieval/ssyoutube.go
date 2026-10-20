package retrieval

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SSYouTube — экспериментальный адаптер.
//
// Результат исследования (2026-10):
//   - ssyoutube.com сообщает о прекращении сервиса в США и не публикует
//     открытый документированный API (доступ «по запросу» на contact@ssyoutube.com).
//   - Сторонние обёртки (архивные GitHub/replit) неофициальны, нестабильны
//     и по сути являются обходом ограничений YouTube — их не встраиваем.
//   - Автоматический HTML-scrape / обход CAPTCHA запрещены политикой проекта.
//
// Поэтому Retrieve по умолчанию возвращает ErrUnavailable.
// Опциональный SSYOUTUBE_API_URL (явно заданный разрешённый endpoint) может
// быть использован только если администратор сознательно его настроил;
// секреты Google туда никогда не передаются.
type SSYouTube struct {
	APIBase    string // пусто = интеграция отключена
	HTTPClient *http.Client
	MaxBytes   int64
}

func NewSSYouTube(apiBase string, maxBytes int64) *SSYouTube {
	if maxBytes <= 0 {
		maxBytes = 64 << 20
	}
	return &SSYouTube{
		APIBase:  strings.TrimRight(strings.TrimSpace(apiBase), "/"),
		MaxBytes: maxBytes,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("слишком много redirects")
				}
				return validatePublicURL(req.URL)
			},
		},
	}
}

func (s *SSYouTube) Name() string { return "ssyoutube" }

func (s *SSYouTube) Available(ctx context.Context) error {
	if s.APIBase == "" {
		return fmt.Errorf("%w: SSYouTube не предоставляет открытый API; задайте SSYOUTUBE_API_URL только для явно разрешённого endpoint или используйте ручной импорт MP4", ErrUnavailable)
	}
	u, err := url.Parse(s.APIBase)
	if err != nil || u.Scheme != "https" {
		return fmt.Errorf("%w: SSYOUTUBE_API_URL должен быть https://", ErrForbidden)
	}
	if err := validatePublicURL(u); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, s.APIBase, nil)
	if err != nil {
		return err
	}
	res, err := s.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	res.Body.Close()
	if res.StatusCode >= 500 {
		return fmt.Errorf("%w: upstream %d", ErrUnavailable, res.StatusCode)
	}
	return nil
}

func (s *SSYouTube) Retrieve(ctx context.Context, videoID string) (io.ReadCloser, error) {
	if err := s.Available(ctx); err != nil {
		return nil, err
	}
	videoID = strings.TrimSpace(videoID)
	if !validYouTubeID(videoID) {
		return nil, fmt.Errorf("%w: некорректный video_id", ErrBadContent)
	}
	// Контракт опционального API: GET {base}?v={id} → video/mp4 (если провайдер так документировал).
	// Без явного разрешённого base не вызывается.
	endpoint := s.APIBase + "?v=" + url.QueryEscape(videoID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "video/mp4,application/octet-stream")
	res, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("%w: HTTP %d", ErrUnavailable, res.StatusCode)
	}
	ct := strings.ToLower(res.Header.Get("Content-Type"))
	if ct != "" && !strings.Contains(ct, "video/") && !strings.Contains(ct, "octet-stream") && !strings.Contains(ct, "mp4") {
		res.Body.Close()
		return nil, fmt.Errorf("%w: content-type %s", ErrBadContent, ct)
	}
	return &limitedReadCloser{ReadCloser: res.Body, remain: s.MaxBytes}, nil
}

func (s *SSYouTube) Diagnose(ctx context.Context) Meta {
	err := s.Available(ctx)
	msg := "SSYouTube: нет открытого API; US-сервис прекращён; авто-scrape запрещён. Используйте ручной импорт MP4 из Studio."
	if s.APIBase != "" && err == nil {
		msg = "Настроенный SSYOUTUBE_API_URL отвечает; автоматическое получение может работать."
	} else if err != nil {
		msg = err.Error()
	}
	return Meta{
		Name:           s.Name(),
		Available:      err == nil,
		Message:        msg,
		Automatic:      err == nil && s.APIBase != "",
		ManualFallback: true,
	}
}

type limitedReadCloser struct {
	io.ReadCloser
	remain int64
}

func (l *limitedReadCloser) Read(p []byte) (int, error) {
	if l.remain <= 0 {
		return 0, ErrTooLarge
	}
	if int64(len(p)) > l.remain {
		p = p[:l.remain]
	}
	n, err := l.ReadCloser.Read(p)
	l.remain -= int64(n)
	return n, err
}

func validYouTubeID(id string) bool {
	if len(id) < 6 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

func validatePublicURL(u *url.URL) error {
	if u == nil || u.Host == "" {
		return ErrForbidden
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return ErrForbidden
	}
	host := u.Hostname()
	if host == "localhost" || strings.HasSuffix(host, ".local") {
		return ErrForbidden
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		// DNS fail — запрещаем (fail closed).
		return fmt.Errorf("%w: dns", ErrForbidden)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("%w: private address", ErrForbidden)
		}
	}
	return nil
}
