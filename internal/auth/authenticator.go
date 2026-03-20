package auth

import (
	"fmt"
	"log/slog"
)

// AuthResult holds the result of authenticating an SMTP session.
type AuthResult struct {
	AccessToken string
	FromEmail   string // from Azure Table lookup or whitelist, may be empty
}

// Authenticate performs the OAuth client credentials flow using the parsed username.
// username is parsed via ParseUsername to extract tenantID+clientID.
// password is used as the client_secret for the OAuth flow.
// Returns AuthResult with the access token, or error with SMTP-appropriate message.
func Authenticate(username, password, delimiter, tablesURL, partitionKey, authorityHost, graphEndpoint string) (*AuthResult, error) {
	tenantID, clientID, fromEmail, err := ParseUsername(username, delimiter, tablesURL, partitionKey)
	if err != nil {
		slog.Error("Failed to parse SMTP username", "username", username, "error", err)
		return nil, fmt.Errorf("535 5.7.8 %s", err)
	}

	token, err := GetAccessToken(tenantID, clientID, password, authorityHost, graphEndpoint)
	if err != nil {
		slog.Error("OAuth token acquisition failed", "tenantID", tenantID, "clientID", clientID, "error", err)
		return nil, fmt.Errorf("535 5.7.8 Authentication failed")
	}

	slog.Info("SMTP authentication successful", "tenantID", tenantID, "clientID", clientID, "fromEmail", fromEmail)
	return &AuthResult{
		AccessToken: token,
		FromEmail:   fromEmail,
	}, nil
}
