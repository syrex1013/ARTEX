package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/Autumn-27/artex/db"
)

// Social Engineering API.

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
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if err := s.m.pg.UpdateCampaignStatus(id, req.Status); err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	c, _ := s.m.pg.GetCampaign(id)
	writeJSON(w, 200, c)
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

// seTemplates returns the built-in social engineering templates.
func (s *Server) seTplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, builtinSETemplates)
}

// builtinSETemplates are ready-made pretext templates.
var builtinSETemplates = []map[string]string{
	{"name": "IT Password Reset", "kind": "phishing", "subject": "Action Required: Password Reset", "body": "Dear {{.Name}},\n\nYour account password is expiring in 24 hours. Please reset it here: {{.Link}}\n\nIT Support"},
	{"name": "Invoice Approval", "kind": "phishing", "subject": "Invoice #{{.InvoiceNo}} requires your approval", "body": "Hi {{.Name}},\n\nPlease review and approve the attached invoice: {{.Link}}\n\nFinance Team"},
	{"name": "DocuSign Request", "kind": "phishing", "subject": "You have a document to sign", "body": "{{.Name}},\n\nPlease review and sign the document: {{.Link}}\n\nDocuSign"},
	{"name": "Helpdesk Ticket", "kind": "phishing", "subject": "Your support ticket #{{.TicketNo}} has been updated", "body": "Hi {{.Name}},\n\nYour IT ticket has been updated. View the details: {{.Link}}\n\nHelpdesk"},
	{"name": "Vendor Portal", "kind": "phishing", "subject": "New vendor portal — register now", "body": "Dear {{.Name}},\n\nPlease complete your vendor registration: {{.Link}}\n\nProcurement"},
	{"name": "IT Security Audit Call", "kind": "vishing", "subject": "IT Security Audit Script", "body": "Caller intro: 'Hi, this is [Name] from IT Security. We're doing a routine audit and need to verify your credentials. Can you confirm your username for me?'"},
	{"name": "HR Benefits SMS", "kind": "smishing", "subject": "HR Benefits SMS", "body": "HR: Your open enrollment deadline is tomorrow. Update your benefits: {{.Link}}"},
}
