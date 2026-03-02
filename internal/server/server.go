package server

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/mail"
	"strings"

	"github.com/Palasito/go-smtp/internal/auth"
	"github.com/Palasito/go-smtp/internal/config"
	"github.com/Palasito/go-smtp/internal/graph"
	"github.com/Palasito/go-smtp/internal/whitelist"
	"github.com/emersion/go-sasl"
	smtp "github.com/emersion/go-smtp"
)

// Backend implements smtp.Backend.
type Backend struct {
	Config    *config.Config
	Whitelist *whitelist.WhitelistConfig // may be nil
}

// NewSession is called for each new SMTP connection.
// It extracts the remote IP, checks whitelist membership, and returns a new Session.
func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	ip := ""
	if tcpAddr, ok := c.Conn().RemoteAddr().(*net.TCPAddr); ok {
		ip = tcpAddr.IP.String()
	}

	whitelisted := b.Whitelist != nil && b.Whitelist.IsWhitelisted(ip)
	if whitelisted {
		slog.Info("Whitelisted IP connected, AUTH will be skipped", "ip", ip)
	}

	return &Session{
		backend:     b,
		whitelisted: whitelisted,
	}, nil
}

// Session implements smtp.Session.
type Session struct {
	backend     *Backend
	accessToken string
	fromEmail   string // override From header (from lookup table or whitelist config)
	from        string // SMTP envelope MAIL FROM address
	to          []string
	whitelisted bool
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
			s.backend.Config.UsernameDelimiter,
			s.backend.Config.AzureTablesURL,
			s.backend.Config.AzureTablesPartKey,
		)
		if err != nil {
			slog.Warn("Authentication failed", "error", err)
			return &smtp.SMTPError{
				Code:         535,
				EnhancedCode: smtp.EnhancedCode{5, 7, 8},
				Message:      err.Error(),
			}
		}
		s.accessToken = result.AccessToken
		s.fromEmail = result.FromEmail
		slog.Info("SMTP session authenticated", "fromEmail", s.fromEmail)
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
		wl := s.backend.Whitelist
		token, err := auth.GetAccessToken(wl.TenantID, wl.ClientID, wl.ClientSecret)
		if err != nil {
			slog.Error("Auto-authentication failed for whitelisted session", "error", err)
			return &smtp.SMTPError{
				Code:         454,
				EnhancedCode: smtp.EnhancedCode{4, 7, 0},
				Message:      "Temporary authentication failure",
			}
		}
		s.accessToken = token
		s.fromEmail = wl.FromEmail
		slog.Info("Auto-authenticated whitelisted session")
		return nil
	}

	if s.accessToken == "" {
		return &smtp.SMTPError{
			Code:         530,
			EnhancedCode: smtp.EnhancedCode{5, 7, 0},
			Message:      "Authentication required",
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
		slog.Error("DATA received but session has no access token")
		return &smtp.SMTPError{
			Code:         530,
			EnhancedCode: smtp.EnhancedCode{5, 7, 0},
			Message:      "Authentication required",
		}
	}

	raw, err := io.ReadAll(r)
	if err != nil {
		slog.Error("Failed to read DATA body", "error", err)
		return &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 0, 0},
			Message:      "Transaction failed",
		}
	}

	slog.Info("Handling message", "from", s.from, "to", s.to)

	// Patch MIME headers: inject missing To:, replace From: if fromEmail is set.
	patched, err := patchHeaders(raw, s.to, s.fromEmail)
	if err != nil {
		slog.Error("Failed to patch MIME headers", "error", err)
		return &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 0, 0},
			Message:      "Transaction failed",
		}
	}

	// Determine effective sender: fromEmail (from lookup/whitelist) takes priority.
	sender := s.from
	if s.fromEmail != "" {
		sender = s.fromEmail
	}

	if err := graph.SendMail(s.accessToken, patched, sender); err != nil {
		slog.Error("Graph API send failed", "error", err)
		return &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 0, 0},
			Message:      "Transaction failed",
		}
	}

	slog.Info("Message delivered successfully", "sender", sender, "recipients", s.to)
	return nil
}

// Reset clears per-message state so the session can handle the next message.
func (s *Session) Reset() {
	s.from = ""
	s.to = nil
}

// Logout is called when the client disconnects.
func (s *Session) Logout() error {
	return nil
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

	var buf bytes.Buffer
	for key, vals := range msg.Header {
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
