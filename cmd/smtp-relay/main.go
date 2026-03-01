package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/JustinIven/smtp-oauth-relay/internal/config"
	"github.com/JustinIven/smtp-oauth-relay/internal/server"
	tlspkg "github.com/JustinIven/smtp-oauth-relay/internal/tls"
	"github.com/JustinIven/smtp-oauth-relay/internal/whitelist"
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

	// --- TLS ---
	tlsCfg, err := tlspkg.LoadTLSConfig(
		cfg.TLSSource,
		cfg.TLSCertFilepath,
		cfg.TLSKeyFilepath,
		cfg.TLSCipherSuite,
		cfg.AzureKeyVaultURL,
		cfg.AzureKeyVaultCertName,
	)
	if err != nil {
		slog.Error("Failed to load TLS configuration", "error", err)
		os.Exit(1)
	}
	if tlsCfg != nil {
		slog.Info("TLS configuration loaded", "source", cfg.TLSSource)
	} else {
		slog.Info("TLS disabled")
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
	s.Addr = ":8025"
	s.Domain = cfg.ServerGreeting
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
	if err := s.Close(); err != nil {
		slog.Error("Error closing SMTP server", "error", err)
	}
	slog.Info("Server stopped")
}
