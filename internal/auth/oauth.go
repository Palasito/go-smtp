package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/Palasito/go-smtp/internal/httpclient"
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

// GetAccessToken exchanges client credentials for a Microsoft Graph API access token.
// Uses the OAuth 2.0 client credentials flow against the Microsoft identity platform.
//
// URL:  POST https://login.microsoftonline.com/{tenantID}/oauth2/v2.0/token
// Body: grant_type=client_credentials&client_id=...&client_secret=...&scope=https://graph.microsoft.com/.default
//
// Returns the access_token string or a descriptive error.
func GetAccessToken(tenantID, clientID, clientSecret string) (string, error) {
	// Check the in-memory cache first. The cache key is a SHA-256 hash of all
	// three credentials, so a wrong clientSecret always produces a different key
	// and will never match a previously cached entry for different credentials.
	key := CacheKey(tenantID, clientID, clientSecret)
	if cached, ok := GetCachedToken(key); ok {
		slog.Debug("OAuth access token served from cache", "tenant", tenantID, "client_id", clientID)
		return cached, nil
	}

	endpoint := fmt.Sprintf(tokenEndpoint, tenantID)

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"scope":         {"https://graph.microsoft.com/.default"},
	}

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
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		excerpt := string(body)
		if len(excerpt) > 500 {
			excerpt = excerpt[:500]
		}
		slog.Error("OAuth token request failed",
			"status", resp.StatusCode,
			"tenant", tenantID,
			"body", excerpt,
		)
		return "", fmt.Errorf("token request returned HTTP %d: %s", resp.StatusCode, excerpt)
	}

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
