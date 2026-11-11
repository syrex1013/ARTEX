package server

import (
	"encoding/json"
	"net/http"

	"github.com/Autumn-27/artex/classify"
	"github.com/Autumn-27/artex/cvss"
	"github.com/Autumn-27/artex/db"
)

// getClassification returns the classification metadata for a finding.
func (s *Server) getClassification(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid finding id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	c, err := s.m.pg.GetFindingClassification(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, c)
}

// putClassification replaces the classification of a finding.
func (s *Server) putClassification(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid finding id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var c db.FindingClassification
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeErr(w, 400, "invalid JSON: "+err.Error())
		return
	}
	c.FindingID = id
	if err := s.m.pg.SaveFindingClassification(&c); err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	got, err := s.m.pg.GetFindingClassification(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, got)
}

// getClassifyLists returns the full CWE, OWASP and ATT&CK pick lists.
func (s *Server) getClassifyLists(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"cwe":    classify.CWE,
		"owasp":  classify.OWASP,
		"attack": classify.Attack,
	})
}

// getClassifySuggest returns keyword-based classification suggestions for a vulnerability class.
func (s *Server) getClassifySuggest(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("vulnclass")
	if q == "" {
		q = r.URL.Query().Get("q")
	}
	sugg := classify.Suggest(q)
	writeJSON(w, 200, sugg)
}

// postCVSSScore parses and scores a CVSS 3.x vector.
func (s *Server) postCVSSScore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Vector string `json:"vector"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	res, err := cvss.Score(req.Vector)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 200, res)
}
