package retry

import (
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// IsRetryable returns true for HTTP status codes that represent transient failures.
func IsRetryable(code int) bool {
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

// Backoff calculates the wait before the next retry attempt.
// If resp carries a Retry-After header (integer seconds) its value takes priority.
// Otherwise exponential back-off is used: base * 2^attempt, capped at maxDelay.
// ±20 % jitter is applied to avoid thundering-herd retry storms.
func Backoff(resp *http.Response, attempt int, base, maxDelay time.Duration) time.Duration {
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
	jitter := 0.8 + rand.Float64()*0.4 // [0.8, 1.2)
	d = time.Duration(float64(d) * jitter)
	if d > maxDelay {
		return maxDelay
	}
	return d
}
