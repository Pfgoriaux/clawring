package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Pfgoriaux/clawring/admin"
	"github.com/Pfgoriaux/clawring/config"
	"github.com/Pfgoriaux/clawring/db"
	"github.com/Pfgoriaux/clawring/middleware"
	"github.com/Pfgoriaux/clawring/proxy"
)

const usageRetentionDays = 30

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("opening database: %v", err)
	}
	defer database.Close()

	// Context cancelled on shutdown — used by rate limiter cleanup goroutines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Periodic usage log pruning
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := database.PruneUsage(usageRetentionDays); err != nil {
					log.Printf("usage prune error: %v", err)
				} else if n > 0 {
					log.Printf("pruned %d usage log entries", n)
				}
			}
		}
	}()

	vendors := proxy.DefaultVendors()

	// Admin server (port 9100)
	adminHandler := &admin.Handler{
		DB:         database,
		MasterKey:  cfg.MasterKey,
		AdminToken: cfg.AdminToken,
		Vendors:    vendors,
	}
	adminRL := middleware.NewRateLimiter(ctx, 100)
	adminRL.TrustedProxies = cfg.TrustedProxies
	adminServer := &http.Server{
		Addr:         net.JoinHostPort(cfg.BindAddr, cfg.AdminPort),
		Handler:      adminRL.Wrap(adminHandler),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Data server (port 9101)
	dataHandler := &proxy.Handler{
		DB:        database,
		MasterKey: cfg.MasterKey,
		Vendors:   vendors,
		Client: &http.Client{
			Timeout: 600 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	dataRL := middleware.NewRateLimiter(ctx, 1000)
	dataRL.TrustedProxies = cfg.TrustedProxies
	dataServer := &http.Server{
		Addr:              net.JoinHostPort(cfg.BindAddr, cfg.DataPort),
		Handler:           dataRL.Wrap(dataHandler),
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      0, // disabled for SSE; per-chunk deadlines set via ResponseController
		IdleTimeout:       120 * time.Second,
	}

	// Start servers — use error channel instead of log.Fatalf in goroutines
	errCh := make(chan error, 2)
	go func() {
		log.Printf("admin server listening on %s", adminServer.Addr)
		if err := adminServer.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- fmt.Errorf("admin server: %w", err)
		}
	}()
	go func() {
		log.Printf("data server listening on %s", dataServer.Addr)
		if err := dataServer.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- fmt.Errorf("data server: %w", err)
		}
	}()

	// Graceful shutdown on signal or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Printf("received signal %v, shutting down...", sig)
	case err := <-errCh:
		log.Printf("server error: %v, shutting down...", err)
	}

	// Cancel context to stop background goroutines (rate limiter cleanup, usage pruning)
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := adminServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("admin server shutdown error: %v", err)
	}
	if err := dataServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("data server shutdown error: %v", err)
	}
	log.Println("shutdown complete")
}
