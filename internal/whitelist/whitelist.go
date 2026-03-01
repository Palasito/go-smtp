package whitelist

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
)

// WhitelistConfig holds the parsed whitelist configuration.
// If Networks is nil/empty, whitelisting is disabled.
type WhitelistConfig struct {
	Networks     []*net.IPNet
	TenantID     string
	ClientID     string
	ClientSecret string
	FromEmail    string // optional, may be empty
}

// ParseNetworks parses a comma-separated string of IPs and CIDRs into a slice of
// *net.IPNet values.
//
// Single IPs (no "/") are treated as host routes: /32 for IPv4, /128 for IPv6.
// Invalid entries are logged as warnings and skipped.
// Returns nil if raw is empty.
//
// Port of Python's parse_whitelist_networks().
func ParseNetworks(raw string) []*net.IPNet {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	var nets []*net.IPNet
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		cidr := entry
		if !strings.Contains(entry, "/") {
			// Plain IP — determine family and append host mask.
			ip := net.ParseIP(entry)
			if ip == nil {
				slog.Warn("Invalid IP/CIDR in WHITELIST_IPS — skipping", "entry", entry)
				continue
			}
			if ip.To4() != nil {
				cidr = entry + "/32"
			} else {
				cidr = entry + "/128"
			}
		}

		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			slog.Warn("Invalid IP/CIDR in WHITELIST_IPS — skipping", "entry", entry, "error", err)
			continue
		}
		nets = append(nets, ipNet)
	}
	return nets
}

// IsWhitelisted reports whether the given IP string is contained in any of the
// configured networks.
// Returns false if wc is nil, Networks is empty, or ip cannot be parsed.
func (wc *WhitelistConfig) IsWhitelisted(ip string) bool {
	if wc == nil || len(wc.Networks) == 0 {
		return false
	}
	addr := net.ParseIP(ip)
	if addr == nil {
		return false
	}
	for _, n := range wc.Networks {
		if n.Contains(addr) {
			return true
		}
	}
	return false
}

// NewWhitelistConfig builds a WhitelistConfig from raw configuration values.
//
// Returns nil, nil when whitelistIPs is empty (whitelisting disabled).
// Returns an error when IPs are provided but any of tenantID / clientID /
// clientSecret are missing (mirrors the Python startup-time validation).
//
// Port of Python's WHITELIST_CONFIG construction.
func NewWhitelistConfig(whitelistIPs, tenantID, clientID, clientSecret, fromEmail string) (*WhitelistConfig, error) {
	if strings.TrimSpace(whitelistIPs) == "" {
		return nil, nil
	}

	// Credentials are mandatory when IP whitelisting is enabled.
	missing := []string{}
	if tenantID == "" {
		missing = append(missing, "WHITELIST_TENANT_ID")
	}
	if clientID == "" {
		missing = append(missing, "WHITELIST_CLIENT_ID")
	}
	if clientSecret == "" {
		missing = append(missing, "WHITELIST_CLIENT_SECRET")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf(
			"WHITELIST_IPS is configured but the following required variables are missing: %s",
			strings.Join(missing, ", "),
		)
	}

	networks := ParseNetworks(whitelistIPs)
	slog.Info("IP whitelist configured", "network_count", len(networks))

	return &WhitelistConfig{
		Networks:     networks,
		TenantID:     tenantID,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		FromEmail:    fromEmail,
	}, nil
}
