package config

// FieldDef describes a single configuration environment variable.
type FieldDef struct {
	EnvVar                string   `json:"envVar"`
	Description           string   `json:"description"`
	Default               string   `json:"default"`
	Type                  string   `json:"type"`
	Required              bool     `json:"required"`
	ValidValues           []string `json:"validValues,omitempty"`
	Group                 string   `json:"group"`
	Sensitive             bool     `json:"sensitive"`
	ValidationHint        string   `json:"validationHint,omitempty"`
	ConditionallyRequired string   `json:"conditionallyRequired,omitempty"`
}

// Schema is the ordered list of all configuration environment variables.
var Schema = []FieldDef{
	// ── Logging ──────────────────────────────────────────────────────────
	{
		EnvVar:         "LOG_LEVEL",
		Description:    "Log verbosity level",
		Default:        "WARNING",
		Type:           "string",
		ValidValues:    []string{"DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"},
		Group:          "Logging",
		ValidationHint: "log verbosity level",
	},
	{
		EnvVar:         "LOG_FILE",
		Description:    "Path to log file (empty = stdout only)",
		Default:        "",
		Type:           "string",
		Group:          "Logging",
		ValidationHint: "path to log file (empty = stdout only)",
	},
	{
		EnvVar:         "LOG_FORMAT",
		Description:    "Log output format",
		Default:        "text",
		Type:           "string",
		ValidValues:    []string{"text", "json"},
		Group:          "Logging",
		ValidationHint: "log output format",
	},
	{
		EnvVar:         "LOG_ROTATE_HOURS",
		Description:    "Hours between log file rotations (0 = no rotation)",
		Default:        "1",
		Type:           "int",
		Group:          "Logging",
		ValidationHint: "non-negative integer, hours (0 = no rotation)",
	},
	{
		EnvVar:         "LOG_RETENTION_DAYS",
		Description:    "Days to keep rotated log files (0 = keep forever)",
		Default:        "0",
		Type:           "int",
		Group:          "Logging",
		ValidationHint: "non-negative integer, days (0 = keep forever)",
	},

	// ── TLS ──────────────────────────────────────────────────────────────
	{
		EnvVar:         "TLS_SOURCE",
		Description:    "TLS certificate source",
		Default:        "file",
		Type:           "string",
		ValidValues:    []string{"off", "auto", "file", "keyvault"},
		Group:          "TLS",
		ValidationHint: "TLS certificate source",
	},
	{
		EnvVar:         "REQUIRE_TLS",
		Description:    "Require TLS for SMTP connections",
		Default:        "true",
		Type:           "bool",
		Group:          "TLS",
		ValidationHint: "true or false",
	},
	{
		EnvVar:         "TLS_CERT_FILEPATH",
		Description:    "Path to TLS certificate PEM file",
		Default:        "certs/cert.pem",
		Type:           "string",
		Group:          "TLS",
		ValidationHint: "file path",
	},
	{
		EnvVar:         "TLS_KEY_FILEPATH",
		Description:    "Path to TLS private key PEM file",
		Default:        "certs/key.pem",
		Type:           "string",
		Group:          "TLS",
		ValidationHint: "file path",
	},
	{
		EnvVar:         "TLS_CIPHER_SUITE",
		Description:    "TLS cipher suites (empty = system defaults)",
		Default:        "",
		Type:           "string",
		Group:          "TLS",
		ValidationHint: "colon-separated OpenSSL cipher names (empty = system defaults)",
	},
	{
		EnvVar:         "TLS_RELOAD_INTERVAL",
		Description:    "Seconds between TLS certificate reload checks (0 = disabled)",
		Default:        "300",
		Type:           "int",
		Group:          "TLS",
		ValidationHint: "non-negative integer, seconds (0 = disabled)",
	},

	// ── SMTP ─────────────────────────────────────────────────────────────
	{
		EnvVar:         "SERVER_GREETING",
		Description:    "SMTP banner greeting text",
		Default:        "Microsoft Graph SMTP GO OAuth Relay",
		Type:           "string",
		Group:          "SMTP",
		ValidationHint: "greeting text",
	},
	{
		EnvVar:         "USERNAME_DELIMITER",
		Description:    "Delimiter separating tenant and client in SMTP username",
		Default:        "@",
		Type:           "string",
		ValidValues:    []string{"@", ":", "|"},
		Group:          "SMTP",
		ValidationHint: "single character delimiter",
	},
	{
		EnvVar:         "ALLOWED_FROM_DOMAINS",
		Description:    "Allowed sender domains (empty = allow all)",
		Default:        "",
		Type:           "string_list",
		Group:          "SMTP",
		ValidationHint: "comma-separated domain list (empty = all)",
	},
	{
		EnvVar:         "MAX_RECIPIENTS",
		Description:    "Maximum RCPT TO addresses per message (0 = unlimited)",
		Default:        "0",
		Type:           "int",
		Group:          "SMTP",
		ValidationHint: "non-negative integer (0 = unlimited)",
	},

	// ── Azure Key Vault ──────────────────────────────────────────────────
	{
		EnvVar:         "AZURE_KEY_VAULT_URL",
		Description:    "Azure Key Vault URL for TLS certificates",
		Default:        "",
		Type:           "string",
		Group:          "Azure Key Vault",
		ValidationHint: "URL",
	},
	{
		EnvVar:         "AZURE_KEY_VAULT_CERT_NAME",
		Description:    "Certificate or secret name in Azure Key Vault",
		Default:        "",
		Type:           "string",
		Group:          "Azure Key Vault",
		ValidationHint: "secret name",
	},

	// ── Azure Tables ─────────────────────────────────────────────────────
	{
		EnvVar:         "AZURE_TABLES_URL",
		Description:    "Azure Tables endpoint URL for user credential lookup",
		Default:        "",
		Type:           "string",
		Group:          "Azure Tables",
		ValidationHint: "URL",
	},
	{
		EnvVar:         "AZURE_TABLES_PARTITION_KEY",
		Description:    "Partition key for Azure Tables lookup",
		Default:        "user",
		Type:           "string",
		Group:          "Azure Tables",
		ValidationHint: "partition key string",
	},

	// ── Whitelist ────────────────────────────────────────────────────────
	{
		EnvVar:         "WHITELIST_IPS",
		Description:    "IP addresses/CIDRs for automatic authentication whitelist",
		Default:        "",
		Type:           "string",
		Group:          "Whitelist",
		ValidationHint: "comma-separated CIDR/IPs",
	},
	{
		EnvVar:                "WHITELIST_TENANT_ID",
		Description:           "Azure AD tenant ID for whitelisted connections",
		Default:               "",
		Type:                  "string",
		Group:                 "Whitelist",
		ValidationHint:        "Azure AD tenant ID",
		ConditionallyRequired: "Required when WHITELIST_IPS is set",
	},
	{
		EnvVar:                "WHITELIST_CLIENT_ID",
		Description:           "Azure AD client ID for whitelisted connections",
		Default:               "",
		Type:                  "string",
		Group:                 "Whitelist",
		ValidationHint:        "Azure AD client ID",
		ConditionallyRequired: "Required when WHITELIST_IPS is set",
	},
	{
		EnvVar:                "WHITELIST_CLIENT_SECRET",
		Description:           "Azure AD client secret for whitelisted connections",
		Default:               "",
		Type:                  "string",
		Group:                 "Whitelist",
		Sensitive:             true,
		ValidationHint:        "client secret",
		ConditionallyRequired: "Required when WHITELIST_IPS is set",
	},
	{
		EnvVar:         "WHITELIST_FROM_EMAIL",
		Description:    "From address override for whitelisted senders",
		Default:        "",
		Type:           "string",
		Group:          "Whitelist",
		ValidationHint: "email address",
	},

	// ── Server Tuning ────────────────────────────────────────────────────
	{
		EnvVar:         "SMTP_PORT",
		Description:    "SMTP server listening port",
		Default:        "8025",
		Type:           "string",
		Group:          "Server Tuning",
		ValidationHint: "port 1-65535",
	},
	{
		EnvVar:         "HEALTH_PORT",
		Description:    "Health/metrics HTTP server port",
		Default:        "9090",
		Type:           "string",
		Group:          "Server Tuning",
		ValidationHint: "port 1-65535, must differ from SMTP_PORT",
	},
	{
		EnvVar:         "MAX_MESSAGE_SIZE",
		Description:    "Maximum message size in bytes (default 35 MB)",
		Default:        "36700160",
		Type:           "int64",
		Group:          "Server Tuning",
		ValidationHint: "positive integer (bytes)",
	},
	{
		EnvVar:         "HTTP_TIMEOUT",
		Description:    "HTTP client timeout for Graph API calls",
		Default:        "30",
		Type:           "int",
		Group:          "Server Tuning",
		ValidationHint: "positive integer (seconds)",
	},
	{
		EnvVar:         "RETRY_ATTEMPTS",
		Description:    "Number of retry attempts for failed Graph API calls",
		Default:        "3",
		Type:           "int",
		Group:          "Server Tuning",
		ValidationHint: "integer 1-10",
	},
	{
		EnvVar:         "RETRY_BASE_DELAY",
		Description:    "Base delay between retries (with exponential backoff)",
		Default:        "1",
		Type:           "int",
		Group:          "Server Tuning",
		ValidationHint: "positive integer (seconds)",
	},
	{
		EnvVar:         "SHUTDOWN_TIMEOUT",
		Description:    "Graceful shutdown timeout",
		Default:        "30",
		Type:           "int",
		Group:          "Server Tuning",
		ValidationHint: "positive integer (seconds)",
	},
	{
		EnvVar:         "SMTP_READ_TIMEOUT",
		Description:    "SMTP session read timeout",
		Default:        "60",
		Type:           "int",
		Group:          "Server Tuning",
		ValidationHint: "positive integer (seconds)",
	},
	{
		EnvVar:         "SMTP_WRITE_TIMEOUT",
		Description:    "SMTP session write timeout",
		Default:        "60",
		Type:           "int",
		Group:          "Server Tuning",
		ValidationHint: "positive integer (seconds)",
	},
	{
		EnvVar:         "TOKEN_CACHE_MARGIN",
		Description:    "Seconds subtracted from token expiry for early refresh",
		Default:        "300",
		Type:           "int",
		Group:          "Server Tuning",
		ValidationHint: "non-negative integer (seconds)",
	},

	// ── Privacy & Notifications ──────────────────────────────────────────
	{
		EnvVar:         "SANITIZE_HEADERS",
		Description:    "Strip privacy-sensitive MIME headers before relay",
		Default:        "false",
		Type:           "bool",
		Group:          "Privacy & Notifications",
		ValidationHint: "true or false",
	},
	{
		EnvVar:         "FAILURE_WEBHOOK_URL",
		Description:    "HTTP(S) URL to POST on permanent send failure",
		Default:        "",
		Type:           "string",
		Group:          "Privacy & Notifications",
		ValidationHint: "URL starting with http:// or https://",
	},

	// ── Sovereign Cloud ──────────────────────────────────────────────────
	{
		EnvVar:         "AZURE_AUTHORITY_HOST",
		Description:    "Azure AD authority host for sovereign clouds",
		Default:        "https://login.microsoftonline.com",
		Type:           "string",
		Group:          "Sovereign Cloud",
		ValidationHint: "URL (e.g. https://login.microsoftonline.us)",
	},
	{
		EnvVar:         "GRAPH_ENDPOINT",
		Description:    "Microsoft Graph API endpoint for sovereign clouds",
		Default:        "https://graph.microsoft.com",
		Type:           "string",
		Group:          "Sovereign Cloud",
		ValidationHint: "URL (e.g. https://graph.microsoft.us)",
	},
}

// Groups returns the ordered list of group names as they appear in the Schema.
func Groups() []string {
	seen := map[string]bool{}
	var groups []string
	for _, f := range Schema {
		if !seen[f.Group] {
			seen[f.Group] = true
			groups = append(groups, f.Group)
		}
	}
	return groups
}

// FieldsByGroup returns only the FieldDef entries belonging to the given group.
func FieldsByGroup(group string) []FieldDef {
	var fields []FieldDef
	for _, f := range Schema {
		if f.Group == group {
			fields = append(fields, f)
		}
	}
	return fields
}
