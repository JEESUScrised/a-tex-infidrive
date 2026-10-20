package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ytdrive/internal/config"
	"ytdrive/internal/ffmpeg"
	"ytdrive/internal/httpapi"
	"ytdrive/internal/jobs"
	"ytdrive/internal/storage"
	"ytdrive/internal/youtube"
)

func main() {
	cfg := config.Load()
	addr := flag.String("addr", cfg.Addr, "адрес HTTP")
	data := flag.String("data", cfg.DataDir, "каталог данных и SQLite")
	flag.Parse()
	cfg.Addr = *addr
	cfg.DataDir = *data

	if err := cfg.Encode.PrepareEncode(); err != nil {
		log.Fatalf("параметры кодирования: %v", err)
	}
	log.Printf("диагностика FFmpeg: %s", ffmpeg.Diagnose(cfg.FFmpeg, cfg.FFProbe))
	if err := ffmpeg.New(cfg.FFmpeg, cfg.FFProbe).Available(); err != nil {
		log.Printf("предупреждение: %v — задания кодирования завершатся ошибкой", err)
	}

	store, err := storage.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("хранилище: %v", err)
	}
	defer store.Close()

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
		log.Printf("YTDrive слушает %s, данные в %s", cfg.Addr, cfg.DataDir)
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
