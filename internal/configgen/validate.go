package configgen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Palasito/go-smtp/internal/config"
)

// Severity represents the severity of a validation finding.
type Severity int

const (
	SeverityError   Severity = iota // blocks deployment
	SeverityWarning                 // advisory
)

// Finding represents a single validation issue.
type Finding struct {
	Severity Severity
	Line     int    // 0 if not tied to a specific line
	Key      string // env var name, empty if general
	Message  string
}

func (f Finding) String() string {
	prefix := "ERROR"
	if f.Severity == SeverityWarning {
		prefix = "WARN "
	}
	if f.Line > 0 {
		return fmt.Sprintf("[%s] line %d: %s — %s", prefix, f.Line, f.Key, f.Message)
	}
	return fmt.Sprintf("[%s] %s — %s", prefix, f.Key, f.Message)
}

// ValidateEnvFile parses the .env file at path and validates all entries
// against the config schema. Returns a slice of findings (errors and warnings).
func ValidateEnvFile(path string) ([]Finding, error) {
	entries, err := ParseEnvFile(path)
	if err != nil {
		return nil, err
	}

	// Build schema lookup
	schemaMap := make(map[string]config.FieldDef)
	for _, f := range config.Schema {
		schemaMap[f.EnvVar] = f
	}

	// Build values map from parsed entries
	values := make(map[string]string)
	for _, e := range entries {
		values[e.Key] = e.Value
	}

	var findings []Finding

	// Per-entry validation
	for _, e := range entries {
		f, known := schemaMap[e.Key]
		if !known {
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Line:     e.Line,
				Key:      e.Key,
				Message:  "not a recognized configuration variable",
			})
			continue
		}

		// Type validation
		switch f.Type {
		case "int":
			if _, err := strconv.Atoi(e.Value); err != nil {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Line:     e.Line,
					Key:      e.Key,
					Message:  fmt.Sprintf("%q is not a valid integer", e.Value),
				})
				continue // skip further checks if type is wrong
			}
		case "int64":
			if _, err := strconv.ParseInt(e.Value, 10, 64); err != nil {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Line:     e.Line,
					Key:      e.Key,
					Message:  fmt.Sprintf("%q is not a valid integer", e.Value),
				})
				continue
			}
		case "bool":
			lower := strings.ToLower(e.Value)
			if lower != "true" && lower != "false" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Line:     e.Line,
					Key:      e.Key,
					Message:  fmt.Sprintf("%q must be \"true\" or \"false\"", e.Value),
				})
				continue
			}
		}
		// "string" and "string_list" always pass type check

		// Enum validation
		if len(f.ValidValues) > 0 {
			matched := false
			for _, v := range f.ValidValues {
				if strings.EqualFold(v, e.Value) {
					matched = true
					break
				}
			}
			if !matched {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Line:     e.Line,
					Key:      e.Key,
					Message:  fmt.Sprintf("%q must be one of: %s", e.Value, strings.Join(f.ValidValues, ", ")),
				})
				continue
			}
		}

		// Port range check
		if strings.Contains(f.ValidationHint, "port") && f.Type == "string" {
			if port, err := strconv.Atoi(e.Value); err == nil {
				if port < 1 || port > 65535 {
					findings = append(findings, Finding{
						Severity: SeverityError,
						Line:     e.Line,
						Key:      e.Key,
						Message:  fmt.Sprintf("%q is not a valid port (must be 1-65535)", e.Value),
					})
				}
			}
		}

		// URL format for FAILURE_WEBHOOK_URL
		if e.Key == "FAILURE_WEBHOOK_URL" && e.Value != "" {
			if !strings.HasPrefix(e.Value, "http://") && !strings.HasPrefix(e.Value, "https://") {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Line:     e.Line,
					Key:      e.Key,
					Message:  "must start with http:// or https://",
				})
			}
		}

		// Positive integer check
		if strings.Contains(f.ValidationHint, "positive") && (f.Type == "int" || f.Type == "int64") {
			if val, err := strconv.Atoi(e.Value); err == nil && val <= 0 {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Line:     e.Line,
					Key:      e.Key,
					Message:  fmt.Sprintf("%q must be a positive integer", e.Value),
				})
			}
		}

		// Non-negative integer check
		if strings.Contains(f.ValidationHint, "non-negative") && (f.Type == "int" || f.Type == "int64") {
			if val, err := strconv.Atoi(e.Value); err == nil && val < 0 {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Line:     e.Line,
					Key:      e.Key,
					Message:  fmt.Sprintf("%q must be a non-negative integer", e.Value),
				})
			}
		}
	}

	// Cross-field checks
	// Port collision
	if smtpPort, ok := values["SMTP_PORT"]; ok {
		if healthPort, ok2 := values["HEALTH_PORT"]; ok2 && smtpPort == healthPort {
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Key:      "SMTP_PORT/HEALTH_PORT",
				Message:  fmt.Sprintf("SMTP_PORT and HEALTH_PORT are both %q", smtpPort),
			})
		}
	}

	// Whitelist dependencies
	if whitelistIPs, ok := values["WHITELIST_IPS"]; ok && whitelistIPs != "" {
		for _, dep := range []string{"WHITELIST_TENANT_ID", "WHITELIST_CLIENT_ID", "WHITELIST_CLIENT_SECRET"} {
			if v, ok := values[dep]; !ok || v == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Key:      dep,
					Message:  "required when WHITELIST_IPS is set",
				})
			}
		}
	}

	// TLS file dependencies
	if tlsSource, ok := values["TLS_SOURCE"]; ok && strings.EqualFold(tlsSource, "file") {
		if _, ok := values["TLS_CERT_FILEPATH"]; !ok {
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Key:      "TLS_CERT_FILEPATH",
				Message:  "recommended when TLS_SOURCE=file",
			})
		}
		if _, ok := values["TLS_KEY_FILEPATH"]; !ok {
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Key:      "TLS_KEY_FILEPATH",
				Message:  "recommended when TLS_SOURCE=file",
			})
		}
	}

	// TLS keyvault dependencies
	if tlsSource, ok := values["TLS_SOURCE"]; ok && strings.EqualFold(tlsSource, "keyvault") {
		if _, ok := values["AZURE_KEY_VAULT_URL"]; !ok {
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Key:      "AZURE_KEY_VAULT_URL",
				Message:  "recommended when TLS_SOURCE=keyvault",
			})
		}
		if _, ok := values["AZURE_KEY_VAULT_CERT_NAME"]; !ok {
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Key:      "AZURE_KEY_VAULT_CERT_NAME",
				Message:  "recommended when TLS_SOURCE=keyvault",
			})
		}
	}

	return findings, nil
}
