package drive

import "time"

// File — метаданные объекта в облаке (байты во внешнем StorageProvider).
type File struct {
	ID           string `json:"id"`
	FolderID     string `json:"folder_id"`
	Name         string `json:"name"`
	MIME         string `json:"mime"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
	Provider     string `json:"provider"`
	ObjectKey    string `json:"-"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	DeletedAt    string `json:"deleted_at,omitempty"`
	Status       string `json:"status"` // uploading | ready | error
}

// Folder — папка в дереве пользователя.
type Folder struct {
	ID        string `json:"id"`
	ParentID  string `json:"parent_id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// ShareLink — публичная ссылка со случайным токеном.
type ShareLink struct {
	ID             string `json:"id"`
	FileID         string `json:"file_id"`
	Token          string `json:"token"`
	URL            string `json:"url,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
	MaxDownloads   int    `json:"max_downloads"` // 0 = без лимита
	DownloadCount  int    `json:"download_count"`
	Revoked        bool   `json:"revoked"`
	CreatedAt      string `json:"created_at"`
	FileName       string `json:"file_name,omitempty"`
	FileSize       int64  `json:"file_size,omitempty"`
}

// PublicShareView — публичные сведения по ссылке без video_id и внутренних ключей.
type PublicShareView struct {
	FileName           string `json:"file_name"`
	FileSize           int64  `json:"file_size"`
	ExpiresAt          string `json:"expires_at,omitempty"`
	DownloadAvailable  bool   `json:"download_available"`
	RetrievalHint      string `json:"retrieval_hint,omitempty"`
}

// Listing — содержимое папки.
type Listing struct {
	FolderID string   `json:"folder_id"`
	Folders  []Folder `json:"folders"`
	Files    []File   `json:"files"`
}

// UploadSession — незавершённая загрузка (метаданные до put во внешнее хранилище).
type UploadSession struct {
	ID          string
	FileID      string
	FolderID    string
	Name        string
	MIME        string
	ObjectKey   string
	MaxBytes    int64
	CreatedAt   time.Time
	ExpectedSHA string
}
