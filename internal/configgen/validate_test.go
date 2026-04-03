package configgen

import (
	"os"
	"path/filepath"
	"testing"
)

func writeValidateEnv(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func countBySeverity(findings []Finding, sev Severity) int {
	n := 0
	for _, f := range findings {
		if f.Severity == sev {
			n++
		}
	}
	return n
}

func TestValidateEnvFile(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantErrors   int
		wantWarnings int
	}{
		{
			name:         "valid minimal config",
			content:      "LOG_LEVEL=INFO\n",
			wantErrors:   0,
			wantWarnings: 0,
		},
		{
			name:         "unknown variable",
			content:      "FOOBAR=xyz\n",
			wantErrors:   0,
			wantWarnings: 1,
		},
		{
			name:         "invalid enum",
			content:      "LOG_LEVEL=TRACE\n",
			wantErrors:   1,
			wantWarnings: 0,
		},
		{
			name:         "invalid integer",
			content:      "RETRY_ATTEMPTS=abc\n",
			wantErrors:   1,
			wantWarnings: 0,
		},
		{
			name:         "invalid bool",
			content:      "REQUIRE_TLS=yes\n",
			wantErrors:   1,
			wantWarnings: 0,
		},
		{
			name:         "port out of range",
			content:      "SMTP_PORT=99999\n",
			wantErrors:   1,
			wantWarnings: 0,
		},
		{
			name:         "port collision",
			content:      "SMTP_PORT=8025\nHEALTH_PORT=8025\n",
			wantErrors:   0,
			wantWarnings: 1,
		},
		{
			name:    "whitelist missing deps",
			content: "WHITELIST_IPS=10.0.0.0/8\n",
			// WHITELIST_TENANT_ID, WHITELIST_CLIENT_ID, WHITELIST_CLIENT_SECRET all missing
			wantErrors:   3,
			wantWarnings: 0,
		},
		{
			name:         "invalid webhook URL",
			content:      "FAILURE_WEBHOOK_URL=ftp://bad\n",
			wantErrors:   1,
			wantWarnings: 0,
		},
		{
			name: "all valid complete config",
			content: "LOG_LEVEL=INFO\n" +
				"LOG_FORMAT=json\n" +
				"LOG_ROTATE_HOURS=1\n" +
				"LOG_RETENTION_DAYS=7\n" +
				"TLS_SOURCE=file\n" +
				"REQUIRE_TLS=true\n" +
				"TLS_CERT_FILEPATH=certs/cert.pem\n" +
				"TLS_KEY_FILEPATH=certs/key.pem\n" +
				"SMTP_PORT=8025\n" +
				"HEALTH_PORT=9090\n" +
				"USERNAME_DELIMITER=@\n" +
				"MAX_MESSAGE_SIZE=36700160\n" +
				"HTTP_TIMEOUT=30\n" +
				"RETRY_ATTEMPTS=3\n" +
				"RETRY_BASE_DELAY=1\n" +
				"SHUTDOWN_TIMEOUT=30\n" +
				"SMTP_READ_TIMEOUT=60\n" +
				"SMTP_WRITE_TIMEOUT=60\n" +
				"TOKEN_CACHE_MARGIN=300\n" +
				"SANITIZE_HEADERS=false\n",
			wantErrors:   0,
			wantWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeValidateEnv(t, tt.content)
			findings, err := ValidateEnvFile(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			gotErrors := countBySeverity(findings, SeverityError)
			gotWarnings := countBySeverity(findings, SeverityWarning)

			if gotErrors != tt.wantErrors {
				t.Errorf("errors: got %d, want %d", gotErrors, tt.wantErrors)
				for _, f := range findings {
					if f.Severity == SeverityError {
						t.Logf("  ERROR: %s", f)
					}
				}
			}
			if gotWarnings != tt.wantWarnings {
				t.Errorf("warnings: got %d, want %d", gotWarnings, tt.wantWarnings)
				for _, f := range findings {
					if f.Severity == SeverityWarning {
						t.Logf("  WARN: %s", f)
					}
				}
			}
		})
	}
}
