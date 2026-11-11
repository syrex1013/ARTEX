package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/Autumn-27/artex/db"
)

// Social Engineering API.

// ---- Campaigns ----------------------------------------------------------------

func (s *Server) listCampaigns(w http.ResponseWriter, r *http.Request) {
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var engID *int64
	if v := r.URL.Query().Get("engagement_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			engID = &id
		}
	}
	list, err := s.m.pg.ListCampaigns(engID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) createCampaign(w http.ResponseWriter, r *http.Request) {
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var c db.SocialEngCampaign
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	got, err := s.m.pg.CreateCampaign(c)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 201, got)
}

func (s *Server) getCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	c, err := s.m.pg.GetCampaign(id)
	if err != nil {
		if errors.Is(err, db.ErrCampaignNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}
	writeJSON(w, 200, c)
}

// patchCampaign handles both full field updates and status-only patches.
func (s *Server) patchCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}

	// Decode as generic map to distinguish status-only vs full update
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}

	// If only status is provided, do a lightweight status update
	if len(body) == 1 {
		if raw, ok2 := body["status"]; ok2 {
			var status string
			if err := json.Unmarshal(raw, &status); err != nil {
				writeErr(w, 400, "invalid status")
				return
			}
			if err := s.m.pg.UpdateCampaignStatus(id, status); err != nil {
				writeErr(w, 422, err.Error())
				return
			}
			c, _ := s.m.pg.GetCampaign(id)
			writeJSON(w, 200, c)
			return
		}
	}

	// Full update: load existing campaign, merge, then save
	existing, err := s.m.pg.GetCampaign(id)
	if err != nil {
		if errors.Is(err, db.ErrCampaignNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}

	// Re-decode the original body into a campaign struct
	fullJSON, _ := json.Marshal(body)
	var patch db.SocialEngCampaign
	if err := json.Unmarshal(fullJSON, &patch); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}

	// Merge non-zero fields
	if patch.Name != "" { existing.Name = patch.Name }
	if patch.Kind != "" { existing.Kind = patch.Kind }
	if patch.Status != "" { existing.Status = patch.Status }
	if patch.Objective != "" { existing.Objective = patch.Objective }
	if patch.Technique != "" { existing.Technique = patch.Technique }
	if patch.Pretext != "" { existing.Pretext = patch.Pretext }
	if patch.TemplateName != "" { existing.TemplateName = patch.TemplateName }
	if patch.RedirectURL != "" { existing.RedirectURL = patch.RedirectURL }
	if patch.SenderID != nil { existing.SenderID = patch.SenderID }
	if patch.LandingPageID != nil { existing.LandingPageID = patch.LandingPageID }
	if patch.FromEmail != "" { existing.FromEmail = patch.FromEmail }
	if patch.FromName != "" { existing.FromName = patch.FromName }
	if patch.SpoofFrom != "" { existing.SpoofFrom = patch.SpoofFrom }
	if patch.SpoofFromName != "" { existing.SpoofFromName = patch.SpoofFromName }
	if patch.Subject != "" { existing.Subject = patch.Subject }
	if patch.HTMLBody != "" { existing.HTMLBody = patch.HTMLBody }
	if patch.TextBody != "" { existing.TextBody = patch.TextBody }

	got, err := s.m.pg.UpdateCampaign(id, *existing)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 200, got)
}

func (s *Server) deleteCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	if err := s.m.pg.DeleteCampaign(id); err != nil {
		if errors.Is(err, db.ErrCampaignNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}
	w.WriteHeader(204)
}

// ---- Targets ----------------------------------------------------------------

func (s *Server) listCampaignTargets(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	targets, err := s.m.pg.ListTargets(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, targets)
}

func (s *Server) addCampaignTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var t db.SocialEngTarget
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	t.CampaignID = id
	got, err := s.m.pg.AddTarget(t)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 201, got)
}

// ---- Tracking ---------------------------------------------------------------

// trackCampaignEvent is a public endpoint called by tracking pixels/redirect links.
func (s *Server) trackCampaignEvent(w http.ResponseWriter, r *http.Request) {
	if s.m.pg == nil {
		w.WriteHeader(204)
		return
	}
	tidStr := r.URL.Query().Get("t")
	event := r.URL.Query().Get("e")
	if tidStr == "" || event == "" {
		w.WriteHeader(400)
		return
	}
	tid, err := strconv.ParseInt(tidStr, 10, 64)
	if err != nil {
		w.WriteHeader(400)
		return
	}
	_ = s.m.pg.RecordTargetEvent(tid, event)
	// Serve a 1×1 transparent PNG for pixel tracking
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(transparentPNG)
}

// transparentPNG is a 1×1 transparent PNG.
var transparentPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x62, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// ---- Senders ----------------------------------------------------------------

func (s *Server) listSenders(w http.ResponseWriter, r *http.Request) {
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	list, err := s.m.pg.ListSenders()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) createSender(w http.ResponseWriter, r *http.Request) {
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var snd db.SESender
	if err := json.NewDecoder(r.Body).Decode(&snd); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	got, err := s.m.pg.CreateSender(snd)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 201, got)
}

func (s *Server) getSender(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	snd, err := s.m.pg.GetSender(id)
	if err != nil {
		if errors.Is(err, db.ErrSenderNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}
	writeJSON(w, 200, snd)
}

func (s *Server) updateSender(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var snd db.SESender
	if err := json.NewDecoder(r.Body).Decode(&snd); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	got, err := s.m.pg.UpdateSender(id, snd)
	if err != nil {
		if errors.Is(err, db.ErrSenderNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 422, err.Error())
		}
		return
	}
	writeJSON(w, 200, got)
}

func (s *Server) deleteSender(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	if err := s.m.pg.DeleteSender(id); err != nil {
		if errors.Is(err, db.ErrSenderNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}
	w.WriteHeader(204)
}

// ---- Landing Pages ----------------------------------------------------------

func (s *Server) listLandingPages(w http.ResponseWriter, r *http.Request) {
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	list, err := s.m.pg.ListLandingPages()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) createLandingPage(w http.ResponseWriter, r *http.Request) {
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var p db.SELandingPage
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	got, err := s.m.pg.CreateLandingPage(p)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 201, got)
}

func (s *Server) getLandingPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	p, err := s.m.pg.GetLandingPage(id)
	if err != nil {
		if errors.Is(err, db.ErrLandingPageNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) updateLandingPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var p db.SELandingPage
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	got, err := s.m.pg.UpdateLandingPage(id, p)
	if err != nil {
		if errors.Is(err, db.ErrLandingPageNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 422, err.Error())
		}
		return
	}
	writeJSON(w, 200, got)
}

func (s *Server) deleteLandingPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	if err := s.m.pg.DeleteLandingPage(id); err != nil {
		if errors.Is(err, db.ErrLandingPageNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}
	w.WriteHeader(204)
}

// ---- Credential capture (public — no auth) ----------------------------------

// submitCredentials receives credentials from landing page form posts.
// Accepts JSON or application/x-www-form-urlencoded.
func (s *Server) submitCredentials(w http.ResponseWriter, r *http.Request) {
	if s.m.pg == nil {
		w.WriteHeader(503)
		return
	}

	data := map[string]any{}
	ct := r.Header.Get("Content-Type")

	if ct == "application/x-www-form-urlencoded" || ct == "multipart/form-data" {
		_ = r.ParseForm()
		for k, vs := range r.Form {
			if len(vs) == 1 {
				data[k] = vs[0]
			} else {
				data[k] = vs
			}
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			w.WriteHeader(400)
			return
		}
	}

	// Extract meta fields
	var cid, tid, lpid *int64
	if v, ok := data["_campaign_id"]; ok {
		if n, ok2 := toInt64(v); ok2 {
			cid = &n
			delete(data, "_campaign_id")
		}
	}
	if v, ok := data["_target_id"]; ok {
		if n, ok2 := toInt64(v); ok2 {
			tid = &n
			delete(data, "_target_id")
		}
	}
	if v, ok := data["_landing_page_id"]; ok {
		if n, ok2 := toInt64(v); ok2 {
			lpid = &n
			delete(data, "_landing_page_id")
		}
	}
	redirectURL, _ := data["_redirect"].(string)
	delete(data, "_redirect")

	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}

	cred := db.SECredential{
		CampaignID:    cid,
		TargetID:      tid,
		LandingPageID: lpid,
		IP:            ip,
		UserAgent:     r.Header.Get("User-Agent"),
		Data:          data,
	}
	_, _ = s.m.pg.StoreCreds(cred)

	if tid != nil {
		_ = s.m.pg.RecordTargetEvent(*tid, "submitted")
	}

	if redirectURL != "" {
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write([]byte(`{"ok":true}`))
}

func (s *Server) listCampaignCreds(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	creds, err := s.m.pg.ListCreds(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, creds)
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case string:
		i, err := strconv.ParseInt(n, 10, 64)
		return i, err == nil
	}
	return 0, false
}

// ---- Templates --------------------------------------------------------------

// seTemplates returns the built-in social engineering templates.
func (s *Server) seTplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, builtinSETemplates)
}

// SETemplate defines a ready-to-use social engineering pretext.
type SETemplate struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Technique   string `json:"technique"`
	Subject     string `json:"subject"`
	HTMLBody    string `json:"html_body"`
	TextBody    string `json:"text_body"`
	Pretext     string `json:"pretext"`
	SpoofDomain string `json:"spoof_domain,omitempty"`
}

var builtinSETemplates = []SETemplate{
	// --- Phishing -------------------------------------------------------------
	{
		Name: "IT Password Reset", Kind: "phishing", Technique: "credential_harvest",
		Subject:  "Action Required: Your password expires in 24 hours",
		Pretext:  "Urgency-based credential harvest pretending to be IT helpdesk.",
		HTMLBody: `<p>Dear {{.Name}},</p><p>Your account password will expire in <strong>24 hours</strong>. Please reset it immediately to avoid losing access.</p><p><a href="{{.Link}}">Reset Password Now</a></p><p>IT Support Team</p><p><small>If you did not request this, please ignore this message.</small></p>`,
		TextBody: "Dear {{.Name}},\n\nYour account password will expire in 24 hours. Please reset it here:\n{{.Link}}\n\nIT Support Team",
		SpoofDomain: "it-support.{{.Company}}",
	},
	{
		Name: "Invoice Approval", Kind: "phishing", Technique: "credential_harvest",
		Subject:  "Invoice #{{.InvoiceNo}} requires your approval",
		Pretext:  "Finance pretexting — employee approves a fake invoice via a login page.",
		HTMLBody: `<p>Hi {{.Name}},</p><p>Finance has submitted invoice <strong>#{{.InvoiceNo}}</strong> for your approval. Please review and approve it by end of day.</p><p><a href="{{.Link}}">Review Invoice</a></p><p>Accounts Payable</p>`,
		TextBody: "Hi {{.Name}},\n\nPlease review invoice #{{.InvoiceNo}}: {{.Link}}\n\nAccounts Payable",
	},
	{
		Name: "DocuSign Document", Kind: "phishing", Technique: "credential_harvest",
		Subject:  "{{.Name}}, please sign the attached document",
		Pretext:  "Fake DocuSign notification. Target clicks 'Review Document' and lands on a cred page.",
		HTMLBody: `<p>{{.Name}},</p><p>Someone has sent you a document to review and sign using DocuSign.</p><p><a href="{{.Link}}">REVIEW DOCUMENT</a></p><p>Do not share this link. It expires in 5 days.</p>`,
		TextBody: "{{.Name}}, please sign the document: {{.Link}}",
		SpoofDomain: "docusign.com",
	},
	{
		Name: "IT Helpdesk Ticket Update", Kind: "phishing", Technique: "credential_harvest",
		Subject:  "Your support ticket #{{.TicketNo}} has been updated",
		Pretext:  "Helpdesk ticket notification with a link back to a fake portal login.",
		HTMLBody: `<p>Hi {{.Name}},</p><p>Your IT support ticket <strong>#{{.TicketNo}}</strong> has been updated and requires your feedback.</p><p><a href="{{.Link}}">View Ticket</a></p><p>IT Helpdesk</p>`,
		TextBody: "Hi {{.Name}}, your ticket #{{.TicketNo}} has been updated: {{.Link}}\n\nIT Helpdesk",
	},
	{
		Name: "Shared File — Microsoft OneDrive", Kind: "phishing", Technique: "credential_harvest",
		Subject:  "{{.FirstName}}, {{.Company}} shared a file with you",
		Pretext:  "File-share lure impersonating Microsoft OneDrive. Redirects to M365 login clone.",
		HTMLBody: `<p>{{.FirstName}},</p><p>Someone at {{.Company}} has shared a document with you on OneDrive.</p><p><a href="{{.Link}}">Open in OneDrive</a></p><p>This link expires in 7 days.</p>`,
		TextBody: "{{.FirstName}}, a file has been shared with you: {{.Link}}",
		SpoofDomain: "onedrive.com",
	},
	{
		Name: "New VPN / Remote Access Setup", Kind: "phishing", Technique: "credential_harvest",
		Subject:  "ACTION REQUIRED: Set up your new VPN access",
		Pretext:  "IT migration pretext. Targets remote workers; harvests VPN credentials.",
		HTMLBody: `<p>Dear {{.Name}},</p><p>As part of our infrastructure upgrade, all employees must re-enroll in the new remote access portal by <strong>Friday</strong>.</p><p><a href="{{.Link}}">Set Up Remote Access</a></p><p>Failure to enroll will result in loss of VPN access.</p><p>IT Operations</p>`,
		TextBody: "Dear {{.Name}},\n\nPlease re-enroll in the new VPN portal: {{.Link}}\n\nIT Operations",
	},
	{
		Name: "Payroll Direct Deposit Change", Kind: "phishing", Technique: "bec",
		Subject:  "Update your direct deposit information",
		Pretext:  "BEC / payroll diversion. HR pretext requesting bank account update.",
		HTMLBody: `<p>Hi {{.Name}},</p><p>HR is updating our payroll system. Please verify and update your direct deposit information before the next pay cycle.</p><p><a href="{{.Link}}">Update Banking Details</a></p><p>Human Resources</p>`,
		TextBody: "Hi {{.Name}},\n\nPlease update your payroll bank details: {{.Link}}\n\nHR",
	},
	{
		Name: "CEO Wire Transfer Request", Kind: "phishing", Technique: "bec",
		Subject:  "Urgent: Confidential wire transfer needed",
		Pretext:  "Classic BEC CEO fraud. Impersonates CEO/CFO to request urgent fund transfer.",
		HTMLBody: `<p>Hi {{.Name}},</p><p>I need you to process an urgent wire transfer of $47,500 to a new vendor. This is time-sensitive and must be completed today. I am in a meeting and cannot be reached by phone.</p><p>Please confirm you can handle this, and I will send you the details.</p><p>Thanks,<br>{{.Company}} CEO</p>`,
		TextBody: "Hi {{.Name}},\n\nPlease process an urgent wire transfer. Reply to confirm and I'll send details.\n\n{{.Company}} CEO",
	},
	{
		Name: "Fake LinkedIn Connection", Kind: "phishing", Technique: "spear_phishing",
		Subject:  "You have a new LinkedIn connection request",
		Pretext:  "Spear-phishing via fake LinkedIn notification. Harvests credentials on clone page.",
		HTMLBody: `<p>{{.Name}},</p><p>A recruiter at a leading cybersecurity firm wants to connect with you on LinkedIn.</p><p><a href="{{.Link}}">Accept Invitation</a></p><p>LinkedIn Team</p>`,
		TextBody: "{{.Name}}, you have a LinkedIn connection request. Accept: {{.Link}}",
		SpoofDomain: "linkedin.com",
	},
	{
		Name: "Security Alert — Unusual Sign-in", Kind: "phishing", Technique: "credential_harvest",
		Subject:  "Security alert: unusual sign-in to your account",
		Pretext:  "Fake security alert creates panic, then harvests credentials on a fake MFA page.",
		HTMLBody: `<p>Hi {{.Name}},</p><p>We detected an unusual sign-in to your account from <strong>Russia (Moscow)</strong>. If this wasn't you, please secure your account immediately.</p><p><a href="{{.Link}}">Secure My Account</a></p><p>Security Team</p>`,
		TextBody: "Hi {{.Name}},\n\nUnusual sign-in detected. Secure your account: {{.Link}}\n\nSecurity Team",
	},
	// --- Vishing --------------------------------------------------------------
	{
		Name: "IT Security Verification Call", Kind: "vishing", Technique: "pretexting",
		Subject:  "IT Security Verification Script",
		Pretext:  "Caller claims to be IT security, asks target to verify credentials or run a tool.",
		TextBody: `Script:
"Hi [Name], this is [Caller] from the IT Security Operations Centre. We're conducting a routine security audit and need to verify your account details. Can you confirm your username for me? We're seeing some unusual activity on accounts in your department and want to make sure yours is secure."

Key objectives:
- Obtain username and confirm role/department
- If possible, persuade target to run a "diagnostic tool" link
- Note any security software mentioned

If challenged: "I completely understand your caution — that's exactly the kind of awareness we train for. You can call the IT helpdesk at [number] and reference ticket [#]. They'll confirm my name is on the case."`,
	},
	{
		Name: "Vendor Invoice Call", Kind: "vishing", Technique: "pretexting",
		Subject:  "Vendor Invoice Approval Script",
		Pretext:  "Caller impersonates a known vendor to redirect payment to a fraudulent account.",
		TextBody: `Script:
"Hi [Name], this is [Caller] calling from [Vendor Name]. I'm following up on invoice #[Number] we sent last week. We've recently updated our banking details and wanted to make sure you have the new information before processing payment."

Key objectives:
- Redirect ACH/wire to attacker-controlled account
- Confirm payment amount and scheduled date
- Avoid triggering a callback to known vendor number`,
	},
	// --- Smishing -------------------------------------------------------------
	{
		Name: "HR Benefits Deadline SMS", Kind: "smishing", Technique: "credential_harvest",
		Subject:  "HR Benefits SMS",
		TextBody: "HR ALERT: Your open enrollment deadline is TOMORROW. Update your benefits now or lose coverage: {{.Link}} Reply STOP to opt out.",
	},
	{
		Name: "Package Delivery SMS", Kind: "smishing", Technique: "credential_harvest",
		Subject:  "Parcel Delivery SMS",
		TextBody: "FedEx: Your package (tracking #{{.TicketNo}}) is on hold due to an incomplete address. Confirm delivery details: {{.Link}}",
		SpoofDomain: "fedex.com",
	},
	{
		Name: "Bank Fraud Alert SMS", Kind: "smishing", Technique: "credential_harvest",
		Subject:  "Bank Fraud Alert SMS",
		TextBody: "ALERT: Your account has been temporarily suspended due to suspicious activity. Verify your identity to restore access: {{.Link}}",
	},
	// --- Pretexting -----------------------------------------------------------
	{
		Name: "New Employee Onboarding Survey", Kind: "pretexting", Technique: "information_gathering",
		Subject:  "Welcome! Complete your onboarding checklist",
		Pretext:  "Targets new employees who are less likely to question requests. Harvests org-chart info.",
		HTMLBody: `<p>Welcome to {{.Company}}, {{.FirstName}}!</p><p>Please complete your onboarding checklist at your earliest convenience. This includes your manager's contact details, your department's IT setup preferences, and access to our internal portals.</p><p><a href="{{.Link}}">Complete Onboarding Checklist</a></p><p>People Operations</p>`,
		TextBody: "Welcome {{.FirstName}}! Complete your onboarding checklist: {{.Link}}\n\nPeople Operations",
	},
}
