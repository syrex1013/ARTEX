package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/Autumn-27/artex/classify"
	"github.com/Autumn-27/artex/db"
)



// Attack Paths API.

func (s *Server) listAttackPaths(w http.ResponseWriter, r *http.Request) {
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var engID, taskID *int64
	if v := r.URL.Query().Get("engagement_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			engID = &id
		}
	}
	if v := r.URL.Query().Get("task_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			taskID = &id
		}
	}
	paths, err := s.m.pg.ListAttackPaths(engID, taskID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, paths)
}

func (s *Server) createAttackPath(w http.ResponseWriter, r *http.Request) {
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var p db.AttackPath
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	got, err := s.m.pg.CreateAttackPath(p)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 201, got)
}

func (s *Server) getAttackPath(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	p, err := s.m.pg.GetAttackPath(id)
	if err != nil {
		if errors.Is(err, db.ErrAttackPathNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) updateAttackPath(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var p db.AttackPath
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	got, err := s.m.pg.UpdateAttackPath(id, p)
	if err != nil {
		if errors.Is(err, db.ErrAttackPathNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 422, err.Error())
		}
		return
	}
	writeJSON(w, 200, got)
}

func (s *Server) deleteAttackPath(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	if err := s.m.pg.DeleteAttackPath(id); err != nil {
		if errors.Is(err, db.ErrAttackPathNotFound) {
			writeErr(w, 404, err.Error())
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}
	w.WriteHeader(204)
}

func (s *Server) putPathSteps(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if s.m.pg == nil {
		writeErr(w, 503, "database unavailable")
		return
	}
	var steps []db.PathStep
	if err := json.NewDecoder(r.Body).Decode(&steps); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if err := s.m.pg.SetPathSteps(id, steps); err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	got, err := s.m.pg.GetAttackPath(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, got)
}

// suggestAttackPath returns ATT&CK-based path suggestions for a finding or free-text.
func (s *Server) suggestAttackPath(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if findingIDStr := r.URL.Query().Get("finding_id"); findingIDStr != "" && s.m.pg != nil {
		if fid, err := strconv.ParseInt(findingIDStr, 10, 64); err == nil {
			if f, err := s.m.pg.GetFinding(fid); err == nil && f != nil {
				q = f.VulnClass + " " + f.Name
			}
		}
	}
	sugg := classify.Suggest(q)
	var steps []db.PathStep
	for i, tech := range sugg.Attack {
		steps = append(steps, db.PathStep{
			Position:  i + 1,
			Kind:      "fact",
			Technique: tech,
		})
	}
	writeJSON(w, 200, map[string]any{
		"query":   q,
		"suggest": sugg,
		"steps":   steps,
	})
}
