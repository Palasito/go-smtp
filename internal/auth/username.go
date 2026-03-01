package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/data/aztables"
	"github.com/google/uuid"
)

// uuidRegex matches a canonical UUID string (8-4-4-4-12 hex digits).
var uuidRegex = regexp.MustCompile(
	`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`,
)

// DecodeUUIDOrBase64URL checks if input is already a UUID string (8-4-4-4-12 hex).
// If it is, the value is returned unchanged.
// Otherwise it attempts base64url decoding (no padding) to obtain 16 bytes, which are
// then formatted as a UUID string.
// Port of Python's decode_uuid_or_base64url().
func DecodeUUIDOrBase64URL(input string) (string, error) {
	if uuidRegex.MatchString(input) {
		return input, nil
	}

	// base64url decode — Python adds padding via  '=' * (-len(input_str) % 4)
	// encoding/base64.RawURLEncoding handles missing padding automatically.
	b, err := base64.RawURLEncoding.DecodeString(input)
	if err != nil {
		return "", fmt.Errorf("invalid base64url encoding in input %q: %w", input, err)
	}

	u, err := uuid.FromBytes(b)
	if err != nil {
		return "", fmt.Errorf("decoded bytes from %q cannot be converted to UUID: %w", input, err)
	}

	return u.String(), nil
}

// ParseUsername parses an SMTP authentication username into its tenantID, clientID,
// and optional fromEmail components.
//
// Expected username format: tenantID{delimiter}clientID[.optional.tld]
// Special case: if clientID is the literal string "lookup", LookupUser is called
// using the tenantID part as the lookup key.
//
// Port of Python's parse_username().
func ParseUsername(username, delimiter, tablesURL, partitionKey string) (tenantID, clientID, fromEmail string, err error) {
	// Remove optional TLD — everything after the first '.' is discarded.
	stripped := strings.SplitN(username, ".", 2)[0]

	if stripped == "" {
		return "", "", "", fmt.Errorf(
			"invalid username format: expected <tenantID>%s<clientID>", delimiter,
		)
	}

	parts := strings.Split(stripped, delimiter)
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf(
			"invalid username format: expected exactly one %q delimiter, got %d parts",
			delimiter, len(parts),
		)
	}

	tenantPart := parts[0]
	clientPart := parts[1]

	// "lookup" keyword triggers Azure Table lookup.
	if clientPart == "lookup" {
		return LookupUser(tablesURL, partitionKey, tenantPart)
	}

	// Decode both UUID or base64url parts.
	parsedTenant, err := DecodeUUIDOrBase64URL(tenantPart)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid tenant identifier: %w", err)
	}

	parsedClient, err := DecodeUUIDOrBase64URL(clientPart)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid client identifier: %w", err)
	}

	return parsedTenant, parsedClient, "", nil
}

// tableEntity is used to unmarshal an Azure Table entity JSON blob.
type tableEntity struct {
	TenantID  string `json:"tenant_id"`
	ClientID  string `json:"client_id"`
	FromEmail string `json:"from_email"`
}

// LookupUser queries Azure Table Storage for a row matching the given lookupID
// (RowKey) under partitionKey, using DefaultAzureCredential.
//
// Equivalent to Python's lookup_user().
func LookupUser(tablesURL, partitionKey, lookupID string) (tenantID, clientID, fromEmail string, err error) {
	if tablesURL == "" {
		return "", "", "", fmt.Errorf("AZURE_TABLES_URL must be set for user lookup")
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create Azure credential for table lookup: %w", err)
	}

	client, err := aztables.NewClient(tablesURL, cred, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create Azure Table client: %w", err)
	}

	filter := fmt.Sprintf("PartitionKey eq '%s' and RowKey eq '%s'", partitionKey, lookupID)
	pager := client.NewListEntitiesPager(&aztables.ListEntitiesOptions{
		Filter: &filter,
	})

	ctx := context.Background()
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to query Azure Table: %w", err)
		}

		for _, rawEntity := range page.Entities {
			var entity tableEntity
			if err := json.Unmarshal(rawEntity, &entity); err != nil {
				slog.Warn("Failed to unmarshal table entity", "error", err)
				continue
			}

			if entity.TenantID == "" || entity.ClientID == "" {
				return "", "", "", fmt.Errorf(
					"entity for RowKey %q is missing tenant_id or client_id", lookupID,
				)
			}

			slog.Info("User found in Azure Table", "lookup_id", lookupID)
			return entity.TenantID, entity.ClientID, entity.FromEmail, nil
		}
	}

	return "", "", "", fmt.Errorf("no entity found in Azure Table for RowKey %q", lookupID)
}
