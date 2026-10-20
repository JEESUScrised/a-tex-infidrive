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

	"ytdrive/internal/config"
	"ytdrive/internal/cryptoutil"
	"ytdrive/internal/ffmpeg"
	"ytdrive/internal/httpapi"
	"ytdrive/internal/jobs"
	"ytdrive/internal/storage"
	"ytdrive/internal/youtube"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("конфигурация: %v", err)
	}
	addr := flag.String("addr", cfg.Addr, "адрес HTTP")
	data := flag.String("data", cfg.StoragePath, "каталог данных")
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
		log.Printf("предупреждение: %v — задания кодирования завершатся ошибкой", err)
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
		log.Printf("OAuth-токены: шифрование at rest включено")
	}

	if cfg.AuthEnabled {
		log.Printf("режим доступа: single-admin (%s)", cfg.AdminUsername)
	} else {
		log.Printf("режим доступа: открытый API (только для development)")
	}

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
			log.Printf("YouTube Data API: OAuth настроен, redirect=%s", cfg.GoogleRedirectURL)
		}
	} else {
		log.Printf("YouTube: GOOGLE_CLIENT_ID/SECRET не заданы — загрузка отключена")
	}

	manager := jobs.New(store, cfg.Encode, cfg.Workers, cfg.QueueSize, yt)
	manager.Start()
	defer manager.Stop()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.New(store, manager, cfg, yt).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("%s слушает %s, данные в %s", cfg.AppName, cfg.Addr, cfg.StoragePath)
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
