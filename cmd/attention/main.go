package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"attention-crm/internal/app"
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "backup":
			if err := runBackup(os.Args[2:]); err != nil {
				log.Fatalf("backup: %v", err)
			}
			return
		case "restore":
			if err := runRestore(os.Args[2:]); err != nil {
				log.Fatalf("restore: %v", err)
			}
			return
		case "version":
			fmt.Printf("%s %s %s\n", app.BuildVersion, app.BuildCommit, app.BuildTime)
			return
		}
	}

	cfg := app.ConfigFromEnv()
	applyGlobalOverrides(&cfg, os.Args[1:])

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

func applyGlobalOverrides(cfg *app.Config, args []string) {
	get := func(key string) string {
		// Supports: --key value, --key=value
		for i := 0; i < len(args); i++ {
			a := strings.TrimSpace(args[i])
			if a == "" {
				continue
			}
			if strings.HasPrefix(a, "--"+key+"=") {
				return strings.TrimSpace(strings.TrimPrefix(a, "--"+key+"="))
			}
			if a == "--"+key && i+1 < len(args) {
				return strings.TrimSpace(args[i+1])
			}
		}
		return ""
	}

	if v := get("listen"); v != "" {
		cfg.ListenAddr = v
	}
	if v := get("data-dir"); v != "" {
		cfg.DataDir = v
	}
	if v := get("public-origin"); v != "" {
		cfg.PublicOrigin = v
		cfg.WebAuthnOrigins = []string{cfg.PublicOrigin}
		if cfg.WebAuthnRPID == "localhost" {
			if u, err := url.Parse(cfg.PublicOrigin); err == nil && u.Hostname() != "" {
				cfg.WebAuthnRPID = u.Hostname()
			}
		}
	}
}

func stripGlobalFlags(args []string) []string {
	out := make([]string, 0, len(args))
	skipNext := false
	for i := 0; i < len(args); i++ {
		if skipNext {
			skipNext = false
			continue
		}
		a := strings.TrimSpace(args[i])
		if a == "" {
			continue
		}
		for _, key := range []string{"listen", "data-dir", "public-origin"} {
			if a == "--"+key {
				skipNext = true
				a = ""
				break
			}
			if strings.HasPrefix(a, "--"+key+"=") {
				a = ""
				break
			}
		}
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}

func runBackup(args []string) error {
	cfg := app.ConfigFromEnv()
	applyGlobalOverrides(&cfg, args)

	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	tenantSlug := fs.String("tenant", "", "tenant slug to back up (required)")
	out := fs.String("out", "", "backup output path (default: <data-dir>/backups/<slug>-<timestamp>.db)")
	if err := fs.Parse(stripGlobalFlags(args)); err != nil {
		return err
	}

	slug := strings.TrimSpace(*tenantSlug)
	if slug == "" {
		return fmt.Errorf("missing --tenant")
	}

	store, err := control.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer store.Close()

	tenant, err := store.TenantBySlug(slug)
	if err != nil {
		return err
	}

	outPath := strings.TrimSpace(*out)
	if outPath == "" {
		ts := time.Now().UTC().Format("20060102T150405Z")
		outPath = filepath.Join(cfg.DataDir, "backups", slug+"-"+ts+".db")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.BackupTo(outPath); err != nil {
		return err
	}

	fmt.Printf("backup written: %s\n", outPath)
	return nil
}

func runRestore(args []string) error {
	cfg := app.ConfigFromEnv()
	applyGlobalOverrides(&cfg, args)

	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	tenantSlug := fs.String("tenant", "", "tenant slug to restore (required)")
	from := fs.String("from", "", "path to backup file (required)")
	if err := fs.Parse(stripGlobalFlags(args)); err != nil {
		return err
	}

	slug := strings.TrimSpace(*tenantSlug)
	if slug == "" {
		return fmt.Errorf("missing --tenant")
	}
	fromPath := strings.TrimSpace(*from)
	if fromPath == "" {
		return fmt.Errorf("missing --from")
	}

	if _, err := os.Stat(fromPath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	store, err := control.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer store.Close()

	tenant, err := store.TenantBySlug(slug)
	if err != nil {
		return err
	}

	if err := tenantdb.RestoreFromBackup(tenant.DBPath, fromPath); err != nil {
		return err
	}

	fmt.Printf("restored tenant %s from %s\n", slug, fromPath)
	return nil
}
