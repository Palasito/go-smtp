// Package httpclient provides a shared, pre-configured http.Client for the relay.
// All outbound HTTP calls (OAuth token endpoint, Microsoft Graph API) use this
// client so that timeouts and transport settings are consistent and configurable.
package httpclient

import (
	"net/http"
	"sync"
	"time"
)

var (
	mu       sync.RWMutex
	instance *http.Client
)

// Init creates the shared http.Client with the given overall request timeout.
// It must be called once at startup before any package in this module makes
// an outbound HTTP request. Subsequent calls replace the existing client.
//
// Transport settings:
//   - TLSHandshakeTimeout:   10 s
//   - ResponseHeaderTimeout: timeout (same as overall)
//   - IdleConnTimeout:       90 s  (matches http.DefaultTransport)
//   - MaxIdleConnsPerHost:   4     (two endpoints: AAD + Graph)
func Init(timeout time.Duration) {
	transport := &http.Transport{
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConnsPerHost:   4,
		ForceAttemptHTTP2:     true,
	}

	mu.Lock()
	instance = &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
	mu.Unlock()
}

// Client returns the shared http.Client.
// Falls back to http.DefaultClient if Init has not been called (e.g. in tests).
func Client() *http.Client {
	mu.RLock()
	c := instance
	mu.RUnlock()
	if c == nil {
		return http.DefaultClient
	}
	return c
}
