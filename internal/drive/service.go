package drive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"ytdrive/internal/blobstore"
	"ytdrive/internal/storage"
)

// Service — пользовательский workflow облачного диска.
type Service struct {
	meta     *Store
	blobs    blobstore.StorageProvider
	publicURL string
	maxUpload int64
}

func NewService(meta *Store, blobs blobstore.StorageProvider, publicURL string, maxUpload int64) *Service {
	if maxUpload < 1 {
		maxUpload = 64 << 20
	}
	return &Service{meta: meta, blobs: blobs, publicURL: strings.TrimRight(publicURL, "/"), maxUpload: maxUpload}
}

func (s *Service) ProviderName() string {
	if s.blobs == nil {
		return ""
	}
	return s.blobs.Name()
}

func (s *Service) ProviderAvailable(ctx context.Context) error {
	if s.blobs == nil {
		return blobstore.ErrNotConfigured
	}
	return s.blobs.Available(ctx)
}

func (s *Service) List(folderID string) (Listing, error) {
	folders, err := s.meta.ListFolders(folderID)
	if err != nil {
		return Listing{}, err
	}
	files, err := s.meta.ListFiles(folderID, false)
	if err != nil {
		return Listing{}, err
	}
	return Listing{FolderID: folderID, Folders: folders, Files: files}, nil
}

func (s *Service) Trash() ([]File, error) {
	return s.meta.ListFiles("", true)
}

func (s *Service) CreateFolder(parentID, name string) (Folder, error) {
	return s.meta.CreateFolder(parentID, name)
}

// UploadStream пишет байты напрямую во внешнее хранилище (без постоянного файла на VPS).
func (s *Service) UploadStream(ctx context.Context, folderID, name, mime string, r io.Reader, sizeHint int64) (File, error) {
	if s.blobs == nil {
		return File{}, blobstore.ErrNotConfigured
	}
	if sizeHint > s.maxUpload {
		return File{}, storage.ErrTooLarge
	}
	name = storage.SanitizeName(name)
	file, err := s.meta.CreateFileMeta(folderID, name, mime, s.blobs.Name(), "pending", "uploading")
	if err != nil {
		return File{}, err
	}
	key := ObjectKeyFor(file.ID, name)
	if err := s.meta.SetObjectKey(file.ID, key); err != nil {
		return File{}, err
	}
	file.ObjectKey = key

	hasher := sha256.New()
	limited := io.LimitReader(r, s.maxUpload+1)
	counted := &countingReader{r: io.TeeReader(limited, hasher)}
	meta, err := s.blobs.Put(ctx, key, counted, blobstore.PutOptions{ContentType: mime, Size: sizeHint})
	if err != nil {
		_ = s.meta.FailFile(file.ID, err.Error())
		_ = s.blobs.Delete(ctx, key)
		return File{}, err
	}
	size := counted.n
	if size == 0 && meta.Size > 0 {
		size = meta.Size
	}
	if size > s.maxUpload {
		_ = s.blobs.Delete(ctx, key)
		_ = s.meta.FailFile(file.ID, "too large")
		return File{}, storage.ErrTooLarge
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	if err := s.meta.FinalizeFile(file.ID, size, sum); err != nil {
		return File{}, err
	}
	return s.meta.GetFile(file.ID)
}

func (s *Service) Download(ctx context.Context, fileID string) (io.ReadCloser, File, error) {
	f, err := s.meta.GetFile(fileID)
	if err != nil {
		return nil, File{}, err
	}
	if f.DeletedAt != "" || f.Status != "ready" {
		return nil, File{}, ErrNotFound
	}
	rc, _, err := s.blobs.Get(ctx, f.ObjectKey)
	if err != nil {
		return nil, File{}, err
	}
	return rc, f, nil
}

func (s *Service) SoftDelete(id string) error { return s.meta.SoftDelete(id) }
func (s *Service) Restore(id string) error    { return s.meta.Restore(id) }

func (s *Service) Purge(ctx context.Context, id string) error {
	f, err := s.meta.Purge(id)
	if err != nil {
		return err
	}
	if f.ObjectKey != "" && s.blobs != nil {
		_ = s.blobs.Delete(ctx, f.ObjectKey)
	}
	return nil
}

func (s *Service) CreateShare(fileID string, ttl time.Duration, maxDownloads int) (ShareLink, error) {
	f, err := s.meta.GetFile(fileID)
	if err != nil {
		return ShareLink{}, err
	}
	if f.Status != "ready" || f.DeletedAt != "" {
		return ShareLink{}, ErrInvalid
	}
	expires := ""
	if ttl > 0 {
		expires = time.Now().UTC().Add(ttl).Format(time.RFC3339)
	}
	link, err := s.meta.CreateShare(fileID, expires, maxDownloads)
	if err != nil {
		return ShareLink{}, err
	}
	link.FileName = f.Name
	link.URL = s.shareURL(link.Token)
	return link, nil
}

func (s *Service) ListShares() ([]ShareLink, error) {
	links, err := s.meta.ListShares()
	if err != nil {
		return nil, err
	}
	for i := range links {
		links[i].URL = s.shareURL(links[i].Token)
	}
	return links, nil
}

func (s *Service) RevokeShare(id string) error { return s.meta.RevokeShare(id) }

func (s *Service) OpenShare(ctx context.Context, token string) (io.ReadCloser, File, ShareLink, error) {
	link, err := s.meta.GetShareByToken(token)
	if err != nil {
		return nil, File{}, ShareLink{}, err
	}
	if link.Revoked {
		return nil, File{}, ShareLink{}, ErrDenied
	}
	if link.ExpiresAt != "" {
		exp, err := time.Parse(time.RFC3339, link.ExpiresAt)
		if err == nil && time.Now().After(exp) {
			return nil, File{}, ShareLink{}, ErrDenied
		}
	}
	if link.MaxDownloads > 0 && link.DownloadCount >= link.MaxDownloads {
		return nil, File{}, ShareLink{}, ErrDenied
	}
	f, err := s.meta.GetFile(link.FileID)
	if err != nil {
		return nil, File{}, ShareLink{}, err
	}
	if f.DeletedAt != "" || f.Status != "ready" {
		return nil, File{}, ShareLink{}, ErrNotFound
	}
	rc, _, err := s.blobs.Get(ctx, f.ObjectKey)
	if err != nil {
		return nil, File{}, ShareLink{}, err
	}
	_ = s.meta.IncShareDownload(link.ID)
	link.URL = s.shareURL(link.Token)
	return rc, f, link, nil
}

func (s *Service) Usage() (int64, error) { return s.meta.UsageBytes() }

func (s *Service) shareURL(token string) string {
	base := s.publicURL
	if base == "" {
		base = ""
	}
	return fmt.Sprintf("%s/s/%s", base, token)
}
