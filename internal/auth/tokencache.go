package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Palasito/go-smtp/internal/metrics"
)

// tokenEntry holds a cached access token and its effective expiry time.
type tokenEntry struct {
	accessToken string
	expiresAt   time.Time
}

var (
	tokenCacheMu    sync.RWMutex
	tokenCacheStore = make(map[string]tokenEntry)

	// tokenCacheMargin is subtracted from the token's expires_in before
	// caching, so the token is refreshed before it actually expires at Azure AD.
	// Configured via SetTokenCacheMargin — default 300 (5 minutes).
	tokenCacheMargin atomic.Int32
)

func init() {
	tokenCacheMargin.Store(300)
}

// SetTokenCacheMargin sets the number of seconds to subtract from a token's
// expires_in when computing the effective cache TTL.
func SetTokenCacheMargin(seconds int) {
	tokenCacheMargin.Store(int32(seconds))
}

// getTokenCacheMargin returns the current margin in seconds.
func getTokenCacheMargin() int {
	return int(tokenCacheMargin.Load())
}

// CacheKey returns a SHA-256 hex digest of the concatenated tenantID, clientID,
// and clientSecret. Using a hash means:
//   - The raw secret is never stored as a map key in memory.
//   - A client supplying an incorrect secret always produces a different key,
//     causing a cache miss and a live Azure AD validation attempt.
func CacheKey(tenantID, clientID, clientSecret string) string {
	h := sha256.Sum256([]byte(tenantID + "\x00" + clientID + "\x00" + clientSecret))
	return hex.EncodeToString(h[:])
}

// GetCachedToken returns the cached access token for key if it exists and has
// not yet reached its effective expiry. Stale entries are evicted on access.
func GetCachedToken(key string) (string, bool) {
	tokenCacheMu.RLock()
	entry, ok := tokenCacheStore[key]
	tokenCacheMu.RUnlock()

	if !ok {
		return "", false
	}
	if time.Now().Before(entry.expiresAt) {
		return entry.accessToken, true
	}

	// Stale entry — evict under write lock.
	tokenCacheMu.Lock()
	delete(tokenCacheStore, key)
	tokenCacheMu.Unlock()
	return "", false
}

// SetToken stores an access token in the cache.
//
//   - expiresIn is the token lifetime in seconds as returned by Azure AD
//     (typically 3599 for a 1-hour token).
//   - marginSeconds is subtracted from expiresIn before computing expiresAt
//     so that the token is refreshed before it actually expires at Azure AD.
//     If the result is ≤ 0, a safety floor of 60 seconds is used instead.
func SetToken(key, accessToken string, expiresIn, marginSeconds int) {
	effective := expiresIn - marginSeconds
	if effective <= 0 {
		effective = 60 // safety floor
	}
	tokenCacheMu.Lock()
	tokenCacheStore[key] = tokenEntry{
		accessToken: accessToken,
		expiresAt:   time.Now().Add(time.Duration(effective) * time.Second),
	}
	metrics.TokenCacheSize.Set(float64(len(tokenCacheStore)))
	tokenCacheMu.Unlock()
}

// StartCacheGC runs a background goroutine that periodically removes expired
// entries from the token cache. It stops when ctx is cancelled.
func StartCacheGC(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				evicted := 0
				tokenCacheMu.Lock()
				for k, e := range tokenCacheStore {
					if now.After(e.expiresAt) {
						delete(tokenCacheStore, k)
						evicted++
					}
				}
				metrics.TokenCacheSize.Set(float64(len(tokenCacheStore)))
				tokenCacheMu.Unlock()
				if evicted > 0 {
					slog.Debug("Token cache GC completed", "evicted", evicted)
				}
			}
		}
	}()
}
