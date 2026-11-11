package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/mailer"
)

// sendCampaign sends the campaign emails to all unsent targets.
// POST /api/social-eng/campaigns/{id}/send
func (s *Server) sendCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}

	campaign, err := s.m.pg.GetCampaign(id)
	if err != nil {
		writeErr(w, 404, "campaign not found")
		return
	}

	targets, err := s.m.pg.ListTargets(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	var senderCfg *db.SESender
	if campaign.SenderID != nil {
		senderCfg, _ = s.m.pg.GetSender(*campaign.SenderID)
	}
	if senderCfg == nil {
		writeErr(w, 422, "campaign has no sender configured")
		return
	}

	sent, failed := 0, 0
	var errs []string

	for _, t := range targets {
		if t.SentAt != nil {
			continue // already sent
		}
		if t.Email == "" {
			continue
		}

		vars := mailer.TargetVars{
			Name:       t.Name,
			FirstName:  mailer.FirstName(t.Name),
			LastName:   mailer.LastName(t.Name),
			Email:      t.Email,
			Department: t.Department,
		}

		// Build tracking link
		trackBase := r.Header.Get("X-Forwarded-Proto")
		if trackBase == "" {
			trackBase = "http"
		}
		host := r.Host
		vars.TrackingLink = fmt.Sprintf("%s://%s/api/social-eng/track?t=%d&e=opened", trackBase, host, t.ID)
		vars.Link = campaign.RedirectURL
		if vars.Link == "" {
			vars.Link = fmt.Sprintf("%s://%s/api/social-eng/track?t=%d&e=clicked", trackBase, host, t.ID)
		}

		htmlBody, _ := mailer.RenderHTML(campaign.HTMLBody, vars)
		textBody := mailer.RenderText(campaign.TextBody, vars)
		subject := mailer.RenderText(campaign.Subject, vars)

		// Inject tracking pixel into HTML
		if htmlBody != "" {
			htmlBody += fmt.Sprintf(`<img src="%s" width="1" height="1" style="display:none" alt="">`, vars.TrackingLink)
		}

		msg := mailer.Message{
			From:          campaign.FromEmail,
			FromName:      campaign.FromName,
			SpoofFrom:     campaign.SpoofFrom,
			SpoofFromName: campaign.SpoofFromName,
			To:            []string{t.Email},
			Subject:       subject,
			HTMLBody:      htmlBody,
			TextBody:      textBody,
		}

		if err := dispatchMessage(r.Context(), senderCfg, msg); err != nil {
			failed++
			errs = append(errs, fmt.Sprintf("%s: %s", t.Email, err.Error()))
			_ = s.m.pg.MarkTargetError(t.ID, err.Error())
		} else {
			sent++
			_ = s.m.pg.MarkTargetSent(t.ID)
		}
	}

	writeJSON(w, 200, map[string]any{
		"sent":   sent,
		"failed": failed,
		"errors": errs,
	})
}

// sendTestEmail sends a single test email to the address in the request body.
// POST /api/social-eng/campaigns/{id}/send-test
func (s *Server) sendTestEmail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}

	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		writeErr(w, 400, "email is required")
		return
	}

	campaign, err := s.m.pg.GetCampaign(id)
	if err != nil {
		writeErr(w, 404, "campaign not found")
		return
	}

	var senderCfg *db.SESender
	if campaign.SenderID != nil {
		senderCfg, _ = s.m.pg.GetSender(*campaign.SenderID)
	}
	if senderCfg == nil {
		writeErr(w, 422, "campaign has no sender configured")
		return
	}

	name := req.Name
	if name == "" {
		name = "Test User"
	}
	vars := mailer.TargetVars{
		Name:         name,
		FirstName:    mailer.FirstName(name),
		LastName:     mailer.LastName(name),
		Email:        req.Email,
		Link:         "https://example.com/test",
		TrackingLink: "https://example.com/track",
		InvoiceNo:    "INV-TEST-001",
		TicketNo:     "TKT-TEST-001",
	}

	htmlBody, _ := mailer.RenderHTML(campaign.HTMLBody, vars)
	textBody := mailer.RenderText(campaign.TextBody, vars)
	subject := "[TEST] " + mailer.RenderText(campaign.Subject, vars)

	msg := mailer.Message{
		From:          campaign.FromEmail,
		FromName:      campaign.FromName,
		SpoofFrom:     campaign.SpoofFrom,
		SpoofFromName: campaign.SpoofFromName,
		To:            []string{req.Email},
		Subject:       subject,
		HTMLBody:      htmlBody,
		TextBody:      textBody,
	}

	if err := dispatchMessage(r.Context(), senderCfg, msg); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"ok": "sent"})
}

// dispatchMessage routes a message through the right sender type.
func dispatchMessage(ctx context.Context, snd *db.SESender, msg mailer.Message) error {
	cfg := snd.Config
	if cfg == nil {
		cfg = map[string]any{}
	}

	switch strings.ToLower(snd.Kind) {
	case "smtp":
		smtpCfg := mailer.SMTPConfig{
			Host:       strField(cfg, "host"),
			Port:       intField(cfg, "port"),
			Username:   strField(cfg, "username"),
			Password:   strField(cfg, "password"),
			TLSMode:    strField(cfg, "tls_mode"),
			SkipVerify: boolField(cfg, "skip_verify"),
		}
		if msg.From == "" {
			msg.From = snd.FromEmail
		}
		if msg.FromName == "" {
			msg.FromName = snd.FromName
		}
		return mailer.SendSMTP(smtpCfg, msg)

	case "agentmail":
		amCfg := mailer.AgentMailConfig{
			APIKey:  strField(cfg, "api_key"),
			InboxID: strField(cfg, "inbox_id"),
			BaseURL: strField(cfg, "base_url"),
		}
		return mailer.SendAgentMail(ctx, amCfg, msg)

	case "webhook":
		whCfg := mailer.WebhookConfig{
			URL:    strField(cfg, "url"),
			Method: strField(cfg, "method"),
		}
		if headers, ok := cfg["headers"].(map[string]any); ok {
			whCfg.Headers = make(map[string]string)
			for k, v := range headers {
				whCfg.Headers[k] = fmt.Sprintf("%v", v)
			}
		}
		return mailer.SendWebhook(ctx, whCfg, msg)

	default:
		return fmt.Errorf("unknown sender kind: %s", snd.Kind)
	}
}

// ---- config field helpers ----

func strField(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

func intField(m map[string]any, k string) int {
	switch v := m[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

func boolField(m map[string]any, k string) bool {
	v, _ := m[k].(bool)
	return v
}
