package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/notify"
)

// engagementRunner enforces engagement testing windows and budgets. It runs beside the Scheduler (its own goroutine,
// so a slow or failing step never delays agent triggers):
//
//   - window closed → every running task of an enforcing engagement is paused with the cause outside_testing_window;
//     window open → only the tasks this runner paused are resumed (a task a human paused stays paused). The target gate
//     independently refuses network calls while the window is closed.
//   - budget → each configured threshold raises one alert (deduplicated by an event key that includes the budget, so
//     raising the budget re-arms it); at 100 % with budget_action=pause the tasks are paused and stay paused.
type engagementRunner struct {
	s          *Server
	pg         *db.DB
	windowTick time.Duration
	budgetTick time.Duration
	lastBudget time.Time
}

const (
	engStateWindowPaused = "engagement_window_paused:" // + id → JSON array of task ids paused by the window
	engStateWindowState  = "engagement_window_state:"  // + id → "open" | "closed"
)

func newEngagementRunner(s *Server) *engagementRunner {
	return &engagementRunner{s: s, pg: s.m.pg, windowTick: 15 * time.Second, budgetTick: time.Minute}
}

func (r *engagementRunner) Run(ctx context.Context) {
	if r.pg == nil {
		return
	}
	t := time.NewTicker(r.windowTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			r.step(ctx, now)
		}
	}
}

func (r *engagementRunner) step(ctx context.Context, now time.Time) {
	list, err := r.pg.ListEngagements()
	if err != nil {
		return
	}
	doBudget := now.Sub(r.lastBudget) >= r.budgetTick
	if doBudget {
		r.lastBudget = now
	}
	for _, e := range list {
		// Keep the forced gate of the engagement's runtime tasks in sync (covers restarts and out-of-band changes).
		r.s.refreshEngagementGates(e.ID)
		if e.Status == "closed" {
			continue
		}
		if e.WindowEnforce {
			r.checkWindow(ctx, e, now)
		}
		if doBudget && e.BudgetEUR != nil && *e.BudgetEUR > 0 {
			r.checkBudget(ctx, e)
		}
	}
}

func (r *engagementRunner) pausedSet(id int64) map[int64]bool {
	out := map[int64]bool{}
	raw, _ := r.pg.GetSchedState(engStateWindowPaused + strconv.FormatInt(id, 10))
	var ids []int64
	if raw != "" && json.Unmarshal([]byte(raw), &ids) == nil {
		for _, v := range ids {
			out[v] = true
		}
	}
	return out
}

func (r *engagementRunner) savePausedSet(id int64, set map[int64]bool) {
	ids := make([]int64, 0, len(set))
	for v := range set {
		ids = append(ids, v)
	}
	b, _ := json.Marshal(ids)
	_ = r.pg.SetSchedState(engStateWindowPaused+strconv.FormatInt(id, 10), string(b))
}

func (r *engagementRunner) task(id int64) *Task {
	t, ok := r.s.m.Task(strconv.FormatInt(id, 10))
	if !ok {
		return nil
	}
	return t
}

func (r *engagementRunner) checkWindow(ctx context.Context, e *db.Engagement, now time.Time) {
	w, err := e.Window()
	if err != nil {
		return
	}
	st := w.At(now)
	ids, err := r.pg.EngagementTaskIDs(e.ID)
	if err != nil {
		return
	}
	paused := r.pausedSet(e.ID)
	changed := false

	if !st.Open {
		cause := agent.Causef("outside_testing_window", "Outside the engagement's testing window",
			"The engagement %q enforces a testing window and it is closed (%s). The task was paused automatically and resumes when the window reopens.", e.Name, st.Reason)
		for _, id := range ids {
			t := r.task(id)
			if t == nil {
				continue
			}
			lc := t.lifecycleSnapshot()
			if isTerminalStatus(lc.Status) || lc.Paused {
				continue
			}
			if _, err := r.s.applyTaskControlWithCause(t, "pause", cause); err != nil {
				log.Printf("[engagement] pause task %d for window of %q: %v", id, e.Name, err)
				continue
			}
			paused[id] = true
			changed = true
		}
	} else {
		for id := range paused {
			t := r.task(id)
			if t == nil || !t.lifecycleSnapshot().Paused {
				delete(paused, id) // gone, or a human already resumed it
				changed = true
				continue
			}
			if _, err := r.s.applyTaskControl(t, "resume"); err != nil {
				log.Printf("[engagement] resume task %d for window of %q: %v", id, e.Name, err)
				continue
			}
			delete(paused, id)
			changed = true
		}
	}
	if changed {
		r.savePausedSet(e.ID, paused)
	}

	// Record and announce the transition once.
	stateKey := engStateWindowState + strconv.FormatInt(e.ID, 10)
	want := map[bool]string{true: "open", false: "closed"}[st.Open]
	if prev, _ := r.pg.GetSchedState(stateKey); prev != want {
		_ = r.pg.SetSchedState(stateKey, want)
		if prev == "" && want == "open" {
			return // first observation of an open window is not news
		}
		detail := map[string]any{"state": want, "reason": st.Reason, "tasks_paused": len(paused)}
		if !st.NextChange.IsZero() {
			detail["next_change"] = st.NextChange.UTC().Format(time.RFC3339)
		}
		_, _ = r.pg.AddEngagementEvent(e.ID, "system", "window_"+want, detail, "")
		title := fmt.Sprintf("Engagement %q: testing window %s", e.Name, want)
		body := "Running tasks were paused until the window reopens."
		if st.Open {
			body = "Tasks paused by the window were resumed."
		} else if st.Reason != "" {
			body = st.Reason + ". " + body
		}
		r.s.sendEngagementAlert(ctx, title, body, "medium")
	}
}

func (r *engagementRunner) checkBudget(ctx context.Context, e *db.Engagement) {
	ids, err := r.pg.EngagementTaskIDs(e.ID)
	if err != nil || len(ids) == 0 {
		return
	}
	u, err := r.pg.UsageForTasks(ids)
	if err != nil {
		return
	}
	pct := u.CostEur / *e.BudgetEUR * 100
	for _, th := range e.BudgetAlertPct {
		if pct < float64(th) {
			continue
		}
		key := fmt.Sprintf("budget:%d:%.2f", th, *e.BudgetEUR)
		detail := map[string]any{"threshold_pct": th, "cost_eur": u.CostEur, "budget_eur": *e.BudgetEUR, "unpriced_tokens": u.UnpricedTokens}
		inserted, err := r.pg.AddEngagementEvent(e.ID, "system", "budget_threshold", detail, key)
		if err != nil || !inserted {
			continue
		}
		sev := "medium"
		if th >= 100 {
			sev = "high"
		}
		body := fmt.Sprintf("Spent %.2f of %.2f EUR (%.0f%%).", u.CostEur, *e.BudgetEUR, pct)
		if u.UnpricedTokens > 0 {
			body += fmt.Sprintf(" %d tokens used models without a price and are not counted.", u.UnpricedTokens)
		}
		r.s.sendEngagementAlert(ctx, fmt.Sprintf("Engagement %q: %d%% of the budget reached", e.Name, th), body, sev)
	}
	if e.BudgetAction == "pause" && pct >= 100 {
		cause := agent.Causef("engagement_budget_exhausted", "The engagement's budget is used up",
			"The engagement %q reached its budget; its tasks were paused. Raise the budget or resume them manually.", e.Name)
		n := 0
		for _, id := range ids {
			t := r.task(id)
			if t == nil {
				continue
			}
			lc := t.lifecycleSnapshot()
			if isTerminalStatus(lc.Status) || lc.Paused {
				continue
			}
			if _, err := r.s.applyTaskControlWithCause(t, "pause", cause); err == nil {
				n++
			}
		}
		if n > 0 {
			_, _ = r.pg.AddEngagementEvent(e.ID, "system", "budget_paused", map[string]any{"tasks": n}, "")
		}
	}
}

// sendEngagementAlert pushes a short notice through every enabled notification channel. Channels render a
// single-item message as "[Severity] title" plus the summary, so a notice reuses that shape instead of the
// finding event pipeline (whose events require a finding). Channel filters do not apply, the channel's rate limit does.
func (s *Server) sendEngagementAlert(ctx context.Context, title, summary, severity string) {
	if s.notifier != nil {
		s.notifier.SendNotice(ctx, title, summary, severity)
	}
}

// SendNotice delivers one notice to all enabled channels and returns how many accepted it.
func (n *Notifier) SendNotice(ctx context.Context, title, summary, severity string) int {
	if n == nil || n.pg == nil || !n.enabled() {
		return 0
	}
	chans, err := n.pg.ListNotificationChannels(ctx)
	if err != nil {
		return 0
	}
	msg := notify.Message{
		Items:   []notify.Item{{Name: title, VulnClass: "engagement", Severity: severity, Summary: strings.TrimSpace(summary)}},
		HomeURL: n.publicBaseURL(),
	}
	sent := 0
	for _, ch := range chans {
		if !ch.IsEnabled() {
			continue
		}
		channel, cfg, ok := n.adapt(ch)
		if !ok {
			continue
		}
		if n.takeTokens(ch.ID, ch.RatePerMin, 1, time.Now()) < 1 {
			log.Printf("[engagement] notice dropped for channel %d: rate limit", ch.ID)
			continue
		}
		sendCtx, cancel := context.WithTimeout(ctx, notifySendTimeout)
		if _, err := channel.Send(sendCtx, cfg, msg); err != nil {
			log.Printf("[engagement] notice to channel %d (%s) failed: %v", ch.ID, ch.Kind, err)
		} else {
			sent++
		}
		cancel()
	}
	return sent
}
