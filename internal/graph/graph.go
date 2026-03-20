package graph

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Palasito/go-smtp/internal/httpclient"
	"github.com/Palasito/go-smtp/internal/metrics"
	"github.com/Palasito/go-smtp/internal/retry"
)

const sendMailEndpointFmt = "%s/v1.0/users/%s/sendMail"

// PermanentError wraps a Graph API failure caused by a non-retryable HTTP status
// (e.g. 400 Bad Request, 403 Forbidden). The SMTP layer uses this type to issue a
// permanent 5xx rejection rather than a temporary 4xx that invites the client to retry.
type PermanentError struct {
	err error
}

func (e *PermanentError) Error() string { return e.err.Error() }
func (e *PermanentError) Unwrap() error { return e.err }

// doAttempt builds a fresh pipe-backed request and executes one HTTP call.
// The caller is responsible for closing resp.Body on a non-error return.
func doAttempt(ctx context.Context, accessToken string, mimeBody []byte, url string) (*http.Response, error) {
	pr, pw := io.Pipe()
	go func() {
		enc := base64.NewEncoder(base64.StdEncoding, pw)
		enc.Write(mimeBody)
		enc.Close()
		pw.Close()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		pr.CloseWithError(err)
		return nil, fmt.Errorf("failed to build Graph API request: %w", err)
	}
	// Provide Content-Length so the HTTP client can send the header correctly
	// without buffering: base64 output is always ceil(n/3)*4 bytes.
	req.ContentLength = int64((len(mimeBody) + 2) / 3 * 4)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "text/plain")

	return httpclient.Client().Do(req)
}

// SendMail sends a raw MIME email via Microsoft Graph API.
//
// Endpoint: POST https://graph.microsoft.com/v1.0/users/{fromEmail}/sendMail
// Headers:  Authorization: Bearer {accessToken}
//
//	Content-Type: text/plain
//
// Body:     base64-encoded raw MIME content (std encoding, as required by the Graph API)
//
// Transient failures (429, 500, 502, 503, 504) and transport errors are
// retried up to retryAttempts total attempts using exponential back-off
// starting at retryBaseDelay.  A Retry-After response header overrides the
// computed delay for 429 responses.
func SendMail(accessToken string, mimeBody []byte, fromEmail string, retryAttempts int, retryBaseDelay time.Duration, httpTimeout time.Duration, graphEndpoint string) error {
	url := fmt.Sprintf(sendMailEndpointFmt, graphEndpoint, fromEmail)

	var lastErr error
	var permanent bool
	var totalAttempts int
	for attempt := 0; attempt < retryAttempts; attempt++ {
		totalAttempts = attempt + 1
		slog.Debug("Sending email via Microsoft Graph API", "from", fromEmail, "attempt", attempt+1)

		attemptCtx, attemptCancel := context.WithTimeout(context.Background(), httpTimeout)
		tAttempt := time.Now()
		resp, err := doAttempt(attemptCtx, accessToken, mimeBody, url)
		if err != nil {
			attemptCancel()
			// Transport-level error — always retryable.
			metrics.GraphAPILatency.WithLabelValues("transport_error").Observe(time.Since(tAttempt).Seconds())
			lastErr = fmt.Errorf("Graph API request failed: %w", err)
			slog.Warn("Graph API transport error", "attempt", attempt+1, "error", err)
			if attempt+1 < retryAttempts {
				delay := retry.Backoff(nil, attempt, retryBaseDelay, 60*time.Second)
				slog.Info("Backing off before retry", "delay", delay)
				time.Sleep(delay)
			}
			continue
		}

		if resp.StatusCode == http.StatusAccepted { // 202 — success
			metrics.GraphAPILatency.WithLabelValues("success").Observe(time.Since(tAttempt).Seconds())
			resp.Body.Close()
			attemptCancel()
			metrics.GraphAPIAttempts.Observe(float64(totalAttempts))
			slog.Info("Email sent successfully via Graph API", "from", fromEmail)
			return nil
		}

		// Read and truncate the error body before deciding what to do.
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		excerpt := string(bodyBytes)
		if len(excerpt) > 500 {
			excerpt = excerpt[:500]
		}
		lastErr = fmt.Errorf("Graph API sendMail failed: status=%d body=%s", resp.StatusCode, excerpt)

		if !retry.IsRetryable(resp.StatusCode) {
			attemptCancel()
			metrics.GraphAPILatency.WithLabelValues("permanent_error").Observe(time.Since(tAttempt).Seconds())
			slog.Error("Graph API sendMail non-retryable failure",
				"status", resp.StatusCode, "from", fromEmail, "body", excerpt)
			permanent = true
			break
		}

		metrics.GraphAPILatency.WithLabelValues("retryable_error").Observe(time.Since(tAttempt).Seconds())
		slog.Warn("Graph API transient failure",
			"status", resp.StatusCode, "from", fromEmail,
			"attempt", attempt+1, "maxAttempts", retryAttempts)
		attemptCancel()
		if attempt+1 < retryAttempts {
			delay := retry.Backoff(resp, attempt, retryBaseDelay, 60*time.Second)
			slog.Info("Backing off before retry", "delay", delay)
			time.Sleep(delay)
		}
	}

	metrics.GraphAPIAttempts.Observe(float64(totalAttempts))
	if permanent {
		slog.Error("Graph API sendMail permanently failed", "from", fromEmail, "error", lastErr)
		return &PermanentError{err: lastErr}
	}
	slog.Error("Graph API sendMail retries exhausted", "from", fromEmail, "error", lastErr)
	return lastErr
}
