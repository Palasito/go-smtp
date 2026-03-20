package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Palasito/go-smtp/internal/auth"
	"github.com/Palasito/go-smtp/internal/config"
	"github.com/Palasito/go-smtp/internal/graph"
	"github.com/Palasito/go-smtp/internal/metrics"
	"github.com/Palasito/go-smtp/internal/webhook"
	"github.com/Palasito/go-smtp/internal/whitelist"
	"github.com/emersion/go-sasl"
	smtp "github.com/emersion/go-smtp"
	"github.com/google/uuid"
)

// Backend implements smtp.Backend.
type Backend struct {
	config    atomic.Pointer[config.Config]
	whitelist atomic.Pointer[whitelist.WhitelistConfig]
}

// SetConfig atomically replaces the current configuration.
func (b *Backend) SetConfig(cfg *config.Config) { b.config.Store(cfg) }

// GetConfig atomically loads the current configuration.
func (b *Backend) GetConfig() *config.Config { return b.config.Load() }

// SetWhitelist atomically replaces the current whitelist (may be nil).
func (b *Backend) SetWhitelist(wl *whitelist.WhitelistConfig) { b.whitelist.Store(wl) }

// GetWhitelist atomically loads the current whitelist (may be nil).
func (b *Backend) GetWhitelist() *whitelist.WhitelistConfig { return b.whitelist.Load() }

// NewSession is called for each new SMTP connection.
// It extracts the remote IP, checks whitelist membership, and returns a new Session.
func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	ip := ""
	if tcpAddr, ok := c.Conn().RemoteAddr().(*net.TCPAddr); ok {
		ip = tcpAddr.IP.String()
	}

	wl := b.GetWhitelist()
	whitelisted := wl != nil && wl.IsWhitelisted(ip)

	sessionID := uuid.New().String()
	if whitelisted {
		slog.Info("Whitelisted IP connected, AUTH will be skipped", "session", sessionID, "ip", ip)
	}

	cfg := b.GetConfig()
	metrics.ActiveConnections.Inc()
	metrics.ConnectionsTotal.Inc()
	return &Session{
		id:          sessionID,
		cfg:         cfg,
		wl:          wl,
		whitelisted: whitelisted,
		connectedAt: time.Now(),
	}, nil
}

// Session implements smtp.Session.
type Session struct {
	id          string // unique correlation ID for this session
	cfg         *config.Config
	wl          *whitelist.WhitelistConfig
	accessToken string
	fromEmail   string // override From header (from lookup table or whitelist config)
	from        string // SMTP envelope MAIL FROM address
	to          []string
	whitelisted bool
	connectedAt time.Time
}

// AuthMechanisms returns the list of supported SASL authentication mechanisms.
// Implements smtp.AuthSession.
func (s *Session) AuthMechanisms() []string {
	return []string{sasl.Plain, sasl.Login}
}

// Auth returns a SASL server for the requested mechanism.
// Implements smtp.AuthSession.
func (s *Session) Auth(mech string) (sasl.Server, error) {
	// Shared authenticate closure — called once both username and password
	// have been collected by the SASL exchange.
	doAuth := func(username, password string) error {
		result, err := auth.Authenticate(
			username, password,
			s.cfg.UsernameDelimiter,
			s.cfg.AzureTablesURL,
			s.cfg.AzureTablesPartKey,
			s.cfg.AzureAuthorityHost,
			s.cfg.GraphEndpoint,
		)
		if err != nil {
			slog.Warn("Authentication failed", "session", s.id, "error", err)
			metrics.AuthTotal.WithLabelValues("failure").Inc()
			return &smtp.SMTPError{
				Code:         535,
				EnhancedCode: smtp.EnhancedCode{5, 7, 8},
				Message:      err.Error(),
			}
		}
		s.accessToken = result.AccessToken
		s.fromEmail = result.FromEmail
		metrics.AuthTotal.WithLabelValues("success").Inc()
		slog.Info("SMTP session authenticated", "session", s.id, "fromEmail", s.fromEmail)
		return nil
	}

	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			return doAuth(username, password)
		}), nil
	case sasl.Login:
		return newLoginServer(doAuth), nil
	}
	return nil, smtp.ErrAuthUnknownMechanism
}

// loginServer implements sasl.Server for the LOGIN mechanism.
// The LOGIN exchange is:
//  1. Server → "Username:" challenge  (or client sends username as initial response)
//  2. Client → username
//  3. Server → "Password:" challenge
//  4. Client → password  →  authenticate
type loginServer struct {
	username string
	gotUser  bool
	onAuth   func(username, password string) error
}

func newLoginServer(onAuth func(username, password string) error) sasl.Server {
	return &loginServer{onAuth: onAuth}
}

func (l *loginServer) Next(response []byte) (challenge []byte, done bool, err error) {
	if !l.gotUser {
		if response == nil {
			// No initial response — ask for username.
			return []byte("Username:"), false, nil
		}
		// Client sent username as initial response.
		l.username = string(response)
		l.gotUser = true
		return []byte("Password:"), false, nil
	}
	// Second call — response is the password.
	err = l.onAuth(l.username, string(response))
	done = true
	return
}

// Mail is called on MAIL FROM. For whitelisted sessions that have not yet
// authenticated, it performs auto-authentication using the whitelist credentials.
// For regular sessions, it rejects the command if no token is present.
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.from = from

	if s.whitelisted && s.accessToken == "" {
		wl := s.wl
		token, err := auth.GetAccessToken(wl.TenantID, wl.ClientID, wl.ClientSecret, s.cfg.AzureAuthorityHost, s.cfg.GraphEndpoint)
		if err != nil {
			slog.Error("Auto-authentication failed for whitelisted session", "session", s.id, "error", err)
			metrics.WhitelistAuthTotal.WithLabelValues("failure").Inc()
			return &smtp.SMTPError{
				Code:         454,
				EnhancedCode: smtp.EnhancedCode{4, 7, 0},
				Message:      "Temporary authentication failure",
			}
		}
		s.accessToken = token
		s.fromEmail = wl.FromEmail
		metrics.WhitelistAuthTotal.WithLabelValues("success").Inc()
		slog.Info("Auto-authenticated whitelisted session", "session", s.id)
		return nil
	}

	if s.accessToken == "" {
		return &smtp.SMTPError{
			Code:         530,
			EnhancedCode: smtp.EnhancedCode{5, 7, 0},
			Message:      "Authentication required",
		}
	}

	// Enforce sender domain allowlist if configured.
	if domains := s.cfg.AllowedFromDomains; len(domains) > 0 {
		fromLower := strings.ToLower(from)
		atIdx := strings.LastIndex(fromLower, "@")
		if atIdx < 0 {
			slog.Warn("MAIL FROM rejected: no domain in address", "session", s.id, "from", from)
			return &smtp.SMTPError{
				Code:         553,
				EnhancedCode: smtp.EnhancedCode{5, 7, 1},
				Message:      "Sender address rejected: invalid address format",
			}
		}
		domain := fromLower[atIdx+1:]
		// Strip surrounding angle brackets if present (e.g. "example.com>")
		domain = strings.TrimRight(domain, ">")
		allowed := false
		for _, d := range domains {
			if d == domain {
				allowed = true
				break
			}
		}
		if !allowed {
			slog.Warn("MAIL FROM rejected: domain not in allowlist", "session", s.id, "from", from, "domain", domain)
			return &smtp.SMTPError{
				Code:         553,
				EnhancedCode: smtp.EnhancedCode{5, 7, 1},
				Message:      "Sender address rejected: domain not allowed",
			}
		}
	}

	return nil
}

// Rcpt is called on RCPT TO. Recipients are accumulated for use in Data().
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.to = append(s.to, to)
	return nil
}

// Data is called when the client sends the message body.
// It reads the raw MIME bytes, patches the To:/From: headers as needed,
// then forwards the message to Microsoft Graph API.
func (s *Session) Data(r io.Reader) error {
	if s.accessToken == "" {
		slog.Error("DATA received but session has no access token", "session", s.id)
		return &smtp.SMTPError{
			Code:         530,
			EnhancedCode: smtp.EnhancedCode{5, 7, 0},
			Message:      "Authentication required",
		}
	}

	// Enforce the server-side message size limit.  We read one extra byte so
	// that an exact-limit message is still accepted while an over-limit one is
	// unambiguously detected.
	maxSize := s.cfg.MaxMessageSize
	raw, err := io.ReadAll(io.LimitReader(r, maxSize+1))
	if err != nil {
		slog.Error("Failed to read DATA body", "session", s.id, "error", err)
		return &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 0, 0},
			Message:      "Transaction failed",
		}
	}
	if int64(len(raw)) > maxSize {
		slog.Warn("Message rejected: exceeds size limit", "session", s.id, "size", len(raw), "limit", maxSize)
		metrics.MessagesTotal.WithLabelValues("rejected").Inc()
		return &smtp.SMTPError{
			Code:         552,
			EnhancedCode: smtp.EnhancedCode{5, 3, 4},
			Message:      "Message size exceeds fixed maximum message size",
		}
	}

	// Patch MIME headers: inject missing To:, replace From: if fromEmail is set.
	patched, err := patchHeaders(raw, s.to, s.fromEmail)
	if err != nil {
		slog.Error("Failed to patch MIME headers", "session", s.id, "error", err)
		return &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 0, 0},
			Message:      "Transaction failed",
		}
	}

	// Extract Message-ID and Subject for logging/correlation.
	var msgID, subject string
	if parsed, parseErr := mail.ReadMessage(bytes.NewReader(patched)); parseErr == nil {
		msgID = parsed.Header.Get("Message-Id")
		subject = parsed.Header.Get("Subject")
		if len(subject) > 78 {
			subject = subject[:78]
		}
	}

	slog.Info("Handling message", "session", s.id, "from", s.from, "to", s.to, "messageId", msgID, "subject", subject)

	// Optionally strip privacy-sensitive headers before forwarding.
	if s.cfg.SanitizeHeaders {
		patched = sanitizeHeaders(patched)
	}

	// Determine effective sender: fromEmail (from lookup/whitelist) takes priority.
	sender := s.from
	if s.fromEmail != "" {
		sender = s.fromEmail
	}

	if err := graph.SendMail(
		s.accessToken, patched, sender,
		s.cfg.RetryAttempts,
		time.Duration(s.cfg.RetryBaseDelay)*time.Second,
		time.Duration(s.cfg.HTTPTimeout)*time.Second,
		s.cfg.GraphEndpoint,
	); err != nil {
		slog.Error("Graph API send failed", "session", s.id, "error", err, "messageId", msgID)
		if s.cfg.FailureWebhookURL != "" {
			webhook.NotifyFailureAsync(s.cfg.FailureWebhookURL, webhook.FailurePayload{
				From:      sender,
				To:        s.to,
				Error:     err.Error(),
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Attempts:  s.cfg.RetryAttempts,
			})
		}
		// Permanent failures (e.g. 400 Bad Request, 403 Forbidden from Graph) get a 5xx so
		// the client does not uselessly retry a message that will never be accepted.
		// All other failures (transient Graph errors, network issues, retry exhaustion)
		// get a 4xx temporary response so the SMTP client queues and retries automatically.
		var permErr *graph.PermanentError
		if errors.As(err, &permErr) {
			metrics.MessagesTotal.WithLabelValues("permanent_failure").Inc()
			return &smtp.SMTPError{
				Code:         554,
				EnhancedCode: smtp.EnhancedCode{5, 0, 0},
				Message:      "Transaction failed",
			}
		}
		metrics.MessagesTotal.WithLabelValues("temporary_failure").Inc()
		return &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 4, 1},
			Message:      "Temporary delivery failure, please retry later",
		}
	}

	metrics.MessagesTotal.WithLabelValues("sent").Inc()
	metrics.MessageSize.Observe(float64(len(patched)))
	metrics.RecipientsPerMessage.Observe(float64(len(s.to)))
	slog.Info("Message delivered successfully", "session", s.id, "sender", sender, "recipients", s.to, "messageId", msgID)
	return nil
}

// Reset clears per-message state so the session can handle the next message.
func (s *Session) Reset() {
	s.from = ""
	s.to = nil
}

// Logout is called when the client disconnects.
func (s *Session) Logout() error {
	metrics.SessionDuration.Observe(time.Since(s.connectedAt).Seconds())
	metrics.ActiveConnections.Dec()
	return nil
}

// headerOrder scans the raw normalized header block (up to the first blank
// \r\n\r\n line) and returns canonicalized header names in their original
// order, deduplicated. Continuation lines (starting with space/tab) are
// attributed to the preceding header and do not produce a new entry.
func headerOrder(norm []byte) []string {
	var order []string
	seen := make(map[string]bool)
	lines := bytes.Split(norm, []byte("\r\n"))
	for _, line := range lines {
		if len(line) == 0 {
			break // blank line = end of headers
		}
		if line[0] == ' ' || line[0] == '\t' {
			continue // continuation line
		}
		idx := bytes.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		name := textproto.CanonicalMIMEHeaderKey(string(line[:idx]))
		if !seen[name] {
			seen[name] = true
			order = append(order, name)
		}
	}
	return order
}

// sanitizeHeaders removes headers that could reveal the originating client's
// identity or infrastructure. Called when SANITIZE_HEADERS=true.
// If parsing fails the original bytes are returned unchanged (delivery is never
// blocked due to sanitization).
func sanitizeHeaders(input []byte) []byte {
	// Headers to strip — case-insensitive match via net/mail canonical form.
	privacyHeaders := []string{
		"Received",
		"X-Originating-Ip",
		"X-Mailer",
		"User-Agent",
		"X-Forwarded-To",
		"X-Forwarded-For",
		"X-Original-To",
	}

	// net/mail canonicalises header names ("x-mailer" → "X-Mailer"), so we
	// compare against the canonical forms stored in privacyHeaders above.
	norm := bytes.ReplaceAll(input, []byte("\r\n"), []byte("\n"))
	norm = bytes.ReplaceAll(norm, []byte("\r"), []byte("\n"))
	norm = bytes.ReplaceAll(norm, []byte("\n"), []byte("\r\n"))

	msg, err := mail.ReadMessage(bytes.NewReader(norm))
	if err != nil {
		slog.Debug("sanitizeHeaders: failed to parse message, skipping sanitization", "error", err)
		return input
	}

	removed := []string{}
	for _, h := range privacyHeaders {
		if _, exists := msg.Header[h]; exists {
			delete(msg.Header, h)
			removed = append(removed, h)
		}
	}
	if len(removed) > 0 {
		slog.Debug("sanitizeHeaders: removed privacy headers", "headers", removed)
	} else {
		slog.Debug("sanitizeHeaders: no privacy headers found to remove")
	}

	body, err := io.ReadAll(msg.Body)
	if err != nil {
		slog.Debug("sanitizeHeaders: failed to read body, skipping sanitization", "error", err)
		return input
	}

	order := headerOrder(norm)

	var buf bytes.Buffer
	for _, key := range order {
		vals, ok := msg.Header[key]
		if !ok {
			continue // was deleted (privacy header)
		}
		for _, v := range vals {
			fmt.Fprintf(&buf, "%s: %s\r\n", key, v)
		}
	}
	buf.WriteString("\r\n")
	body = bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
	body = bytes.ReplaceAll(body, []byte("\r"), []byte("\n"))
	body = bytes.ReplaceAll(body, []byte("\n"), []byte("\r\n"))
	buf.Write(body)

	return buf.Bytes()
}

// patchHeaders manipulates the MIME message to ensure the To: and From: headers
// are correct before forwarding to the Microsoft Graph sendMail endpoint.
//
// It mirrors the Python implementation: parse with net/mail (lenient, handles
// malformed messages without a blank-line separator), patch the header map,
// then re-serialize with \r\n line endings as required by RFC2822 and Graph.
//
//   - Injects a "To:" header from rcptTos when absent.
//   - Replaces "From:" with fromEmail when fromEmail is non-empty.
func patchHeaders(input []byte, rcptTos []string, fromEmail string) ([]byte, error) {
	// Normalise to \r\n before handing to net/mail, which expects CRLF.
	norm := bytes.ReplaceAll(input, []byte("\r\n"), []byte("\n"))
	norm = bytes.ReplaceAll(norm, []byte("\r"), []byte("\n"))
	norm = bytes.ReplaceAll(norm, []byte("\n"), []byte("\r\n"))

	msg, err := mail.ReadMessage(bytes.NewReader(norm))
	if err != nil {
		// Malformed message (e.g. no blank line between headers and body).
		// Fall back: prepend the required headers and a separator, then
		// append the original content as the body.
		slog.Debug("net/mail parse failed, using fallback header injection", "error", err)
		var prefix strings.Builder
		if fromEmail != "" {
			fmt.Fprintf(&prefix, "From: %s\r\n", fromEmail)
		}
		if len(rcptTos) > 0 {
			fmt.Fprintf(&prefix, "To: %s\r\n", strings.Join(rcptTos, ", "))
		}
		prefix.WriteString("\r\n")
		slog.Debug("Fallback injected headers", "prefix", prefix.String())
		return append([]byte(prefix.String()), norm...), nil
	}

	// --- Patch the header map (same logic as Python envel['To'] / envel['From']) ---

	// Inject To: from SMTP envelope recipients when absent in the MIME headers.
	// Graph sendMail resolves recipients from MIME headers, not the SMTP envelope.
	if len(msg.Header["To"]) == 0 && len(rcptTos) > 0 {
		msg.Header["To"] = []string{strings.Join(rcptTos, ", ")}
	}

	// Replace From: when the session has a lookup/whitelist override.
	if fromEmail != "" {
		msg.Header["From"] = []string{fromEmail}
	}

	// --- Re-serialize with \r\n endings ---
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		return nil, fmt.Errorf("reading message body: %w", err)
	}

	order := headerOrder(norm)

	var buf bytes.Buffer
	written := make(map[string]bool)
	for _, key := range order {
		vals, ok := msg.Header[key]
		if !ok {
			continue
		}
		for _, v := range vals {
			fmt.Fprintf(&buf, "%s: %s\r\n", key, v)
		}
		written[key] = true
	}
	// Write any headers that were injected and not in the original order.
	for key, vals := range msg.Header {
		if written[key] {
			continue
		}
		for _, v := range vals {
			fmt.Fprintf(&buf, "%s: %s\r\n", key, v)
		}
	}
	buf.WriteString("\r\n")

	// Normalise body line endings to \r\n.
	body = bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
	body = bytes.ReplaceAll(body, []byte("\r"), []byte("\n"))
	body = bytes.ReplaceAll(body, []byte("\n"), []byte("\r\n"))
	buf.Write(body)

	slog.Debug("Patched MIME headers", "headers", buf.String()[:min(buf.Len(), 500)])

	return buf.Bytes(), nil
}
