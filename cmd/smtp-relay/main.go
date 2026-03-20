package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/Palasito/go-smtp/internal/auth"
	"github.com/Palasito/go-smtp/internal/config"
	"github.com/Palasito/go-smtp/internal/health"
	"github.com/Palasito/go-smtp/internal/httpclient"
	"github.com/Palasito/go-smtp/internal/server"
	tlspkg "github.com/Palasito/go-smtp/internal/tls"
	"github.com/Palasito/go-smtp/internal/version"
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

	slog.Info("Starting smtp-relay",
		"version", version.Version,
		"commit", version.Commit,
		"buildDate", version.BuildDate,
		"go", runtime.Version(),
		"pid", os.Getpid(),
		"smtpPort", cfg.SMTPPort,
		"healthPort", cfg.HealthPort,
		"tlsSource", cfg.TLSSource,
		"maxRecipients", cfg.MaxRecipients,
		"azureAuthorityHost", cfg.AzureAuthorityHost,
		"graphEndpoint", cfg.GraphEndpoint,
	)

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

	// --- Token cache GC ---
	auth.StartCacheGC(reloadCtx, 5*time.Minute)
	slog.Info("Token cache GC started", "interval", "5m")

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
	backend := &server.Backend{}
	backend.SetConfig(cfg)
	backend.SetWhitelist(wl)

	s := gosmtp.NewServer(backend)
	s.Addr = ":" + cfg.SMTPPort
	s.Domain = cfg.ServerGreeting
	s.MaxMessageBytes = cfg.MaxMessageSize
	s.MaxRecipients = cfg.MaxRecipients
	s.TLSConfig = tlsCfg
	s.AllowInsecureAuth = !cfg.RequireTLS
	s.ReadTimeout = time.Duration(cfg.SMTPReadTimeout) * time.Second
	s.WriteTimeout = time.Duration(cfg.SMTPWriteTimeout) * time.Second
	slog.Info("SMTP server configured",
		"readTimeout", cfg.SMTPReadTimeout,
		"writeTimeout", cfg.SMTPWriteTimeout,
	)

	go func() {
		slog.Info("SMTP server starting", "addr", s.Addr, "requireTLS", cfg.RequireTLS)
		if err := s.ListenAndServe(); err != nil {
			slog.Error("SMTP server stopped", "error", err)
		}
	}()

	// --- Health / readiness HTTP server ---
	healthSrv := &http.Server{
		Addr: ":" + cfg.HealthPort,
		Handler: health.NewMux(func() error {
			conn, err := net.DialTimeout("tcp", "127.0.0.1:"+cfg.SMTPPort, 2*time.Second)
			if err != nil {
				return fmt.Errorf("SMTP listener not reachable: %w", err)
			}
			conn.Close()
			return nil
		}),
	}
	go func() {
		slog.Info("Health server starting", "addr", healthSrv.Addr)
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Health server stopped", "error", err)
		}
	}()

	// Block until SIGINT/SIGTERM (shutdown) or SIGHUP (config reload).
	quit := make(chan os.Signal, 1)
	hup := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	signal.Notify(hup, syscall.SIGHUP)

loop:
	for {
		select {
		case <-quit:
			break loop

		case <-hup:
			slog.Info("SIGHUP received, reloading configuration")
			newCfg, reloadErr := config.Load()
			if reloadErr != nil {
				slog.Warn("Configuration reload failed, keeping current config", "error", reloadErr)
				continue
			}

			// Fields that cannot be changed without a full restart — warn only.
			if newCfg.SMTPPort != cfg.SMTPPort {
				slog.Warn("SIGHUP: SMTP_PORT change ignored — requires restart",
					"current", cfg.SMTPPort, "new", newCfg.SMTPPort)
			}
			if newCfg.HealthPort != cfg.HealthPort {
				slog.Warn("SIGHUP: HEALTH_PORT change ignored — requires restart",
					"current", cfg.HealthPort, "new", newCfg.HealthPort)
			}
			if newCfg.TLSSource != cfg.TLSSource {
				slog.Warn("SIGHUP: TLS_SOURCE change ignored — requires restart",
					"current", cfg.TLSSource, "new", newCfg.TLSSource)
			}

			// Log level.
			if newCfg.LogLevel != cfg.LogLevel {
				lvl = logLevelFromString(newCfg.LogLevel)
				slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout,
					&slog.HandlerOptions{Level: lvl})))
				slog.Info("Log level updated", "level", newCfg.LogLevel)
			}

			// OAuth token cache margin.
			if newCfg.TokenCacheMargin != cfg.TokenCacheMargin {
				auth.SetTokenCacheMargin(newCfg.TokenCacheMargin)
				slog.Info("Token cache margin updated", "marginSeconds", newCfg.TokenCacheMargin)
			}

			// HTTP client timeout.
			if newCfg.HTTPTimeout != cfg.HTTPTimeout {
				httpclient.Init(time.Duration(newCfg.HTTPTimeout) * time.Second)
				slog.Info("HTTP client timeout updated", "timeoutSeconds", newCfg.HTTPTimeout)
			}

			// SMTP session timeouts (effective for new connections).
			if newCfg.SMTPReadTimeout != cfg.SMTPReadTimeout {
				s.ReadTimeout = time.Duration(newCfg.SMTPReadTimeout) * time.Second
				slog.Info("SMTP ReadTimeout updated", "readTimeoutSeconds", newCfg.SMTPReadTimeout)
			}
			if newCfg.SMTPWriteTimeout != cfg.SMTPWriteTimeout {
				s.WriteTimeout = time.Duration(newCfg.SMTPWriteTimeout) * time.Second
				slog.Info("SMTP WriteTimeout updated", "writeTimeoutSeconds", newCfg.SMTPWriteTimeout)
			}

			// Reload whitelist.
			newWl, wlErr := whitelist.NewWhitelistConfig(
				newCfg.WhitelistIPs,
				newCfg.WhitelistTenantID,
				newCfg.WhitelistClientID,
				newCfg.WhitelistClientSecret,
				newCfg.WhitelistFromEmail,
			)
			if wlErr != nil {
				slog.Warn("SIGHUP: whitelist reload failed, keeping current whitelist", "error", wlErr)
			} else {
				backend.SetWhitelist(newWl)
				if newWl != nil {
					slog.Info("Whitelist reloaded", "networks", len(newWl.Networks))
				} else {
					slog.Info("Whitelist cleared (no WHITELIST_IPS configured)")
				}
			}

			// Hot-swap backend config — picked up by all subsequent sessions.
			backend.SetConfig(newCfg)
			cfg = newCfg
			slog.Info("Configuration reloaded successfully")
		}
	}

	slog.Info("Shutdown signal received, stopping server")
	reloadCancel() // stop TLS auto-reload goroutine
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(cfg.ShutdownTimeout)*time.Second,
	)
	defer cancel()
	if err := healthSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Error shutting down health server", "error", err)
	}
	if err := s.Shutdown(shutdownCtx); err != nil {
		slog.Error("Error during graceful shutdown", "error", err)
	}
	slog.Info("Server stopped")
}
