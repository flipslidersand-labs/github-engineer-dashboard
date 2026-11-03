package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/flipslidersand/github-engineer-dashboard/backend-go/internal/cache"
	"github.com/flipslidersand/github-engineer-dashboard/backend-go/internal/handler"
)

func main() {
	port := getenv("PORT", "8080")
	cacheDB := getenv("CACHE_DB", "cache-go.db")
	ttl := getenvi("CACHE_TTL_SECONDS", 300)

	c, err := cache.New(cacheDB, ttl)
	if err != nil {
		log.Fatalf("cache init: %v", err)
	}
	defer c.Close()

	deps := &handler.Deps{
		Cache:        c,
		GithubToken:  os.Getenv("GITHUB_TOKEN"),
		GithubAPIURL: getenv("GITHUB_API_URL", "https://api.github.com"),
		// Shared across requests so GitHub API calls reuse pooled connections
		// instead of a fresh TCP/TLS handshake per request (Issue #128).
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(os.Getenv("CORS_ORIGINS"), deps.GithubToken != ""))
	handler.Register(r, deps)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
		// ReadHeaderTimeout guards against Slowloris-style connections that
		// trickle in headers to hold a socket open; Read/WriteTimeout bound
		// how long any single request may take end to end.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("github-engineer-dashboard go backend listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	stop()
	log.Println("shutting down: waiting for in-flight requests to finish")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvi(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

// corsMiddleware allows origins listed in the comma-separated `origins` string.
// Pass "*" to allow any origin. When hasServerToken is true, a GITHUB_TOKEN
// fallback is active for unauthenticated callers, so "*" is refused (Issue
// #123: that combination lets any third-party site ride on the operator's
// token) and no origins are reflected instead.
func corsMiddleware(origins string, hasServerToken bool) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range strings.Split(origins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = true
		}
	}
	wildcard := allowed["*"]
	if wildcard && hasServerToken {
		log.Println("CORS_ORIGINS=\"*\" ignored: GITHUB_TOKEN fallback is set, refusing to reflect arbitrary origins")
		wildcard = false
		delete(allowed, "*")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (wildcard || allowed[origin]) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "X-GitHub-Token, Content-Type")
				w.Header().Set("Vary", "Origin")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
