package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

type (
	dbLLMProfile      = db.LLMProfile
	dbEngagement      = db.Engagement
	dbTaskOpts        = db.TaskCreateOptions
	dbLLMUsage        = db.LLMUsage
	dbEngagementEvent = db.EngagementEvent
)

// TestEngagementAPIAndRunnerPG drives the engagement API through the real mux and the runner against a real task:
// CRUD, scope, attach, forced gate, window pause/resume (only window-paused tasks), summary.
func TestEngagementAPIAndRunnerPG(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer m.Close()
	td := t.TempDir()
	s := New(context.Background(), m, td, td, td)
	h := s.Handler()
	tok, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, path string, body any) (int, map[string]any) {
		var rdr *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rdr = bytes.NewReader(b)
		} else {
			rdr = bytes.NewReader(nil)
		}
		r := httptest.NewRequest(method, path, rdr)
		r.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	name := "api-engagement-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	// A daily slot that is certainly not now (the hour two hours ahead, one hour long) in UTC.
	closedSlot := func() []map[string]any {
		start := time.Now().UTC().Add(2 * time.Hour)
		end := start.Add(time.Hour)
		return []map[string]any{{"start": start.Format("15:04"), "end": end.Format("15:04")}}
	}
	code, out := do("POST", "/api/engagements", map[string]any{"name": name, "status": "testing", "window_enforce": true, "daily_windows": closedSlot(), "budget_eur": 5})
	if code != 201 {
		t.Fatalf("create = %d %v", code, out)
	}
	id := int64(out["id"].(float64))
	idStr := strconv.FormatInt(id, 10)
	t.Cleanup(func() { _, _ = do("DELETE", "/api/engagements/"+idStr, nil) })

	if code, _ := do("POST", "/api/engagements", map[string]any{"name": name}); code != 409 {
		t.Errorf("duplicate name = %d, want 409", code)
	}
	if code, _ := do("POST", "/api/engagements", map[string]any{"name": "x", "window_enforce": true}); code != 400 {
		t.Errorf("enforce without a window = %d, want 400", code)
	}
	if code, _ := do("PATCH", "/api/engagements/"+idStr, map[string]any{"status": "bogus"}); code != 400 {
		t.Errorf("bad status = %d, want 400", code)
	}
	if code, _ := do("GET", "/api/engagements/999999999", nil); code != 404 {
		t.Errorf("unknown engagement = %d, want 404", code)
	}

	// scope
	if code, out := do("POST", "/api/engagements/"+idStr+"/scope", map[string]any{"kind": "domain", "value": "Prod.Example.com", "excluded": true, "reason": "payments"}); code != 200 || out["value"] != "prod.example.com" {
		t.Fatalf("scope add = %d %v", code, out)
	}
	if code, _ := do("POST", "/api/engagements/"+idStr+"/scope", map[string]any{"kind": "domain", "value": "not a domain"}); code != 400 {
		t.Errorf("bad scope = %d, want 400", code)
	}

	// two tasks: one will be paused by the window, one is paused by a human beforehand
	mk := func() (string, *Task) {
		code, out := do("POST", "/api/tasks", map[string]any{"description": "engagement runner test", "goal": "Verify the engagement runner pauses and resumes this task [targetless]", "engagement_id": id})
		if code != 201 {
			t.Fatalf("create task = %d %v", code, out)
		}
		tid := out["id"].(string)
		task, _ := m.Task(tid)
		return tid, task
	}
	tid1, task1 := mk()
	tid2, task2 := mk()
	t.Cleanup(func() { _, _ = do("DELETE", "/api/tasks/"+tid1, nil); _, _ = do("DELETE", "/api/tasks/"+tid2, nil) })
	if task1.engagementID.Load() != id || !task1.engagementGate.Load() {
		t.Fatalf("a task of an enforcing engagement must carry the engagement and a forced gate: id=%d gate=%v", task1.engagementID.Load(), task1.engagementGate.Load())
	}
	if code, _ := do("POST", "/api/tasks/"+tid2+"/control", map[string]any{"action": "pause"}); code != 200 {
		t.Fatalf("human pause = %d", code)
	}

	r := newEngagementRunner(s)
	r.step(context.Background(), time.Now())
	if !task1.lifecycleSnapshot().Paused {
		t.Fatal("the window is closed: the running task must be paused")
	}
	set := r.pausedSet(id)
	if !set[mustInt(tid1)] || set[mustInt(tid2)] {
		t.Fatalf("only the window may be recorded as the pauser: %v", set)
	}

	// open the window (no daily slots) and run again: only the window-paused task resumes
	if code, out := do("PATCH", "/api/engagements/"+idStr, map[string]any{"daily_windows": []map[string]any{}, "window_enforce": false}); code != 200 {
		t.Fatalf("patch = %d %v", code, out)
	}
	// enforcement is off now, so re-enable with an always-open period to keep the runner interested
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	if code, out := do("PATCH", "/api/engagements/"+idStr, map[string]any{"window_start": past, "window_end": future, "window_enforce": true}); code != 200 {
		t.Fatalf("patch 2 = %d %v", code, out)
	}
	r.step(context.Background(), time.Now())
	if task1.lifecycleSnapshot().Paused {
		t.Error("the window reopened: the task it paused must be resumed")
	}
	if !task2.lifecycleSnapshot().Paused {
		t.Error("a task paused by a human must stay paused")
	}

	// summary
	code, sum := do("GET", "/api/engagements/"+idStr+"/summary", nil)
	if code != 200 {
		t.Fatalf("summary = %d", code)
	}
	if tasks := sum["tasks"].([]any); len(tasks) != 2 {
		t.Errorf("summary tasks = %d, want 2", len(tasks))
	}
	if w := sum["window"].(map[string]any); w["open"] != true || w["enforced"] != true {
		t.Errorf("window = %v", w)
	}
	if code, out := do("GET", "/api/engagements/"+idStr+"/events", nil); code != 200 || len(out["events"].([]any)) < 3 {
		t.Errorf("events = %d %v", code, out)
	}

	// detach clears the forced gate
	if code, _ := do("DELETE", "/api/engagements/"+idStr+"/tasks/"+tid1, nil); code != 200 {
		t.Fatalf("detach = %d", code)
	}
	if task1.engagementID.Load() != 0 || task1.engagementGate.Load() {
		t.Error("detaching must clear the engagement link and the forced gate")
	}
	_ = http.StatusOK
}

func mustInt(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// TestEngagementBudgetThresholdsOnce: each threshold raises exactly one event, a raised budget re-arms them,
// and budget_action=pause pauses the tasks at 100 %.
func TestEngagementBudgetThresholdsOnce(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer m.Close()
	td := t.TempDir()
	s := New(context.Background(), m, td, td, td)
	pg := m.pg

	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	prof := "budget-test-profile-" + suffix
	pid, err := pg.SaveProfile(&dbLLMProfile{Name: prof, Format: "anthropic", Model: "m", Streaming: true, PriceInputPerM: 10, PriceOutputPerM: 20})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.DeleteProfile(pid) })

	budget := 1.0
	eid, err := pg.SaveEngagement(&dbEngagement{Name: "budget-" + suffix, Status: "testing", BudgetEUR: &budget, BudgetAlertPct: []int{50, 100}, BudgetAction: "pause"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.DeleteEngagement(eid) })
	task, err := pg.CreateTaskWithOptions("budget task", "Check the engagement budget thresholds [targetless]", dbTaskOpts{EngagementID: &eid})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.DeleteTask(task.ID) })
	tid := strconv.FormatInt(task.ID, 10)

	// 60,000 input tokens at 10 EUR/M = 0.60 EUR → 60 %
	if err := pg.InsertLLMUsage(&dbLLMUsage{TaskID: tid, Model: "m", ProfileName: prof, InputTokens: 60000, Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	e, _ := pg.GetEngagement(eid)
	r := newEngagementRunner(s)
	r.checkBudget(context.Background(), e)
	r.checkBudget(context.Background(), e) // again: still one event
	evs, _ := pg.ListEngagementEvents(eid, 50)
	if n := countKind(evs, "budget_threshold"); n != 1 {
		t.Fatalf("at 60%% want exactly one threshold event (50%%), got %d", n)
	}

	// cross 100 %
	if err := pg.InsertLLMUsage(&dbLLMUsage{TaskID: tid, Model: "m", ProfileName: prof, InputTokens: 50000, Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	r.checkBudget(context.Background(), e)
	r.checkBudget(context.Background(), e)
	evs, _ = pg.ListEngagementEvents(eid, 50)
	if n := countKind(evs, "budget_threshold"); n != 2 {
		t.Fatalf("at 110%% want 2 threshold events, got %d", n)
	}
	if rt, ok := m.Task(tid); ok && !rt.lifecycleSnapshot().Paused {
		t.Error("budget_action=pause: the task must be paused at 100%")
	}

	// raising the budget re-arms the thresholds
	bigger := 2.0
	e.BudgetEUR = &bigger
	r.checkBudget(context.Background(), e) // 1.10 EUR of 2.00 = 55 %: the 50 % key for the new budget fires
	evs, _ = pg.ListEngagementEvents(eid, 50)
	if n := countKind(evs, "budget_threshold"); n != 3 {
		t.Fatalf("a new budget re-arms: want 3 events, got %d", n)
	}
}

func countKind(evs []dbEngagementEvent, kind string) int {
	n := 0
	for _, e := range evs {
		if e.Kind == kind {
			n++
		}
	}
	return n
}
