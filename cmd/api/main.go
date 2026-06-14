package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thinhnguyen/snapgram/internal/auth"
	"github.com/thinhnguyen/snapgram/internal/config"
	"github.com/thinhnguyen/snapgram/internal/httpapi"
	"github.com/thinhnguyen/snapgram/internal/media"
	"github.com/thinhnguyen/snapgram/internal/posts"
	"github.com/thinhnguyen/snapgram/internal/store"
	"github.com/thinhnguyen/snapgram/internal/worker"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)

	repo := store.NewMemoryRepository()
	tokens := auth.NewTokenService(cfg.JWTSecret, 24*time.Hour)
	postService := posts.NewService(repo)
	mediaService := media.NewService(repo, cfg.CDNBaseURL)
	workerService := worker.NewInlineProcessor(repo, mediaService)

	router := httpapi.NewRouter(httpapi.Dependencies{
		Config:       cfg,
		Logger:       logger,
		Auth:         auth.NewService(repo, tokens),
		Posts:        postService,
		Media:        mediaService,
		Worker:       workerService,
		TokenService: tokens,
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Printf("api listening addr=%s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Printf("api server failed error=%v", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Printf("api shutdown failed error=%v", err)
		os.Exit(1)
	}
}
