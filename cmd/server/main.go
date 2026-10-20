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
	"ytdrive/internal/httpapi"
	"ytdrive/internal/jobs"
	"ytdrive/internal/storage"
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
	store, err := storage.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("хранилище: %v", err)
	}
	defer store.Close()

	manager := jobs.New(store, cfg.Encode, cfg.Workers, cfg.QueueSize)
	manager.Start()
	defer manager.Stop()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.New(store, manager, cfg).Handler(),
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
