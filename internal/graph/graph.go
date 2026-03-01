package graph

import (
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
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

	encoded := base64.StdEncoding.EncodeToString(mimeBody)

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("failed to build Graph API request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "text/plain")

	slog.Debug("Sending email via Microsoft Graph API", "from", fromEmail)

	resp, err := http.DefaultClient.Do(req)
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
