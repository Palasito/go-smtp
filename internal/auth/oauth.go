package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Palasito/go-smtp/internal/httpclient"
	"github.com/Palasito/go-smtp/internal/metrics"
)

// tokenEndpoint is the Microsoft identity platform token URL template.
const tokenEndpoint = "https://login.microsoftonline.com/%s/oauth2/v2.0/token"

// tokenResponse holds the fields we care about from the token endpoint JSON response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// isRetryableTokenStatus returns true for HTTP status codes that indicate a
// transient failure worth retrying.
func isRetryableTokenStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// tokenBackoff computes the delay before the next retry. It honours the
// Retry-After header when present and falls back to exponential backoff
// capped at 30 s.
func tokenBackoff(resp *http.Response, attempt int) time.Duration {
	const maxDelay = 30 * time.Second
	base := 1 * time.Second
	if resp != nil {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
				d := time.Duration(secs) * time.Second
				if d > maxDelay {
					return maxDelay
				}
				return d
			}
		}
	}
	d := time.Duration(float64(base) * math.Pow(2, float64(attempt)))
	// Add ±20% jitter to avoid thundering-herd retry storms.
	jitter := 0.8 + rand.Float64()*0.4 // [0.8, 1.2)
	d = time.Duration(float64(d) * jitter)
	if d > maxDelay {
		return maxDelay
	}
	return d
}

// GetAccessToken exchanges client credentials for a Microsoft Graph API access token.
// Uses the OAuth 2.0 client credentials flow against the Microsoft identity platform.
//
// URL:  POST https://login.microsoftonline.com/{tenantID}/oauth2/v2.0/token
// Body: grant_type=client_credentials&client_id=...&client_secret=...&scope=https://graph.microsoft.com/.default
//
// Retries up to 3 attempts on transient errors (429/5xx) and transport failures
// with exponential backoff. Returns the access_token string or a descriptive error.
func GetAccessToken(tenantID, clientID, clientSecret string) (string, error) {
	// Check the in-memory cache first. The cache key is a SHA-256 hash of all
	// three credentials, so a wrong clientSecret always produces a different key
	// and will never match a previously cached entry for different credentials.
	key := CacheKey(tenantID, clientID, clientSecret)
	if cached, ok := GetCachedToken(key); ok {
		metrics.TokenCacheHits.Inc()
		slog.Debug("OAuth access token served from cache", "tenant", tenantID, "client_id", clientID)
		return cached, nil
	}
	metrics.TokenCacheMisses.Inc()

	endpoint := fmt.Sprintf(tokenEndpoint, tenantID)

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"scope":         {"https://graph.microsoft.com/.default"},
	}

	const maxAttempts = 3
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost, endpoint, strings.NewReader(form.Encode()),
		)
		if err != nil {
			return "", fmt.Errorf("failed to build token request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := httpclient.Client().Do(req)
		if err != nil {
			lastErr = fmt.Errorf("token request failed: %w", err)
			slog.Warn("OAuth token request transport error, retrying",
				"attempt", attempt+1, "max", maxAttempts, "error", err,
			)
			time.Sleep(tokenBackoff(nil, attempt))
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("failed to read token response body: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			var tr tokenResponse
			if err := json.Unmarshal(body, &tr); err != nil {
				return "", fmt.Errorf("failed to parse token response JSON: %w", err)
			}

			if tr.AccessToken == "" {
				slog.Error("Token response contained no access_token",
					"error", tr.Error,
					"description", tr.ErrorDesc,
				)
				return "", fmt.Errorf("no access_token in response: %s — %s", tr.Error, tr.ErrorDesc)
			}

			SetToken(key, tr.AccessToken, tr.ExpiresIn, tokenCacheMarginSecs)
			slog.Info("OAuth access token acquired and cached", "tenant", tenantID, "client_id", clientID)
			return tr.AccessToken, nil
		}

		excerpt := string(body)
		if len(excerpt) > 500 {
			excerpt = excerpt[:500]
		}

		if isRetryableTokenStatus(resp.StatusCode) {
			lastErr = fmt.Errorf("token request returned HTTP %d: %s", resp.StatusCode, excerpt)
			slog.Warn("OAuth token request returned retryable status, retrying",
				"attempt", attempt+1, "max", maxAttempts,
				"status", resp.StatusCode, "tenant", tenantID,
			)
			time.Sleep(tokenBackoff(resp, attempt))
			continue
		}

		// Non-retryable error — fail immediately.
		slog.Error("OAuth token request failed",
			"status", resp.StatusCode,
			"tenant", tenantID,
			"body", excerpt,
		)
		return "", fmt.Errorf("token request returned HTTP %d: %s", resp.StatusCode, excerpt)
	}

	return "", fmt.Errorf("token request failed after %d attempts: %w", maxAttempts, lastErr)
}
