package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"attention-crm/internal/app"
)

func main() {
	cfg := app.ConfigFromEnv()

	listen := flag.String("listen", "", "listen address (host:port), overrides ATTENTION_LISTEN_ADDR/PORT")
	dataDir := flag.String("data-dir", "", "data directory for control DB/session key/tenant DBs, overrides ATTENTION_DATA_DIR")
	publicOrigin := flag.String("public-origin", "", "public origin (e.g. https://crm.example.com); used for WebAuthn defaults, overrides ATTENTION_PUBLIC_ORIGIN")
	flag.Parse()

	if strings.TrimSpace(*listen) != "" {
		cfg.ListenAddr = strings.TrimSpace(*listen)
	}
	if strings.TrimSpace(*dataDir) != "" {
		cfg.DataDir = strings.TrimSpace(*dataDir)
	}
	if strings.TrimSpace(*publicOrigin) != "" {
		cfg.PublicOrigin = strings.TrimSpace(*publicOrigin)
		// Public origin is the most common single-value WebAuthn deployment config.
		// If explicitly set by flag, prefer it over any implicit defaults.
		cfg.WebAuthnOrigins = []string{cfg.PublicOrigin}
		// If RPID is still a localhost default, derive from the origin host.
		if cfg.WebAuthnRPID == "localhost" {
			if u, err := url.Parse(cfg.PublicOrigin); err == nil && u.Hostname() != "" {
				cfg.WebAuthnRPID = u.Hostname()
			}
		}
	}

	server, err := app.NewServer(cfg)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	httpServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      server.Router(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 20 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("attention-crm listening on %s", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
