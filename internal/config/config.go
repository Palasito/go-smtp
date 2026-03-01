package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Logging
	LogLevel string // LOG_LEVEL — default "WARNING", valid: DEBUG/INFO/WARNING/ERROR/CRITICAL

	// TLS
	TLSSource       string // TLS_SOURCE — default "file", valid: off/file/keyvault
	RequireTLS      bool   // REQUIRE_TLS — default true
	TLSCertFilepath string // TLS_CERT_FILEPATH — default "certs/cert.pem"
	TLSKeyFilepath  string // TLS_KEY_FILEPATH — default "certs/key.pem"
	TLSCipherSuite  string // TLS_CIPHER_SUITE — optional, empty = system defaults

	// SMTP
	ServerGreeting    string // SERVER_GREETING — default "Microsoft Graph SMTP OAuth Relay"
	UsernameDelimiter string // USERNAME_DELIMITER — default "@", valid: "@", ":", "|"

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
}

var (
	validLogLevels  = []string{"DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"}
	validTLSSources = []string{"off", "file", "keyvault"}
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

	// --- TLS cert/key paths ---
	cfg.TLSCertFilepath = getEnvOrDefault("TLS_CERT_FILEPATH", "certs/cert.pem")
	cfg.TLSKeyFilepath = getEnvOrDefault("TLS_KEY_FILEPATH", "certs/key.pem")
	cfg.TLSCipherSuite = os.Getenv("TLS_CIPHER_SUITE") // optional, empty = system defaults

	// --- SERVER_GREETING ---
	cfg.ServerGreeting = getEnvOrDefault("SERVER_GREETING", "Microsoft Graph SMTP OAuth Relay")

	// --- USERNAME_DELIMITER ---
	delimiter := getEnvOrDefault("USERNAME_DELIMITER", "@")
	if !contains(validDelimiters, delimiter) {
		return nil, fmt.Errorf(
			"invalid USERNAME_DELIMITER %q: must be one of %s",
			delimiter, strings.Join(validDelimiters, ", "),
		)
	}
	cfg.UsernameDelimiter = delimiter

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
