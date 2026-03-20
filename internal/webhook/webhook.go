// Package webhook provides best-effort HTTP notification for delivery failures.
// Notifications are fire-and-forget: failures to reach the webhook endpoint are
// logged but never surfaced to the SMTP client.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Palasito/go-smtp/internal/httpclient"
	"github.com/Palasito/go-smtp/internal/metrics"
)

// webhookSem limits the number of concurrent in-flight webhook goroutines.
var webhookSem = make(chan struct{}, 16)

// FailurePayload is the JSON body POSTed to the webhook URL on a permanent
// delivery failure (i.e. after all retry attempts have been exhausted).
type FailurePayload struct {
	From      string   `json:"from"`
	To        []string `json:"to"`
	Error     string   `json:"error"`
	Timestamp string   `json:"timestamp"` // RFC 3339
	Attempts  int      `json:"attempts"`
}

// NotifyFailureAsync dispatches NotifyFailure in a bounded goroutine pool.
// If the semaphore is full, the notification is dropped and a warning is logged.
func NotifyFailureAsync(webhookURL string, payload FailurePayload) {
	select {
	case webhookSem <- struct{}{}:
		go func() {
			defer func() { <-webhookSem }()
			NotifyFailure(webhookURL, payload)
		}()
	default:
		slog.Warn("Webhook: goroutine pool full, dropping notification", "from", payload.From)
	}
}

// NotifyFailure posts payload to webhookURL as a JSON body.
// The call should be invoked with `go` so it never blocks the SMTP session.
// A 5-second timeout is applied; any network or HTTP error is logged at WARN
// level and silently discarded.
func NotifyFailure(webhookURL string, payload FailurePayload) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body, err := json.Marshal(payload)
	if err != nil {
		metrics.WebhookTotal.WithLabelValues("failure").Inc()
		slog.Warn("Webhook: failed to marshal payload", "error", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		metrics.WebhookTotal.WithLabelValues("failure").Inc()
		slog.Warn("Webhook: failed to build request", "url", webhookURL, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpclient.Client().Do(req)
	if err != nil {
		metrics.WebhookTotal.WithLabelValues("failure").Inc()
		slog.Warn("Webhook: delivery failed", "url", webhookURL, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		metrics.WebhookTotal.WithLabelValues("failure").Inc()
		slog.Warn("Webhook: non-2xx response", "url", webhookURL, "status", resp.StatusCode)
		return
	}

	metrics.WebhookTotal.WithLabelValues("success").Inc()
	slog.Info("Webhook: failure notification delivered", "url", webhookURL,
		"from", payload.From, "status", fmt.Sprintf("%d", resp.StatusCode))
}
