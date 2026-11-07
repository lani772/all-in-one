package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lani772/all-in-one/engine/internal/auth"
	"github.com/lani772/all-in-one/engine/internal/controlapi"
	"github.com/lani772/all-in-one/engine/internal/deployment"
	"github.com/lani772/all-in-one/engine/internal/objectstore"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	secret := []byte(os.Getenv("AUTH_HMAC_SECRET"))
	if len(secret) < 32 {
		log.Fatal("AUTH_HMAC_SECRET must contain at least 32 bytes")
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	db, err := deployment.OpenPostgres(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	objects, err := objectstore.NewMinIOStore(ctx, objectstore.MinIOConfig{
		Endpoint:  strings.TrimSpace(os.Getenv("S3_ENDPOINT")),
		Bucket:    strings.TrimSpace(os.Getenv("S3_BUCKET")),
		AccessKey: os.Getenv("S3_ACCESS_KEY"),
		SecretKey: os.Getenv("S3_SECRET_KEY"),
		Region:    envOr("S3_REGION", "us-east-1"),
		UseTLS:    envBool("S3_USE_TLS", false),
	})
	if err != nil {
		log.Fatal(err)
	}

	maxSourceBytes := int64(20 << 20)
	if raw := os.Getenv("MAX_SOURCE_BYTES"); raw != "" {
		parsed, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || parsed < 1 || parsed > 100<<20 {
			log.Fatal("MAX_SOURCE_BYTES must be between 1 and 104857600")
		}
		maxSourceBytes = parsed
	}
	api, err := controlapi.New(db, objects, controlapi.Config{MaxSourceBytes: maxSourceBytes, UploadTTL: 15 * time.Minute})
	if err != nil {
		log.Fatal(err)
	}
	authenticator, err := auth.New(secret, auth.SQLNonceStore{DB: db}, time.Now)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", api.Health)
	mux.HandleFunc("/readyz", api.Ready)
	mux.Handle("/api/v1/", authenticator.Middleware(api))

	addr := envOr("LISTEN_ADDR", ":8080")
	server := &http.Server{
		Addr: addr, Handler: mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go cleanupNonces(ctx, db)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtc); err != nil {
			log.Printf("HTTP shutdown: %v", err)
		}
	}()
	log.Printf("hosting control API listening on %s; build execution is disabled", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func cleanupNonces(ctx context.Context, db *sql.DB) {
	// Periodically remove expired replay nonces and mark abandoned source upload
	// sessions expired. Source-object garbage collection is a separate worker task.
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, nonceErr := db.ExecContext(cleanupCtx, `DELETE FROM auth_nonces WHERE expires_at < now()`)
			_, uploadErr := db.ExecContext(cleanupCtx, `UPDATE source_uploads SET status = 'expired' WHERE status = 'pending' AND expires_at < now()`)
			cancel()
			err := errors.Join(nonceErr, uploadErr)
			if err != nil {
				log.Printf("nonce cleanup failed: %v", err)
			}
		}
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "true" || value == "1" || value == "yes" {
		return true
	}
	if value == "false" || value == "0" || value == "no" {
		return false
	}
	return fallback
}
