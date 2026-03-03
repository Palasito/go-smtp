package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Palasito/go-smtp/internal/auth"
	"github.com/Palasito/go-smtp/internal/config"
	"github.com/Palasito/go-smtp/internal/httpclient"
	"github.com/Palasito/go-smtp/internal/server"
	tlspkg "github.com/Palasito/go-smtp/internal/tls"
	"github.com/Palasito/go-smtp/internal/whitelist"
	gosmtp "github.com/emersion/go-smtp"
)

// logLevelFromString maps the Config.LogLevel string to an slog.Level.
// slog levels: DEBUG=-4, INFO=0, WARN=4, ERROR=8
func logLevelFromString(level string) slog.Level {
	switch level {
	case "DEBUG":
		return slog.LevelDebug // -4
	case "INFO":
		return slog.LevelInfo // 0
	case "WARNING":
		return slog.LevelWarn // 4
	case "ERROR":
		return slog.LevelError // 8
	case "CRITICAL":
		return slog.Level(12) // above ERROR, analogous to CRITICAL
	default:
		return slog.LevelWarn
	}
}

func main() {
	// Load configuration from environment variables.
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Configure structured logger with the requested log level.
	lvl := logLevelFromString(cfg.LogLevel)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
	}))
	slog.SetDefault(logger)

	slog.Info("Configuration loaded successfully")

	// --- HTTP client ---
	httpclient.Init(time.Duration(cfg.HTTPTimeout) * time.Second)
	slog.Info("HTTP client initialised", "timeout", cfg.HTTPTimeout)

	// --- OAuth token cache ---
	auth.SetTokenCacheMargin(cfg.TokenCacheMargin)
	slog.Info("OAuth token cache initialised", "marginSeconds", cfg.TokenCacheMargin)

	// --- TLS ---
	reloadCtx, reloadCancel := context.WithCancel(context.Background())
	tlsCfg, tlsReloader, err := tlspkg.LoadTLSConfig(
		cfg.TLSSource,
		cfg.TLSCertFilepath,
		cfg.TLSKeyFilepath,
		cfg.TLSCipherSuite,
		cfg.AzureKeyVaultURL,
		cfg.AzureKeyVaultCertName,
	)
	if err != nil {
		reloadCancel()
		slog.Error("Failed to load TLS configuration", "error", err)
		os.Exit(1)
	}
	if tlsCfg != nil {
		slog.Info("TLS configuration loaded", "source", cfg.TLSSource)
	} else {
		slog.Info("TLS disabled")
	}
	if tlsReloader != nil && cfg.TLSReloadInterval > 0 {
		tlsReloader.Start(reloadCtx, time.Duration(cfg.TLSReloadInterval)*time.Second)
		slog.Info("TLS certificate auto-reload enabled", "intervalSeconds", cfg.TLSReloadInterval)
	}

	// --- Whitelist ---
	wl, err := whitelist.NewWhitelistConfig(
		cfg.WhitelistIPs,
		cfg.WhitelistTenantID,
		cfg.WhitelistClientID,
		cfg.WhitelistClientSecret,
		cfg.WhitelistFromEmail,
	)
	if err != nil {
		slog.Error("Failed to load whitelist configuration", "error", err)
		os.Exit(1)
	}
	if wl != nil {
		slog.Info("IP whitelist enabled", "networks", len(wl.Networks))
	}

	// --- SMTP server ---
	backend := &server.Backend{
		Config:    cfg,
		Whitelist: wl,
	}

	s := gosmtp.NewServer(backend)
	s.Addr = ":" + cfg.SMTPPort
	s.Domain = cfg.ServerGreeting
	s.MaxMessageBytes = cfg.MaxMessageSize
	s.TLSConfig = tlsCfg
	s.AllowInsecureAuth = !cfg.RequireTLS

	go func() {
		slog.Info("SMTP server starting", "addr", s.Addr, "requireTLS", cfg.RequireTLS)
		if err := s.ListenAndServe(); err != nil {
			slog.Error("SMTP server stopped", "error", err)
		}
	}()

	// Block until SIGINT or SIGTERM is received.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutdown signal received, stopping server")
	reloadCancel() // stop TLS auto-reload goroutine
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(cfg.ShutdownTimeout)*time.Second,
	)
	defer cancel()
	if err := s.Shutdown(shutdownCtx); err != nil {
		slog.Error("Error during graceful shutdown", "error", err)
	}
	slog.Info("Server stopped")
}
