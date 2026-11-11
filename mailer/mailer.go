// Package mailer provides email sending for social engineering campaigns.
// It supports SMTP (with STARTTLS/TLS), AgentMail API, and a generic HTTP
// webhook sender. All senders honour the SpoofFrom fields — the envelope
// MAIL FROM uses the authenticated sender while the RFC 2822 From header is
// set to the spoofed address so the recipient's MUA displays the target name.
package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

// Message is the full payload to deliver.
type Message struct {
	// Envelope sender — must be an address the SMTP auth controls.
	From     string
	FromName string

	// If non-empty, the RFC 2822 From/Reply-To headers show this address
	// instead of From. The SMTP MAIL FROM envelope stays as From.
	SpoofFrom     string
	SpoofFromName string

	To      []string // primary recipients
	CC      []string
	Subject string

	HTMLBody string
	TextBody string

	// Extra headers (e.g. X-Mailer, X-Priority, List-Unsubscribe).
	ExtraHeaders map[string]string

	// Optional attachment: filename → raw bytes.
	Attachments map[string][]byte
}

// addr formats "Name <email>" or just "email".
func addr(name, email string) string {
	if name == "" {
		return email
	}
	encoded := mime.QEncoding.Encode("utf-8", name)
	return encoded + " <" + email + ">"
}

// displayFrom returns what goes in the From header.
func (m Message) displayFrom() string {
	if m.SpoofFrom != "" {
		return addr(m.SpoofFrom, m.SpoofFrom) // SpoofFrom is already the full "Name <email>" or bare email
	}
	return addr(m.FromName, m.From)
}

// build constructs the raw MIME message.
func (m Message) Build() ([]byte, error) {
	var buf bytes.Buffer

	// Headers
	from := m.displayFrom()
	buf.WriteString("From: " + from + "\r\n")
	if m.SpoofFrom != "" {
		// Reply-To points back to the real sender so replies don't bounce.
		buf.WriteString("Reply-To: " + addr(m.FromName, m.From) + "\r\n")
	}
	tos := make([]string, len(m.To))
	for i, t := range m.To {
		tos[i] = t
	}
	buf.WriteString("To: " + strings.Join(tos, ", ") + "\r\n")
	if len(m.CC) > 0 {
		buf.WriteString("Cc: " + strings.Join(m.CC, ", ") + "\r\n")
	}
	buf.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", m.Subject) + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	buf.WriteString("X-Mailer: ARTEX/1.0\r\n")
	for k, v := range m.ExtraHeaders {
		buf.WriteString(k + ": " + v + "\r\n")
	}

	if len(m.Attachments) > 0 {
		return m.buildMultipart(&buf)
	}

	if m.HTMLBody != "" && m.TextBody != "" {
		return m.buildAlternative(&buf)
	}
	if m.HTMLBody != "" {
		buf.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		qp := quotedprintable.NewWriter(&buf)
		qp.Write([]byte(m.HTMLBody))
		qp.Close()
		return buf.Bytes(), nil
	}
	buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	qp := quotedprintable.NewWriter(&buf)
	qp.Write([]byte(m.TextBody))
	qp.Close()
	return buf.Bytes(), nil
}

func (m Message) buildAlternative(buf *bytes.Buffer) ([]byte, error) {
	mw := multipart.NewWriter(buf)
	buf.WriteString(`Content-Type: multipart/alternative; boundary="` + mw.Boundary() + `"` + "\r\n\r\n")
	writeTextPart(mw, "text/plain", m.TextBody)
	writeHTMLPart(mw, m.HTMLBody)
	mw.Close()
	return buf.Bytes(), nil
}

func (m Message) buildMultipart(buf *bytes.Buffer) ([]byte, error) {
	mw := multipart.NewWriter(buf)
	buf.WriteString(`Content-Type: multipart/mixed; boundary="` + mw.Boundary() + `"` + "\r\n\r\n")
	if m.HTMLBody != "" && m.TextBody != "" {
		// nested alternative inside mixed
		altBuf := &bytes.Buffer{}
		altW := multipart.NewWriter(altBuf)
		altBuf.WriteString(`Content-Type: multipart/alternative; boundary="` + altW.Boundary() + `"` + "\r\n\r\n")
		writeTextPart(altW, "text/plain", m.TextBody)
		writeHTMLPart(altW, m.HTMLBody)
		altW.Close()
		ph, _ := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {`multipart/alternative; boundary="` + altW.Boundary() + `"`}})
		ph.Write(altBuf.Bytes())
	} else if m.HTMLBody != "" {
		writeHTMLPart(mw, m.HTMLBody)
	} else {
		writeTextPart(mw, "text/plain", m.TextBody)
	}
	for name, data := range m.Attachments {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", "application/octet-stream")
		h.Set("Content-Disposition", `attachment; filename="`+name+`"`)
		h.Set("Content-Transfer-Encoding", "base64")
		part, err := mw.CreatePart(h)
		if err != nil {
			return nil, err
		}
		enc := base64.NewEncoder(base64.StdEncoding, part)
		enc.Write(data)
		enc.Close()
	}
	mw.Close()
	return buf.Bytes(), nil
}

func writeTextPart(mw *multipart.Writer, ct, body string) {
	h := textproto.MIMEHeader{"Content-Type": {ct + "; charset=utf-8"}, "Content-Transfer-Encoding": {"quoted-printable"}}
	pw, _ := mw.CreatePart(h)
	qp := quotedprintable.NewWriter(pw)
	qp.Write([]byte(body))
	qp.Close()
}

func writeHTMLPart(mw *multipart.Writer, body string) {
	writeTextPart(mw, "text/html", body)
}

// ---- SMTP ----

// SMTPConfig holds connection details for an SMTP relay.
type SMTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	// TLSMode: "none" | "starttls" | "tls"
	TLSMode    string `json:"tls_mode"`
	SkipVerify bool   `json:"skip_verify"`
}

// SendSMTP delivers msg through cfg.
func SendSMTP(cfg SMTPConfig, msg Message) error {
	raw, err := msg.Build()
	if err != nil {
		return fmt.Errorf("mailer: build: %w", err)
	}

	host := cfg.Host
	port := cfg.Port
	if port == 0 {
		switch cfg.TLSMode {
		case "tls":
			port = 465
		default:
			port = 587
		}
	}
	addr := fmt.Sprintf("%s:%d", host, port)

	tlsCfg := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: cfg.SkipVerify,
	}

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, host)
	}

	var c *smtp.Client

	if cfg.TLSMode == "tls" {
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("mailer: tls dial: %w", err)
		}
		c, err = smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("mailer: smtp new client: %w", err)
		}
	} else {
		conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
		if err != nil {
			return fmt.Errorf("mailer: dial: %w", err)
		}
		c, err = smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("mailer: smtp new client: %w", err)
		}
		if cfg.TLSMode == "starttls" || cfg.TLSMode == "" {
			if ok, _ := c.Extension("STARTTLS"); ok {
				if err := c.StartTLS(tlsCfg); err != nil {
					return fmt.Errorf("mailer: starttls: %w", err)
				}
			}
		}
	}
	defer c.Quit()

	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("mailer: auth: %w", err)
		}
	}

	// Envelope sender is always the real (authenticated) address.
	if err := c.Mail(msg.From); err != nil {
		return fmt.Errorf("mailer: MAIL FROM: %w", err)
	}
	for _, rcpt := range msg.To {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("mailer: RCPT TO %s: %w", rcpt, err)
		}
	}
	for _, rcpt := range msg.CC {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("mailer: RCPT TO %s: %w", rcpt, err)
		}
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("mailer: DATA: %w", err)
	}
	if _, err := wc.Write(raw); err != nil {
		return fmt.Errorf("mailer: write body: %w", err)
	}
	return wc.Close()
}

// ---- AgentMail ----

// AgentMailConfig holds an AgentMail API configuration.
// AgentMail (agentmail.to) provides inboxes for AI agents; each inbox
// has a unique address and an API key for sending.
type AgentMailConfig struct {
	APIKey  string `json:"api_key"`
	InboxID string `json:"inbox_id"` // e.g. "abc123@agentmail.to"
	// BaseURL defaults to https://api.agentmail.to/v0
	BaseURL string `json:"base_url,omitempty"`
}

type agentMailPayload struct {
	To      []agentMailAddr `json:"to"`
	CC      []agentMailAddr `json:"cc,omitempty"`
	From    *agentMailAddr  `json:"from,omitempty"`
	Subject string          `json:"subject"`
	Text    string          `json:"text,omitempty"`
	HTML    string          `json:"html,omitempty"`
}

type agentMailAddr struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

// SendAgentMail sends msg via the AgentMail REST API.
func SendAgentMail(ctx context.Context, cfg AgentMailConfig, msg Message) error {
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.agentmail.to/v0"
	}
	inboxID := cfg.InboxID
	if inboxID == "" {
		return fmt.Errorf("agentmail: inbox_id is required")
	}

	payload := agentMailPayload{
		Subject: msg.Subject,
		Text:    msg.TextBody,
		HTML:    msg.HTMLBody,
	}
	for _, t := range msg.To {
		payload.To = append(payload.To, agentMailAddr{Email: t})
	}
	for _, t := range msg.CC {
		payload.CC = append(payload.CC, agentMailAddr{Email: t})
	}
	// Spoof the From display name/address if set.
	if msg.SpoofFrom != "" {
		payload.From = &agentMailAddr{Name: msg.SpoofFromName, Email: msg.SpoofFrom}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("agentmail: marshal: %w", err)
	}

	url := fmt.Sprintf("%s/inboxes/%s/messages", base, inboxID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("agentmail: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("agentmail: http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("agentmail: HTTP %d: %s", resp.StatusCode, string(rb))
	}
	return nil
}

// ---- Generic Webhook / HTTP API sender ----

// WebhookConfig calls a generic HTTP endpoint (SendGrid, Mailgun, Resend, etc.)
// using a simple JSON POST. Configure template fields to map our payload.
type WebhookConfig struct {
	URL        string            `json:"url"`
	Method     string            `json:"method"` // default POST
	Headers    map[string]string `json:"headers"`
	BodyTpl    string            `json:"body_tpl"` // Go template; vars: .From .To .Subject .HTML .Text
}

// SendWebhook posts msg to a generic HTTP endpoint.
func SendWebhook(ctx context.Context, cfg WebhookConfig, msg Message) error {
	method := cfg.Method
	if method == "" {
		method = http.MethodPost
	}
	payload := map[string]any{
		"from":    msg.displayFrom(),
		"to":      msg.To,
		"subject": msg.Subject,
		"html":    msg.HTMLBody,
		"text":    msg.TextBody,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, method, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("webhook: HTTP %d: %s", resp.StatusCode, string(rb))
	}
	return nil
}
