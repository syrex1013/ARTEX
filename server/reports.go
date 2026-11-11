package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/classify"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/report"
)

// Reports API. Routes are registered in server.go.

func (s *Server) listReports(w http.ResponseWriter, r *http.Request) {
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var engID, taskID *int64
	if v := r.URL.Query().Get("engagement_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeErr(w, 400, "invalid engagement_id")
			return
		}
		engID = &id
	}
	if v := r.URL.Query().Get("task_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeErr(w, 400, "invalid task_id")
			return
		}
		taskID = &id
	}
	reps, err := s.m.pg.ListReports(engID, taskID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, reps)
}

func (s *Server) createReport(w http.ResponseWriter, r *http.Request) {
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var in db.ReportInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	rep, err := s.m.pg.CreateReport(in)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 201, rep)
}

func (s *Server) getFormalReport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid report id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	rep, err := s.m.pg.GetReport(id)
	if err != nil {
		if errors.Is(err, db.ErrReportNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}
	writeJSON(w, 200, rep)
}

func (s *Server) updateReport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid report id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var in db.ReportInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	rep, err := s.m.pg.UpdateReport(id, in)
	if err != nil {
		if errors.Is(err, db.ErrReportNotFound) {
			writeErr(w, 404, err.Error())
		} else if errors.Is(err, db.ErrReportImmutable) {
			writeErr(w, 409, err.Error())
		} else {
			writeErr(w, 422, err.Error())
		}
		return
	}
	writeJSON(w, 200, rep)
}

func (s *Server) deleteReport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid report id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	if err := s.m.pg.DeleteReport(id); err != nil {
		if errors.Is(err, db.ErrReportNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}
	w.WriteHeader(204)
}

func (s *Server) transitionReport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid report id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var req struct {
		To    string `json:"to"`
		Actor string `json:"actor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	rep, err := s.m.pg.TransitionReport(id, req.To, req.Actor)
	if err != nil {
		if errors.Is(err, db.ErrReportNotFound) {
			writeErr(w, 404, err.Error())
		} else if errors.Is(err, db.ErrReportOpenComments) {
			writeErr(w, 409, err.Error())
		} else {
			writeErr(w, 422, err.Error())
		}
		return
	}
	// Emit engagement alert if report is linked to an engagement.
	if rep.EngagementID != nil {
		go s.sendEngagementAlert(r.Context(), fmt.Sprintf("Report %q → %s", rep.Title, rep.Status),
			fmt.Sprintf("Status changed to %s by %s", rep.Status, req.Actor), "medium")
	}
	writeJSON(w, 200, rep)
}

func (s *Server) putReportFindings(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid report id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var entries []db.ReportFinding
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	for i := range entries {
		entries[i].ReportID = id
	}
	if err := s.m.pg.SetReportFindings(id, entries); err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	got, err := s.m.pg.ListReportFindings(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, got)
}

func (s *Server) listReportComments(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid report id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	cs, err := s.m.pg.ListReportComments(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, cs)
}

func (s *Server) addReportComment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid report id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var c db.ReportComment
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	c.ReportID = id
	got, err := s.m.pg.AddReportComment(c)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 201, got)
}

func (s *Server) resolveComment(w http.ResponseWriter, r *http.Request) {
	_, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid report id")
		return
	}
	cid, ok2 := pathInt(r, "cid")
	if !ok2 {
		writeErr(w, 400, "invalid comment id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var req struct {
		Resolved bool `json:"resolved"`
	}
	req.Resolved = true
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := s.m.pg.ResolveComment(cid, req.Resolved); err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) listReportVersions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid report id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	vs, err := s.m.pg.ListReportVersions(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, vs)
}

// exportReport renders a report to the requested format and streams it.
func (s *Server) exportReport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid report id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "html"
	}

	rep, err := s.m.pg.GetReport(id)
	if err != nil {
		if errors.Is(err, db.ErrReportNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}

	doc, err := s.buildReportDocument(rep)
	if err != nil {
		writeErr(w, 500, "build document: "+err.Error())
		return
	}

	safeTitle := strings.Map(func(r rune) rune {
		if r == ' ' || r == '/' || r == '\\' {
			return '-'
		}
		return r
	}, rep.Title)

	switch format {
	case "md", "markdown":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.md"`, safeTitle))
		fmt.Fprint(w, report.RenderMarkdown(doc))
	case "html":
		html, err := report.RenderHTML(doc)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.html"`, safeTitle))
		fmt.Fprint(w, html)
	case "docx":
		data, err := report.RenderDOCX(doc)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.docx"`, safeTitle))
		w.Write(data)
	case "json":
		data, err := report.RenderJSON(doc)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.json"`, safeTitle))
		w.Write(data)
	default:
		writeErr(w, 400, "unsupported format; use md, html, docx, json")
	}
}

// buildReportDocument assembles a report.Document from DB.
func (s *Server) buildReportDocument(rep *db.Report) (report.Document, error) {
	doc := report.Document{
		Title:    rep.Title,
		Template: rep.Template,
		Branding: report.Branding{
			Client:     rep.Branding.Client,
			Author:     rep.Branding.Author,
			LogoDataURL: rep.Branding.LogoDataURL,
			Footer:     rep.Branding.Footer,
		},
		ExecSummary: rep.ExecSummary,
		GeneratedAt: time.Now().UTC(),
	}
	if s.m.pg == nil {
		return doc, nil
	}
	entries, err := s.m.pg.ListReportFindings(rep.ID)
	if err != nil {
		return doc, err
	}
	findingIDs := make([]int64, len(entries))
	for i, e := range entries {
		findingIDs[i] = e.FindingID
	}
	classMap, _ := s.m.pg.ListFindingClassifications(findingIDs)

	for _, e := range entries {
		f, err := s.m.pg.GetFinding(e.FindingID)
		if err != nil || f == nil {
			continue
		}
		fe := report.FindingEntry{
			ID:              e.FindingID,
			VulnClass:       f.VulnClass,
			Name:            f.Name,
			Severity:        f.Severity,
			Summary:         f.Summary,
			Evidence:        f.Evidence,
			Position:        e.Position,
			Included:        e.Included,
			SummaryOverride: e.SummaryOverride,
		}
		if c, ok := classMap[e.FindingID]; ok {
			fe.CVSSVector = c.CVSSVector
			fe.CVSSScore = c.CVSSScore
			fe.CVSSSeverity = c.CVSSSeverity
			fe.CWE = c.CWE
			fe.OWASP = c.OWASP
			fe.Attack = c.Attack
			fe.Remediation = c.Remediation
		}
		doc.Findings = append(doc.Findings, fe)
	}
	sort.Slice(doc.Findings, func(i, j int) bool {
		return doc.Findings[i].Position < doc.Findings[j].Position
	})
	return doc, nil
}

// getClassifyLists is defined in classification.go.

// suggestClassification returns keyword-based classification for a vuln description.
// This wraps the same classify.Suggest used by the endpoint but adds attack-path context.
func (s *Server) suggestClassificationFromText(text string) any {
	return classify.Suggest(text)
}
