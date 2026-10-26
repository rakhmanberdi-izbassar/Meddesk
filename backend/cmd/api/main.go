package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/meddesk/meddesk/backend/internal/analytics"
	"github.com/meddesk/meddesk/backend/internal/audit"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/config"
	"github.com/meddesk/meddesk/backend/internal/departments"
	"github.com/meddesk/meddesk/backend/internal/knowledge"
	"github.com/meddesk/meddesk/backend/internal/media"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
	"github.com/meddesk/meddesk/backend/internal/platform/health"
	"github.com/meddesk/meddesk/backend/internal/platform/postgres"
	"github.com/meddesk/meddesk/backend/internal/settings"
	"github.com/meddesk/meddesk/backend/internal/tickets"
	"github.com/meddesk/meddesk/backend/internal/users"
)

var version = "0.5.1"

func main() {
	check := flag.Bool("healthcheck", false, "check local API readiness and exit")
	flag.Parse()
	os.Exit(run(*check))
}

func run(healthcheck bool) int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration invalid", "error", err.Error())
		return 1
	}
	if healthcheck {
		return checkReadiness(cfg.HTTPAddress)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := postgres.NewWithRootCA(ctx, cfg.DatabaseURL, cfg.RequestTimeout, cfg.DatabaseCACert)
	if err != nil {
		// The postgres constructor returns only scrubbed, static errors; never log raw pgx errors or configuration.
		logger.Error("database pool initialization failed", "error", err.Error())
		return 1
	}
	defer pool.Close()
	if err := os.MkdirAll(cfg.MediaDirectory, 0o750); err != nil {
		logger.Error("media directory is not writable")
		return 1
	}
	files, err := media.NewFileStore(cfg.MediaDirectory)
	if err != nil {
		logger.Error("media storage initialization failed")
		return 1
	}
	defer files.Close()
	readiness := health.NewService(pool, db.New(pool))
	handler := health.NewHandler(readiness, version)
	authOptions := auth.Options{AllowedOrigins: cfg.AllowedOrigins, CookieSecure: cfg.CookieSecure, SessionTTL: cfg.SessionTTL, TrustedProxyCIDRs: cfg.TrustedProxyCIDRs}
	authService := auth.NewService(auth.NewRepository(pool), cfg.SessionTTL)
	guard := auth.NewGuard(authService, authOptions)
	authHandler := auth.NewHandler(authService, guard, authOptions)
	usersHandler := users.NewHandler(users.NewService(users.NewRepository(pool)), guard)
	departmentsHandler := departments.NewHandler(departments.NewService(departments.NewRepository(pool)), guard)
	knowledgeService := knowledge.NewService(knowledge.NewRepository(pool))
	knowledgeHandler := knowledge.NewHandler(knowledgeService, guard)
	ticketsHandler := tickets.NewHandler(tickets.NewService(tickets.NewRepository(pool), knowledgeService), guard)
	settingsHandler := settings.NewHandler(settings.NewService(settings.NewRepository(pool)), guard)
	analyticsHandler := analytics.NewHandler(analytics.NewService(analytics.NewRepository(pool)), guard)
	auditHandler := audit.NewHandler(audit.NewService(audit.NewRepository(pool)), guard)
	mediaHandler := media.NewHandler(media.NewService(media.NewRepository(pool), files, media.Options{MaxImageBytes: 10 << 20, MaxVideoBytes: cfg.MediaMaxVideoBytes}), guard)
	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           handler.RouterWithMedia(logger, cfg.RequestTimeout, cfg.MediaUploadTimeout, cfg.MediaStreamTimeout, authHandler.RegisterRoutes, usersHandler.RegisterRoutes, departmentsHandler.RegisterRoutes, knowledgeHandler.RegisterRoutes, mediaHandler.RegisterRoutes, ticketsHandler.RegisterRoutes, settingsHandler.RegisterRoutes, analyticsHandler.RegisterRoutes, auditHandler.RegisterRoutes),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.RequestTimeout,
		WriteTimeout:      cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	logger.Info("API started", "address", cfg.HTTPAddress, "environment", cfg.Environment, "version", version)
	select {
	case err := <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server stopped unexpectedly")
			return 1
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP graceful shutdown timed out")
			_ = server.Close()
			return 1
		}
	}
	logger.Info("API stopped")
	return 0
}

func checkReadiness(address string) int {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return 1
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Get(fmt.Sprintf("http://%s/api/v1/ready", net.JoinHostPort(host, port)))
	if err != nil {
		return 1
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
