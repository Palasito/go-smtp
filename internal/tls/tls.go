// Package tls provides TLS configuration helpers for the SMTP relay.
// crypto/tls is imported under the alias "cryptotls" to avoid a name collision
// with this package.
package tls

import (
	"context"
	cryptotls "crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"software.sslmate.com/src/go-pkcs12"
)

// openSSLToGoSuite maps a subset of common OpenSSL cipher suite names to their
// corresponding Go crypto/tls uint16 constants.
// TLS 1.3 ciphers are excluded — Go does not allow configuring them.
var openSSLToGoSuite = map[string]uint16{
	"ECDHE-ECDSA-AES128-GCM-SHA256": cryptotls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	"ECDHE-RSA-AES128-GCM-SHA256":   cryptotls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	"ECDHE-ECDSA-AES256-GCM-SHA384": cryptotls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	"ECDHE-RSA-AES256-GCM-SHA384":   cryptotls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	"ECDHE-ECDSA-CHACHA20-POLY1305": cryptotls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	"ECDHE-RSA-CHACHA20-POLY1305":   cryptotls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
	"ECDHE-RSA-AES128-CBC-SHA":      cryptotls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	"ECDHE-ECDSA-AES128-CBC-SHA":    cryptotls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	"ECDHE-RSA-AES256-CBC-SHA":      cryptotls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	"ECDHE-ECDSA-AES256-CBC-SHA":    cryptotls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	"RSA-AES128-GCM-SHA256":         cryptotls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	"AES128-GCM-SHA256":             cryptotls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	"RSA-AES256-GCM-SHA384":         cryptotls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	"AES256-GCM-SHA384":             cryptotls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	"RSA-AES128-CBC-SHA":            cryptotls.TLS_RSA_WITH_AES_128_CBC_SHA,
	"AES128-SHA":                    cryptotls.TLS_RSA_WITH_AES_128_CBC_SHA,
	"RSA-AES256-CBC-SHA":            cryptotls.TLS_RSA_WITH_AES_256_CBC_SHA,
	"AES256-SHA":                    cryptotls.TLS_RSA_WITH_AES_256_CBC_SHA,
	"RSA-AES128-CBC-SHA256":         cryptotls.TLS_RSA_WITH_AES_128_CBC_SHA256,
	"AES128-SHA256":                 cryptotls.TLS_RSA_WITH_AES_128_CBC_SHA256,
	"ECDHE-RSA-AES128-SHA256":       cryptotls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
	"ECDHE-ECDSA-AES128-SHA256":     cryptotls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
	"RSA-3DES-EDE-CBC-SHA":          cryptotls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	"DES-CBC3-SHA":                  cryptotls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	"ECDHE-RSA-DES-CBC3-SHA":        cryptotls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	"ECDHE-ECDSA-DES-CBC3-SHA":      cryptotls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA, // closest approximation
}

// parseCipherSuites parses a colon-separated list of OpenSSL cipher names and
// returns the corresponding Go uint16 constants. Unrecognised names are logged
// as warnings and skipped.
func parseCipherSuites(raw string) []uint16 {
	names := strings.Split(raw, ":")
	out := make([]uint16, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if id, ok := openSSLToGoSuite[name]; ok {
			out = append(out, id)
		} else {
			slog.Warn("Unrecognised cipher suite name — skipping", "cipher", name)
		}
	}
	return out
}

// applyCipherSuites sets CipherSuites on cfg when a non-empty cipher string is given.
func applyCipherSuites(cfg *cryptotls.Config, cipherSuite string) {
	if cipherSuite == "" {
		return
	}
	suites := parseCipherSuites(cipherSuite)
	if len(suites) > 0 {
		cfg.CipherSuites = suites
	}
}

// LoadTLSConfig creates a *tls.Config and a *CertReloader based on the TLS source.
//
//   - source="off":       returns nil, nil, nil
//   - source="file":      loads PEM cert+key from certPath and keyPath
//   - source="keyvault":  fetches PKCS#12 from Azure Key Vault, parses it, builds tls.Config
//
// The returned CertReloader's GetCertificate hook is wired into the tls.Config.
// Call CertReloader.Start(ctx, interval) to enable background hot-reload.
func LoadTLSConfig(source, certPath, keyPath, cipherSuite, kvURL, kvCertName string) (*cryptotls.Config, *CertReloader, error) {
	switch source {
	case "off":
		return nil, nil, nil

	case "file":
		return loadFromFile(certPath, keyPath, cipherSuite)

	case "keyvault":
		return loadFromKeyVault(kvURL, kvCertName, cipherSuite)

	default:
		return nil, nil, fmt.Errorf("unknown TLS source %q: must be off, file, or keyvault", source)
	}
}

// loadFromFile loads a PEM certificate and key from the local filesystem.
func loadFromFile(certPath, keyPath, cipherSuite string) (*cryptotls.Config, *CertReloader, error) {
	if _, err := os.Stat(certPath); err != nil {
		return nil, nil, fmt.Errorf("TLS cert file not found at %q: %w", certPath, err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		return nil, nil, fmt.Errorf("TLS key file not found at %q: %w", keyPath, err)
	}

	loader := func() (*cryptotls.Certificate, error) {
		cert, err := cryptotls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS key pair from files: %w", err)
		}
		return &cert, nil
	}

	reloader, err := newCertReloader(loader)
	if err != nil {
		return nil, nil, err
	}

	cfg := &cryptotls.Config{
		GetCertificate: reloader.GetCertificate,
	}
	applyCipherSuites(cfg, cipherSuite)

	slog.Info("TLS certificate loaded from file", "cert", certPath, "key", keyPath)
	return cfg, reloader, nil
}

// loadFromKeyVault fetches a PKCS#12 certificate from Azure Key Vault and
// builds a tls.Config from it.
func loadFromKeyVault(kvURL, kvCertName, cipherSuite string) (*cryptotls.Config, *CertReloader, error) {
	if kvURL == "" {
		return nil, nil, fmt.Errorf("AZURE_KEY_VAULT_URL must be set when TLS_SOURCE=keyvault")
	}
	if kvCertName == "" {
		return nil, nil, fmt.Errorf("AZURE_KEY_VAULT_CERT_NAME must be set when TLS_SOURCE=keyvault")
	}

	// Authenticate using the ambient Azure credential chain.
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Azure credential: %w", err)
	}

	client, err := azsecrets.NewClient(kvURL, cred, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Key Vault secrets client: %w", err)
	}

	// loader captures the already-initialised Key Vault client.
	// It is called once immediately and then on every reload tick.
	loader := func() (*cryptotls.Certificate, error) {
		ctx := context.Background()
		resp, err := client.GetSecret(ctx, kvCertName, "", nil)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch secret %q from Key Vault: %w", kvCertName, err)
		}
		if resp.Value == nil || *resp.Value == "" {
			return nil, fmt.Errorf("secret %q is empty in Key Vault", kvCertName)
		}

		// The secret value is the base64-encoded PKCS#12 blob.
		pfxData, err := base64.StdEncoding.DecodeString(*resp.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to base64-decode Key Vault secret %q: %w", kvCertName, err)
		}

		// DecodeChain returns the private key, end-entity cert, and any CA certs.
		privateKey, certificate, caCerts, err := pkcs12.DecodeChain(pfxData, "")
		if err != nil {
			return nil, fmt.Errorf("failed to parse PKCS#12 data from Key Vault: %w", err)
		}
		if certificate == nil {
			return nil, fmt.Errorf("no certificate found in PKCS#12 data from Key Vault")
		}
		if privateKey == nil {
			return nil, fmt.Errorf("no private key found in PKCS#12 data from Key Vault")
		}

		tlsCert := cryptotls.Certificate{PrivateKey: privateKey}
		tlsCert.Certificate = append(tlsCert.Certificate, certificate.Raw)
		for _, ca := range caCerts {
			tlsCert.Certificate = append(tlsCert.Certificate, ca.Raw)
		}
		tlsCert.Leaf = certificate // avoids re-parsing on every handshake
		return &tlsCert, nil
	}

	reloader, err := newCertReloader(loader)
	if err != nil {
		return nil, nil, err
	}

	cfg := &cryptotls.Config{
		GetCertificate: reloader.GetCertificate,
	}

	// Populate client CA pool from the chain on first load (CA list is stable
	// across rotations; only the leaf cert changes).
	if leaf := reloader.Cert(); leaf != nil && len(leaf.Certificate) > 1 {
		pool := x509.NewCertPool()
		for _, derBytes := range leaf.Certificate[1:] {
			if ca, err := x509.ParseCertificate(derBytes); err == nil {
				pool.AddCert(ca)
			}
		}
		cfg.ClientCAs = pool
	}

	applyCipherSuites(cfg, cipherSuite)

	slog.Info("TLS certificate loaded from Azure Key Vault", "vault", kvURL, "cert", kvCertName)
	return cfg, reloader, nil
}

// ---------------------------------------------------------------------------
// CertReloader
// ---------------------------------------------------------------------------

// CertReloader holds a hot-reloadable TLS certificate.
// It is wired into crypto/tls via the GetCertificate callback, so new
// connections always use the most recently loaded certificate without
// requiring a server restart.
type CertReloader struct {
	mu   sync.RWMutex
	cert *cryptotls.Certificate
	load func() (*cryptotls.Certificate, error)
}

// newCertReloader builds a CertReloader with the provided loader function and
// performs an immediate load to verify the source is reachable.
func newCertReloader(load func() (*cryptotls.Certificate, error)) (*CertReloader, error) {
	r := &CertReloader{load: load}
	if err := r.Reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// GetCertificate implements the crypto/tls GetCertificate hook.
// It is goroutine-safe and is called by the TLS stack on every new handshake.
func (r *CertReloader) GetCertificate(_ *cryptotls.ClientHelloInfo) (*cryptotls.Certificate, error) {
	r.mu.RLock()
	cert := r.cert
	r.mu.RUnlock()
	return cert, nil
}

// Cert returns the currently cached certificate. Returns nil before the first
// successful load.
func (r *CertReloader) Cert() *cryptotls.Certificate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cert
}

// Reload fetches a fresh certificate from the underlying source and atomically
// replaces the cached copy. Safe to call concurrently.
func (r *CertReloader) Reload() error {
	cert, err := r.load()
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.cert = cert
	r.mu.Unlock()
	return nil
}

// Start runs a background goroutine that reloads the certificate every
// interval until ctx is cancelled. It is a no-op when interval <= 0.
func (r *CertReloader) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := r.Reload(); err != nil {
					slog.Error("TLS certificate reload failed", "error", err)
				} else {
					slog.Info("TLS certificate reloaded successfully")
				}
			}
		}
	}()
}
