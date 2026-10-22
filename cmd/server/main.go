package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ytdrive/internal/blobstore"
	"ytdrive/internal/config"
	"ytdrive/internal/contentcrypto"
	"ytdrive/internal/cryptoutil"
	"ytdrive/internal/drive"
	"ytdrive/internal/ffmpeg"
	"ytdrive/internal/httpapi"
	"ytdrive/internal/jobs"
	"ytdrive/internal/pipeline"
	"ytdrive/internal/retrieval"
	"ytdrive/internal/storage"
	"ytdrive/internal/youtube"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("конфигурация: %v", err)
	}
	addr := flag.String("addr", cfg.Addr, "адрес HTTP")
	data := flag.String("data", cfg.StoragePath, "каталог метаданных SQLite")
	flag.Parse()
	cfg.Addr = *addr
	cfg.StoragePath = *data
	cfg.DataDir = *data
	if os.Getenv("DATABASE_PATH") == "" {
		cfg.DatabasePath = filepath.Join(cfg.StoragePath, "ytdrive.db")
	}
	if os.Getenv("TEMP_PATH") == "" {
		cfg.TempPath = filepath.Join(cfg.StoragePath, "tmp")
	}

	if err := cfg.Encode.PrepareEncode(); err != nil {
		log.Fatalf("параметры кодирования: %v", err)
	}
	log.Printf("%s: диагностика FFmpeg: %s", cfg.AppName, ffmpeg.Diagnose(cfg.FFmpeg, cfg.FFProbe))
	if err := ffmpeg.New(cfg.FFmpeg, cfg.FFProbe).Available(); err != nil {
		log.Printf("предупреждение: %v", err)
	}

	store, err := storage.OpenPaths(storage.Paths{
		Root:     cfg.StoragePath,
		Database: cfg.DatabasePath,
		Temp:     cfg.TempPath,
	})
	if err != nil {
		log.Fatalf("хранилище: %v", err)
	}
	defer store.Close()

	if len(cfg.TokenEncryptionKey) > 0 {
		box, err := cryptoutil.NewBox(cfg.TokenEncryptionKey)
		if err != nil {
			log.Fatalf("шифрование токенов: %v", err)
		}
		store.SetTokenBox(box)
	}

	var blobs blobstore.StorageProvider
	if !cfg.ExperimentalYouTubeStore {
		blobs, err = blobstore.NewFromConfig(blobstore.Config{
			Provider:          cfg.BlobProvider,
			S3Endpoint:        cfg.S3Endpoint,
			S3Region:          cfg.S3Region,
			S3Bucket:          cfg.S3Bucket,
			S3AccessKeyID:     cfg.S3AccessKeyID,
			S3SecretAccessKey: cfg.S3SecretAccessKey,
			S3ForcePathStyle:  cfg.S3ForcePathStyle,
			S3Prefix:          cfg.S3Prefix,
		})
		if err != nil {
			log.Fatalf("blobstore: %v", err)
		}
		log.Printf("STORAGE_PROVIDER=%s", cfg.BlobProvider)
	} else {
		log.Printf("EXPERIMENTAL_YOUTUBE_STORE: пользовательские байты только через YouTube+temp; privacy=%s", cfg.YouTubePrivacy)
	}

	driveMeta, err := drive.Open(store.DB())
	if err != nil {
		log.Fatalf("drive meta: %v", err)
	}
	driveSvc := drive.NewService(driveMeta, blobs, cfg.PublicURL, cfg.MaxUpload)

	var yt youtube.VideoStorageProvider = youtube.Unconfigured{}
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		svc, err := youtube.NewService(youtube.Credentials{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  cfg.GoogleRedirectURL,
		}, store)
		if err != nil {
			log.Printf("YouTube: %v", err)
		} else {
			yt = svc
			log.Printf("YouTube OAuth настроен, redirect=%s", cfg.GoogleRedirectURL)
		}
	}

	var pipe *pipeline.Service
	if cfg.ExperimentalYouTubeStore {
		var cbox *contentcrypto.Box
		if len(cfg.ContentEncryptionKey) == 32 {
			cbox, err = contentcrypto.NewBox(cfg.ContentEncryptionKey)
			if err != nil {
				log.Fatalf("content crypto: %v", err)
			}
		} else if cfg.AppEnv == config.EnvProduction {
			log.Fatal("CONTENT_ENCRYPTION_KEY обязателен")
		} else {
			// development: ephemeral key
			k := make([]byte, 32)
			copy(k, []byte("dev-content-key-not-for-prod!!!!"))
			cbox, _ = contentcrypto.NewBox(k)
			log.Printf("CONTENT_ENCRYPTION_KEY не задан — используется ephemeral dev-ключ")
		}
		var ret retrieval.VideoRetriever
		tempDir := cfg.BrowserRetrieverTemp
		if tempDir == "" {
			tempDir = cfg.TempPath
		}
		studioJS := retrieval.ResolveStudioWorkerJS(cfg.StudioRetrieverJS)
		if studioJS != "" && cfg.StudioBrowserProfile != "" {
			st := retrieval.NewStudio(studioJS, cfg.StudioBrowserProfile, tempDir, time.Duration(cfg.BrowserRetrieverTimeoutSec)*time.Second, cfg.MaxTempBytes)
			st.Headless = cfg.StudioRetrieverHeadless
			if err := st.Available(context.Background()); err != nil {
				log.Printf("studio retriever недоступен (%v) — пробуем SSYouTube Playwright", err)
			} else {
				ret = st
				log.Printf("retrieval: YouTube Studio worker=%s profile=%s temp=%s", studioJS, cfg.StudioBrowserProfile, tempDir)
			}
		}
		if ret == nil {
			workerJS := retrieval.ResolveWorkerJS(cfg.BrowserRetrieverJS)
			if workerJS != "" {
				pw := retrieval.NewPlaywright(workerJS, tempDir, time.Duration(cfg.BrowserRetrieverTimeoutSec)*time.Second, 64<<20)
				pw.Headless = cfg.BrowserRetrieverHeadless
				if err := pw.Available(context.Background()); err != nil {
					log.Printf("playwright retriever недоступен (%v) — fallback SSYouTube/manual", err)
					ret = retrieval.NewSSYouTube(cfg.SSYouTubeAPIURL, 64<<20)
				} else {
					ret = pw
					log.Printf("retrieval: playwright SSYouTube worker=%s temp=%s", workerJS, tempDir)
				}
			} else {
				ret = retrieval.NewSSYouTube(cfg.SSYouTubeAPIURL, 64<<20)
			}
		}
		if d, ok := ret.(interface{ Diagnose(context.Context) retrieval.Meta }); ok {
			log.Printf("retrieval: %s", d.Diagnose(context.Background()).Message)
		}
		pipe = pipeline.New(driveSvc, driveMeta, yt, ret, cbox, pipeline.Config{
			MaxBytes:       cfg.ExperimentalMaxBytes,
			MaxChunkBytes:  cfg.MaxChunkBytes,
			MaxTempBytes:   cfg.MaxTempBytes,
			Privacy:        cfg.YouTubePrivacy,
			TempDir:        tempDir,
			Encode:         cfg.Encode,
			WaitProcessing: cfg.YouTubeWaitProcessing,
		})
	}

	manager := jobs.NewWithPrivacy(store, cfg.Encode, cfg.Workers, cfg.QueueSize, yt, cfg.YouTubePrivacy)
	manager.Start()
	defer manager.Stop()

	if cfg.AuthEnabled {
		log.Printf("режим доступа: single-admin (%s)", cfg.AdminUsername)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.NewWithDrive(store, manager, cfg, yt, driveSvc, pipe).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("%s слушает %s, метаданные в %s", cfg.AppName, cfg.Addr, cfg.StoragePath)
		errCh <- srv.ListenAndServe()
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	case <-sig:
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
	fmt.Println("сервер остановлен")
}
