package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Logging
	LogLevel string // LOG_LEVEL — default "WARNING", valid: DEBUG/INFO/WARNING/ERROR/CRITICAL
	LogFile  string // LOG_FILE — optional, path to log file (logs are always written to stdout as well)

	// TLS
	TLSSource       string // TLS_SOURCE — default "file", valid: off/auto/file/keyvault
	RequireTLS      bool   // REQUIRE_TLS — default true
	TLSCertFilepath string // TLS_CERT_FILEPATH — default "certs/cert.pem"
	TLSKeyFilepath  string // TLS_KEY_FILEPATH — default "certs/key.pem"
	TLSCipherSuite  string // TLS_CIPHER_SUITE — optional, empty = system defaults

	// SMTP
	ServerGreeting     string   // SERVER_GREETING — default "Microsoft Graph SMTP OAuth Relay"
	UsernameDelimiter  string   // USERNAME_DELIMITER — default "@", valid: "@", ":", "|"
	AllowedFromDomains []string // ALLOWED_FROM_DOMAINS — optional, comma-separated list of allowed sender domains
	MaxRecipients      int      // MAX_RECIPIENTS — default 0 (unlimited), maximum RCPT TO addresses per message

	// Azure Key Vault
	AzureKeyVaultURL      string // AZURE_KEY_VAULT_URL — optional
	AzureKeyVaultCertName string // AZURE_KEY_VAULT_CERT_NAME — optional

	// Azure Tables
	AzureTablesURL     string // AZURE_TABLES_URL — optional
	AzureTablesPartKey string // AZURE_TABLES_PARTITION_KEY — default "user"

	// Whitelist
	WhitelistIPs          string // WHITELIST_IPS — optional, raw comma-separated string
	WhitelistTenantID     string // WHITELIST_TENANT_ID — optional
	WhitelistClientID     string // WHITELIST_CLIENT_ID — optional
	WhitelistClientSecret string // WHITELIST_CLIENT_SECRET — optional
	WhitelistFromEmail    string // WHITELIST_FROM_EMAIL — optional

	// Server tuning
	SMTPPort          string // SMTP_PORT — default "8025", valid port 1–65535
	HealthPort        string // HEALTH_PORT — default "9090", valid port 1–65535
	MaxMessageSize    int64  // MAX_MESSAGE_SIZE — default 36700160 (35 MB), must be >0
	HTTPTimeout       int    // HTTP_TIMEOUT — default 30 (seconds), must be >0
	RetryAttempts     int    // RETRY_ATTEMPTS — default 3, valid 1–10
	RetryBaseDelay    int    // RETRY_BASE_DELAY — default 1 (seconds), must be >0
	ShutdownTimeout   int    // SHUTDOWN_TIMEOUT — default 30 (seconds), must be >0
	SMTPReadTimeout   int    // SMTP_READ_TIMEOUT — default 60 (seconds), must be >0
	SMTPWriteTimeout  int    // SMTP_WRITE_TIMEOUT — default 60 (seconds), must be >0
	TLSReloadInterval int    // TLS_RELOAD_INTERVAL — default 300 (seconds), 0 = disabled
	TokenCacheMargin  int    // TOKEN_CACHE_MARGIN — default 300 (seconds), subtracted from token expires_in; must be >=0
	SanitizeHeaders   bool   // SANITIZE_HEADERS — default false, strip privacy-sensitive headers before relaying
	FailureWebhookURL string // FAILURE_WEBHOOK_URL — optional, HTTP(S) endpoint to POST on permanent send failure

	// Sovereign / national cloud overrides
	AzureAuthorityHost string // AZURE_AUTHORITY_HOST — default "https://login.microsoftonline.com", e.g. "https://login.microsoftonline.us"
	GraphEndpoint      string // GRAPH_ENDPOINT — default "https://graph.microsoft.com", e.g. "https://graph.microsoft.us"
}

var (
	validLogLevels  = []string{"DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"}
	validTLSSources = []string{"off", "auto", "file", "keyvault"}
	validDelimiters = []string{"@", ":", "|"}
)

// getEnvOrDefault returns the value of the environment variable named by key,
// or defaultVal if the variable is not set or empty.
func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// contains checks whether target is present in the slice (exact match).
func contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// Load reads all configuration from environment variables, applies defaults, and
// validates values. Returns a populated *Config or a descriptive error.
func Load() (*Config, error) {
	cfg := &Config{}

	// --- LOG_LEVEL ---
	logLevel := strings.ToUpper(getEnvOrDefault("LOG_LEVEL", "WARNING"))
	if !contains(validLogLevels, logLevel) {
		return nil, fmt.Errorf(
			"invalid LOG_LEVEL %q: must be one of %s",
			logLevel, strings.Join(validLogLevels, ", "),
		)
	}
	cfg.LogLevel = logLevel

	// --- LOG_FILE ---
	cfg.LogFile = os.Getenv("LOG_FILE") // optional, empty = stdout only

	// --- TLS_SOURCE ---
	tlsSource := strings.ToLower(getEnvOrDefault("TLS_SOURCE", "file"))
	if !contains(validTLSSources, tlsSource) {
		return nil, fmt.Errorf(
			"invalid TLS_SOURCE %q: must be one of %s",
			tlsSource, strings.Join(validTLSSources, ", "),
		)
	}
	cfg.TLSSource = tlsSource

	// --- REQUIRE_TLS ---
	requireTLSStr := strings.ToLower(getEnvOrDefault("REQUIRE_TLS", "true"))
	switch requireTLSStr {
	case "true":
		cfg.RequireTLS = true
	case "false":
		cfg.RequireTLS = false
	default:
		return nil, fmt.Errorf(
			"invalid REQUIRE_TLS %q: must be \"true\" or \"false\"",
			requireTLSStr,
		)
	}

	// Auto-correct contradictory TLS settings: requiring TLS with source=off is
	// impossible, so silently promote to "auto" with a warning.
	if cfg.RequireTLS && cfg.TLSSource == "off" {
		slog.Warn("REQUIRE_TLS=true but TLS_SOURCE=off: overriding TLS_SOURCE to \"auto\"")
		cfg.TLSSource = "auto"
	}

	// --- TLS cert/key paths ---
	cfg.TLSCertFilepath = getEnvOrDefault("TLS_CERT_FILEPATH", "certs/cert.pem")
	cfg.TLSKeyFilepath = getEnvOrDefault("TLS_KEY_FILEPATH", "certs/key.pem")
	cfg.TLSCipherSuite = os.Getenv("TLS_CIPHER_SUITE") // optional, empty = system defaults

	// --- SERVER_GREETING ---
	cfg.ServerGreeting = getEnvOrDefault("SERVER_GREETING", "Microsoft Graph SMTP GO OAuth Relay")

	// --- USERNAME_DELIMITER ---
	delimiter := getEnvOrDefault("USERNAME_DELIMITER", "@")
	if !contains(validDelimiters, delimiter) {
		return nil, fmt.Errorf(
			"invalid USERNAME_DELIMITER %q: must be one of %s",
			delimiter, strings.Join(validDelimiters, ", "),
		)
	}
	cfg.UsernameDelimiter = delimiter

	// --- ALLOWED_FROM_DOMAINS ---
	if rawDomains := os.Getenv("ALLOWED_FROM_DOMAINS"); rawDomains != "" {
		for _, d := range strings.Split(rawDomains, ",") {
			d = strings.TrimSpace(strings.ToLower(d))
			if d != "" {
				cfg.AllowedFromDomains = append(cfg.AllowedFromDomains, d)
			}
		}
	}

	// --- MAX_RECIPIENTS ---
	maxRecipientsStr := getEnvOrDefault("MAX_RECIPIENTS", "0")
	maxRecipients, err := strconv.Atoi(maxRecipientsStr)
	if err != nil || maxRecipients < 0 {
		return nil, fmt.Errorf(
			"invalid MAX_RECIPIENTS %q: must be a non-negative integer (0 = unlimited)",
			maxRecipientsStr,
		)
	}
	cfg.MaxRecipients = maxRecipients

	// --- Azure Key Vault ---
	cfg.AzureKeyVaultURL = os.Getenv("AZURE_KEY_VAULT_URL")
	cfg.AzureKeyVaultCertName = os.Getenv("AZURE_KEY_VAULT_CERT_NAME")

	// --- Azure Tables ---
	cfg.AzureTablesURL = os.Getenv("AZURE_TABLES_URL")
	cfg.AzureTablesPartKey = getEnvOrDefault("AZURE_TABLES_PARTITION_KEY", "user")

	// --- Whitelist ---
	cfg.WhitelistIPs = os.Getenv("WHITELIST_IPS")
	cfg.WhitelistTenantID = os.Getenv("WHITELIST_TENANT_ID")
	cfg.WhitelistClientID = os.Getenv("WHITELIST_CLIENT_ID")
	cfg.WhitelistClientSecret = os.Getenv("WHITELIST_CLIENT_SECRET")
	cfg.WhitelistFromEmail = os.Getenv("WHITELIST_FROM_EMAIL")

	// --- SMTP_PORT ---
	smtpPort := getEnvOrDefault("SMTP_PORT", "8025")
	smtpPortNum, err := strconv.Atoi(smtpPort)
	if err != nil || smtpPortNum < 1 || smtpPortNum > 65535 {
		return nil, fmt.Errorf(
			"invalid SMTP_PORT %q: must be an integer between 1 and 65535",
			smtpPort,
		)
	}
	cfg.SMTPPort = smtpPort

	// --- HEALTH_PORT ---
	healthPort := getEnvOrDefault("HEALTH_PORT", "9090")
	healthPortNum, err := strconv.Atoi(healthPort)
	if err != nil || healthPortNum < 1 || healthPortNum > 65535 {
		return nil, fmt.Errorf(
			"invalid HEALTH_PORT %q: must be an integer between 1 and 65535",
			healthPort,
		)
	}
	cfg.HealthPort = healthPort

	// Reject port collision — both listeners cannot bind the same port.
	if cfg.SMTPPort == cfg.HealthPort {
		return nil, fmt.Errorf(
			"SMTP_PORT and HEALTH_PORT cannot be the same (%s)", cfg.SMTPPort,
		)
	}

	// --- MAX_MESSAGE_SIZE ---
	maxMsgStr := getEnvOrDefault("MAX_MESSAGE_SIZE", "36700160")
	maxMsg, err := strconv.ParseInt(maxMsgStr, 10, 64)
	if err != nil || maxMsg <= 0 {
		return nil, fmt.Errorf(
			"invalid MAX_MESSAGE_SIZE %q: must be a positive integer (bytes)",
			maxMsgStr,
		)
	}
	cfg.MaxMessageSize = maxMsg

	// --- HTTP_TIMEOUT ---
	httpTimeoutStr := getEnvOrDefault("HTTP_TIMEOUT", "30")
	httpTimeout, err := strconv.Atoi(httpTimeoutStr)
	if err != nil || httpTimeout <= 0 {
		return nil, fmt.Errorf(
			"invalid HTTP_TIMEOUT %q: must be a positive integer (seconds)",
			httpTimeoutStr,
		)
	}
	cfg.HTTPTimeout = httpTimeout

	// --- RETRY_ATTEMPTS ---
	retryAttemptsStr := getEnvOrDefault("RETRY_ATTEMPTS", "3")
	retryAttempts, err := strconv.Atoi(retryAttemptsStr)
	if err != nil || retryAttempts < 1 || retryAttempts > 10 {
		return nil, fmt.Errorf(
			"invalid RETRY_ATTEMPTS %q: must be an integer between 1 and 10",
			retryAttemptsStr,
		)
	}
	cfg.RetryAttempts = retryAttempts

	// --- RETRY_BASE_DELAY ---
	retryDelayStr := getEnvOrDefault("RETRY_BASE_DELAY", "1")
	retryDelay, err := strconv.Atoi(retryDelayStr)
	if err != nil || retryDelay <= 0 {
		return nil, fmt.Errorf(
			"invalid RETRY_BASE_DELAY %q: must be a positive integer (seconds)",
			retryDelayStr,
		)
	}
	cfg.RetryBaseDelay = retryDelay

	// --- SHUTDOWN_TIMEOUT ---
	shutdownStr := getEnvOrDefault("SHUTDOWN_TIMEOUT", "30")
	shutdownTimeout, err := strconv.Atoi(shutdownStr)
	if err != nil || shutdownTimeout <= 0 {
		return nil, fmt.Errorf(
			"invalid SHUTDOWN_TIMEOUT %q: must be a positive integer (seconds)",
			shutdownStr,
		)
	}
	cfg.ShutdownTimeout = shutdownTimeout

	// --- SMTP_READ_TIMEOUT ---
	smtpReadStr := getEnvOrDefault("SMTP_READ_TIMEOUT", "60")
	smtpRead, err := strconv.Atoi(smtpReadStr)
	if err != nil || smtpRead <= 0 {
		return nil, fmt.Errorf(
			"invalid SMTP_READ_TIMEOUT %q: must be a positive integer (seconds)",
			smtpReadStr,
		)
	}
	cfg.SMTPReadTimeout = smtpRead

	// --- SMTP_WRITE_TIMEOUT ---
	smtpWriteStr := getEnvOrDefault("SMTP_WRITE_TIMEOUT", "60")
	smtpWrite, err := strconv.Atoi(smtpWriteStr)
	if err != nil || smtpWrite <= 0 {
		return nil, fmt.Errorf(
			"invalid SMTP_WRITE_TIMEOUT %q: must be a positive integer (seconds)",
			smtpWriteStr,
		)
	}
	cfg.SMTPWriteTimeout = smtpWrite

	// --- TLS_RELOAD_INTERVAL ---
	tlsReloadStr := getEnvOrDefault("TLS_RELOAD_INTERVAL", "300")
	tlsReload, err := strconv.Atoi(tlsReloadStr)
	if err != nil || tlsReload < 0 {
		return nil, fmt.Errorf(
			"invalid TLS_RELOAD_INTERVAL %q: must be a non-negative integer (seconds, 0 = disabled)",
			tlsReloadStr,
		)
	}
	cfg.TLSReloadInterval = tlsReload

	// --- TOKEN_CACHE_MARGIN ---
	tokenCacheMarginStr := getEnvOrDefault("TOKEN_CACHE_MARGIN", "300")
	tokenCacheMargin, err := strconv.Atoi(tokenCacheMarginStr)
	if err != nil || tokenCacheMargin < 0 {
		return nil, fmt.Errorf(
			"invalid TOKEN_CACHE_MARGIN %q: must be a non-negative integer (seconds)",
			tokenCacheMarginStr,
		)
	}
	cfg.TokenCacheMargin = tokenCacheMargin

	// --- SANITIZE_HEADERS ---
	sanitizeStr := strings.ToLower(getEnvOrDefault("SANITIZE_HEADERS", "false"))
	switch sanitizeStr {
	case "true":
		cfg.SanitizeHeaders = true
	case "false":
		cfg.SanitizeHeaders = false
	default:
		return nil, fmt.Errorf(
			"invalid SANITIZE_HEADERS %q: must be \"true\" or \"false\"",
			sanitizeStr,
		)
	}

	// --- FAILURE_WEBHOOK_URL ---
	failureWebhook := os.Getenv("FAILURE_WEBHOOK_URL")
	if failureWebhook != "" &&
		!strings.HasPrefix(failureWebhook, "http://") &&
		!strings.HasPrefix(failureWebhook, "https://") {
		return nil, fmt.Errorf(
			"invalid FAILURE_WEBHOOK_URL %q: must start with http:// or https://",
			failureWebhook,
		)
	}
	cfg.FailureWebhookURL = failureWebhook

	// --- AZURE_AUTHORITY_HOST ---
	cfg.AzureAuthorityHost = getEnvOrDefault("AZURE_AUTHORITY_HOST", "https://login.microsoftonline.com")
	cfg.AzureAuthorityHost = strings.TrimRight(cfg.AzureAuthorityHost, "/")

	// --- GRAPH_ENDPOINT ---
	cfg.GraphEndpoint = getEnvOrDefault("GRAPH_ENDPOINT", "https://graph.microsoft.com")
	cfg.GraphEndpoint = strings.TrimRight(cfg.GraphEndpoint, "/")

	// If WHITELIST_IPS is set, the OAuth credentials must also be provided.
	if cfg.WhitelistIPs != "" {
		missing := []string{}
		if cfg.WhitelistTenantID == "" {
			missing = append(missing, "WHITELIST_TENANT_ID")
		}
		if cfg.WhitelistClientID == "" {
			missing = append(missing, "WHITELIST_CLIENT_ID")
		}
		if cfg.WhitelistClientSecret == "" {
			missing = append(missing, "WHITELIST_CLIENT_SECRET")
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf(
				"WHITELIST_IPS is set but the following required variables are missing: %s",
				strings.Join(missing, ", "),
			)
		}
	}

	return cfg, nil
}
