package graph

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/Palasito/go-smtp/internal/httpclient"
)

const sendMailEndpoint = "https://graph.microsoft.com/v1.0/users/%s/sendMail"

// SendMail sends a raw MIME email via Microsoft Graph API.
//
// Endpoint: POST https://graph.microsoft.com/v1.0/users/{fromEmail}/sendMail
// Headers:  Authorization: Bearer {accessToken}
//
//	Content-Type: text/plain
//
// Body:     base64-encoded raw MIME content (std encoding, as required by the Graph API)
//
// Returns nil on HTTP 202 Accepted; a descriptive error on any other status.
// Port of Python's send_email().
func SendMail(accessToken string, mimeBody []byte, fromEmail string) error {
	url := fmt.Sprintf(sendMailEndpoint, fromEmail)

	// Stream base64 encoding directly into the request body via a pipe,
	// avoiding an in-memory copy of the full encoded string.
	pr, pw := io.Pipe()
	go func() {
		enc := base64.NewEncoder(base64.StdEncoding, pw)
		enc.Write(mimeBody)
		enc.Close()
		pw.Close()
	}()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, pr)
	if err != nil {
		pr.CloseWithError(err)
		return fmt.Errorf("failed to build Graph API request: %w", err)
	}
	// Provide Content-Length so the HTTP client can send the header correctly
	// without buffering: base64 output is always (n+2)/3*4 bytes.
	req.ContentLength = int64((len(mimeBody) + 2) / 3 * 4)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "text/plain")

	slog.Debug("Sending email via Microsoft Graph API", "from", fromEmail)

	resp, err := httpclient.Client().Do(req)
	if err != nil {
		return fmt.Errorf("Graph API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted { // 202
		slog.Info("Email sent successfully via Graph API", "from", fromEmail)
		return nil
	}

	// Read and truncate error body for logging / error message.
	bodyBytes, _ := io.ReadAll(resp.Body)
	excerpt := string(bodyBytes)
	if len(excerpt) > 500 {
		excerpt = excerpt[:500]
	}

	slog.Error("Graph API sendMail failed",
		"status", resp.StatusCode,
		"from", fromEmail,
		"body", excerpt,
	)
	return fmt.Errorf("Graph API sendMail failed: status=%d body=%s", resp.StatusCode, excerpt)
}
