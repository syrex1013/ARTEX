package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/engagement"
)

// Engagement API. See engagement_runner.go for enforcement.

// engagementNeedsGate reports whether tasks of the engagement must run with the target gate on: it is what enforces
// the testing window and the "do not test" entries.
func (s *Server) engagementNeedsGate(e *db.Engagement) bool {
	if e == nil {
		return false
	}
	if e.WindowEnforce {
		return true
	}
	has, err := s.m.pg.EngagementHasExclusions(e.ID)
	return err == nil && has
}

// refreshEngagementGateForTask recomputes one runtime task's engagement link and forced gate from the database.
func (s *Server) refreshEngagementGateForTask(t *Task) {
	if t == nil || s.m.pg == nil {
		return
	}
	id, err := strconv.ParseInt(t.ID, 10, 64)
	if err != nil {
		return
	}
	e, err := s.m.pg.EngagementForTask(id)
	if err != nil {
		return
	}
	if e == nil {
		t.engagementID.Store(0)
		t.engagementGate.Store(false)
		return
	}
	t.engagementID.Store(e.ID)
	t.engagementGate.Store(s.engagementNeedsGate(e))
}

// refreshEngagementGates re-syncs every live task of one engagement (after a window/scope change).
func (s *Server) refreshEngagementGates(engID int64) {
	ids, err := s.m.pg.EngagementTaskIDs(engID)
	if err != nil {
		return
	}
	for _, id := range ids {
		if t, ok := s.m.Task(strconv.FormatInt(id, 10)); ok {
			s.refreshEngagementGateForTask(t)
		}
	}
}

var errClear = errors.New("clear")

func engID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid engagement id")
	}
	return id, ok
}

func engErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrEngagementNotFound):
		writeErr(w, 404, err.Error())
	case errors.Is(err, db.ErrEngagementExists):
		writeErr(w, 409, err.Error())
	default:
		writeErr(w, 400, err.Error())
	}
}

func (s *Server) engList(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	list, err := pg.ListEngagements()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"engagements": list})
}

func (s *Server) engGet(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	id, ok := engID(w, r)
	if pg == nil || !ok {
		return
	}
	e, err := pg.GetEngagement(id)
	if err != nil {
		engErr(w, err)
		return
	}
	writeJSON(w, 200, e)
}

// engagementBody is the writable part of an engagement. Pointer fields: absent = keep (PATCH) / default (create).
type engagementBody struct {
	Name           *string            `json:"name"`
	CompanyID      *int64             `json:"company_id"`
	ClearCompany   bool               `json:"clear_company"`
	Status         *string            `json:"status"`
	ROE            *string            `json:"roe"`
	Contacts       *json.RawMessage   `json:"contacts"`
	WindowTZ       *string            `json:"window_tz"`
	WindowStart    *string            `json:"window_start"` // RFC3339; "" clears
	WindowEnd      *string            `json:"window_end"`
	DailyWindows   *[]engagement.Slot `json:"daily_windows"`
	WindowEnforce  *bool              `json:"window_enforce"`
	BudgetEUR      *float64           `json:"budget_eur"`
	ClearBudget    bool               `json:"clear_budget"`
	BudgetAlertPct *[]int             `json:"budget_alert_pct"`
	BudgetAction   *string            `json:"budget_action"`
}

func (b *engagementBody) apply(e *db.Engagement) error {
	if b.Name != nil {
		e.Name = *b.Name
	}
	if b.ClearCompany {
		e.CompanyID = nil
	} else if b.CompanyID != nil {
		e.CompanyID = b.CompanyID
	}
	if b.Status != nil {
		e.Status = *b.Status
	}
	if b.ROE != nil {
		e.ROE = *b.ROE
	}
	if b.Contacts != nil {
		e.Contacts = []byte(*b.Contacts)
	}
	if b.WindowTZ != nil {
		e.WindowTZ = strings.TrimSpace(*b.WindowTZ)
	}
	parse := func(v *string) (*time.Time, error) {
		if v == nil {
			return nil, nil
		}
		if strings.TrimSpace(*v) == "" {
			return nil, errClear
		}
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*v))
		if err != nil {
			return nil, errors.New("window times must be RFC3339, e.g. 2026-05-04T22:00:00Z")
		}
		return &t, nil
	}
	if t, err := parse(b.WindowStart); err == errClear {
		e.WindowStart = nil
	} else if err != nil {
		return err
	} else if t != nil {
		e.WindowStart = t
	}
	if t, err := parse(b.WindowEnd); err == errClear {
		e.WindowEnd = nil
	} else if err != nil {
		return err
	} else if t != nil {
		e.WindowEnd = t
	}
	if b.DailyWindows != nil {
		raw, _ := json.Marshal(*b.DailyWindows)
		e.DailyWindows = raw
	}
	if b.WindowEnforce != nil {
		e.WindowEnforce = *b.WindowEnforce
	}
	if b.ClearBudget {
		e.BudgetEUR = nil
	} else if b.BudgetEUR != nil {
		e.BudgetEUR = b.BudgetEUR
	}
	if b.BudgetAlertPct != nil {
		e.BudgetAlertPct = *b.BudgetAlertPct
	}
	if b.BudgetAction != nil {
		e.BudgetAction = *b.BudgetAction
	}
	return nil
}

func (s *Server) engCreate(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var body engagementBody
	if err := decode(r, &body); err != nil {
		writeErr(w, 400, "bad json: "+err.Error())
		return
	}
	e := &db.Engagement{}
	if err := body.apply(e); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	id, err := pg.SaveEngagement(e)
	if err != nil {
		engErr(w, err)
		return
	}
	_, _ = pg.AddEngagementEvent(id, "operator", "created", map[string]any{"name": e.Name}, "")
	out, _ := pg.GetEngagement(id)
	writeJSON(w, 201, out)
}

func (s *Server) engPatch(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	id, ok := engID(w, r)
	if pg == nil || !ok {
		return
	}
	e, err := pg.GetEngagement(id)
	if err != nil {
		engErr(w, err)
		return
	}
	before := *e
	var body engagementBody
	if err := decode(r, &body); err != nil {
		writeErr(w, 400, "bad json: "+err.Error())
		return
	}
	if err := body.apply(e); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if _, err := pg.SaveEngagement(e); err != nil {
		engErr(w, err)
		return
	}
	if e.Status != before.Status {
		_, _ = pg.AddEngagementEvent(id, "operator", "status_changed", map[string]any{"from": before.Status, "to": e.Status}, "")
	}
	if e.WindowEnforce != before.WindowEnforce || string(e.DailyWindows) != string(before.DailyWindows) || e.WindowTZ != before.WindowTZ ||
		!sameTime(e.WindowStart, before.WindowStart) || !sameTime(e.WindowEnd, before.WindowEnd) {
		_, _ = pg.AddEngagementEvent(id, "operator", "window_changed", map[string]any{"enforce": e.WindowEnforce}, "")
		// Re-evaluate right away instead of waiting for the next runner tick.
		_ = pg.SetSchedState(engStateWindowState+strconv.FormatInt(id, 10), "")
	}
	if e.BudgetEUR != nil && (before.BudgetEUR == nil || *before.BudgetEUR != *e.BudgetEUR) {
		_, _ = pg.AddEngagementEvent(id, "operator", "budget_changed", map[string]any{"budget_eur": *e.BudgetEUR}, "")
	}
	s.refreshEngagementGates(id)
	out, _ := pg.GetEngagement(id)
	writeJSON(w, 200, out)
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func (s *Server) engDelete(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	id, ok := engID(w, r)
	if pg == nil || !ok {
		return
	}
	ids, _ := pg.EngagementTaskIDs(id)
	if err := pg.DeleteEngagement(id); err != nil {
		engErr(w, err)
		return
	}
	for _, tid := range ids {
		if t, ok := s.m.Task(strconv.FormatInt(tid, 10)); ok {
			t.engagementID.Store(0)
			t.engagementGate.Store(false)
		}
	}
	writeJSON(w, 200, map[string]any{"deleted": id})
}

func (s *Server) engScopeList(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	id, ok := engID(w, r)
	if pg == nil || !ok {
		return
	}
	if _, err := pg.GetEngagement(id); err != nil {
		engErr(w, err)
		return
	}
	rows, err := pg.ListEngagementScope(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"scope": rows})
}

func (s *Server) engScopeAdd(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	id, ok := engID(w, r)
	if pg == nil || !ok {
		return
	}
	if _, err := pg.GetEngagement(id); err != nil {
		engErr(w, err)
		return
	}
	var req struct {
		Kind     string `json:"kind"`
		Value    string `json:"value"`
		Excluded bool   `json:"excluded"`
		Reason   string `json:"reason"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "bad json: "+err.Error())
		return
	}
	added, norm, err := pg.AddEngagementScope(id, strings.ToLower(strings.TrimSpace(req.Kind)), req.Value, req.Excluded, req.Reason)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if added {
		_, _ = pg.AddEngagementEvent(id, "operator", "scope_added", map[string]any{"kind": req.Kind, "value": norm, "excluded": req.Excluded}, "")
		s.refreshEngagementGates(id)
	}
	writeJSON(w, 200, map[string]any{"added": added, "value": norm})
}

func (s *Server) engScopeDelete(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	id, ok := engID(w, r)
	sid, ok2 := pathInt(r, "sid")
	if pg == nil || !ok || !ok2 {
		return
	}
	row, err := pg.DeleteEngagementScope(id, sid)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	_, _ = pg.AddEngagementEvent(id, "operator", "scope_removed", map[string]any{"kind": row.Kind, "value": row.Value, "excluded": row.Excluded}, "")
	s.refreshEngagementGates(id)
	writeJSON(w, 200, map[string]any{"deleted": sid})
}

func (s *Server) engAttachTask(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	id, ok := engID(w, r)
	if pg == nil || !ok {
		return
	}
	var req struct {
		TaskID json.Number `json:"task_id"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "bad json: "+err.Error())
		return
	}
	tid, err := strconv.ParseInt(string(req.TaskID), 10, 64)
	if err != nil || tid <= 0 {
		writeErr(w, 400, "task_id is required")
		return
	}
	if err := pg.SetTaskEngagement(tid, &id); err != nil {
		engErr(w, err)
		return
	}
	_, _ = pg.AddEngagementEvent(id, "operator", "task_attached", map[string]any{"task_id": tid}, "")
	if t, ok := s.m.Task(strconv.FormatInt(tid, 10)); ok {
		s.refreshEngagementGateForTask(t)
	}
	writeJSON(w, 200, map[string]any{"attached": tid})
}

func (s *Server) engDetachTask(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	id, ok := engID(w, r)
	tid, ok2 := pathInt(r, "tid")
	if pg == nil || !ok || !ok2 {
		return
	}
	if e, _ := pg.EngagementForTask(tid); e == nil || e.ID != id {
		writeErr(w, 404, "the task does not belong to this engagement")
		return
	}
	if err := pg.SetTaskEngagement(tid, nil); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	_, _ = pg.AddEngagementEvent(id, "operator", "task_detached", map[string]any{"task_id": tid}, "")
	if t, ok := s.m.Task(strconv.FormatInt(tid, 10)); ok {
		s.refreshEngagementGateForTask(t)
	}
	writeJSON(w, 200, map[string]any{"detached": tid})
}

func (s *Server) engEvents(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	id, ok := engID(w, r)
	if pg == nil || !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	evs, err := pg.ListEngagementEvents(id, limit)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"events": evs})
}

// engSummary is the dashboard of one engagement.
func (s *Server) engSummary(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	id, ok := engID(w, r)
	if pg == nil || !ok {
		return
	}
	e, err := pg.GetEngagement(id)
	if err != nil {
		engErr(w, err)
		return
	}
	ids, err := pg.EngagementTaskIDs(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	type taskRow struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Desc   string `json:"description"`
		Status string `json:"status"`
		Paused bool   `json:"paused"`
		Queued bool   `json:"queued"`
	}
	tasks := make([]taskRow, 0, len(ids))
	byStatus := map[string]int{}
	cov := struct {
		Denominator int `json:"denominator"`
		Tested      int `json:"tested"`
		Tasks       int `json:"tasks"`
	}{}
	as := s.m.Assets()
	for _, tid := range ids {
		t, err := pg.GetTask(tid)
		if err != nil || t == nil {
			continue
		}
		state := t.Status
		if t.Paused {
			state = "paused"
		} else if t.Queued {
			state = "queued"
		}
		byStatus[state]++
		tasks = append(tasks, taskRow{ID: t.ID, Name: t.Name, Desc: t.Description, Status: t.Status, Paused: t.Paused, Queued: t.Queued})
		if as != nil && t.CoverageEnabled {
			if c, err := as.TaskCoverage(t.ID, t.ExplorationID); err == nil && c.Denominator > 0 {
				cov.Denominator += c.Denominator
				cov.Tested += c.Tested
				cov.Tasks++
			}
		}
	}
	findings, _ := pg.EngagementFindingStats(ids)
	bySeverity := map[string]int{}
	for _, f := range findings {
		bySeverity[f.Severity] += f.Count
	}
	retests, _ := pg.EngagementRetestCounts(ids)
	usage, _ := pg.UsageForTasks(ids)
	events, _ := pg.ListEngagementEvents(id, 15)

	win := map[string]any{"enforced": e.WindowEnforce, "configured": false, "open": true}
	if w, err := e.Window(); err == nil {
		configured := e.WindowStart != nil || e.WindowEnd != nil || len(w.Slots) > 0
		st := w.At(time.Now())
		win["configured"] = configured
		win["open"] = st.Open
		win["reason"] = st.Reason
		if !st.NextChange.IsZero() {
			win["next_change"] = st.NextChange.UTC().Format(time.RFC3339)
		}
	} else {
		win["open"], win["reason"] = false, err.Error()
	}

	budget := map[string]any{"action": e.BudgetAction}
	if e.BudgetEUR != nil {
		budget["budget_eur"] = *e.BudgetEUR
		if *e.BudgetEUR > 0 && usage != nil {
			budget["used_pct"] = usage.CostEur / *e.BudgetEUR * 100
		}
	}
	writeJSON(w, 200, map[string]any{
		"engagement":  e,
		"tasks":       tasks,
		"task_states": byStatus,
		"findings":    map[string]any{"total": sumValues(bySeverity), "by_severity": bySeverity, "buckets": findings},
		"coverage":    cov,
		"retests":     retests,
		"usage":       usage,
		"budget":      budget,
		"window":      win,
		"events":      events,
	})
}

func sumValues(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}
