package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/engagement"
	"github.com/jackc/pgx/v5/pgconn"
)

// Engagements: a client engagement groups tasks under one scope / rules of engagement, an optional testing
// window and a budget. See engagement/ for the pure window logic.

var (
	ErrEngagementNotFound = errors.New("engagement not found")
	ErrEngagementExists   = errors.New("an engagement with this name already exists")
)

// EngagementStatuses is the lifecycle, in order.
var EngagementStatuses = []string{"scoping", "testing", "reporting", "retest", "closed"}

func ValidEngagementStatus(s string) bool {
	for _, v := range EngagementStatuses {
		if v == s {
			return true
		}
	}
	return false
}

type Engagement struct {
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	CompanyID      *int64          `json:"company_id,omitempty"`
	CompanyName    string          `json:"company_name,omitempty"`
	Status         string          `json:"status"`
	ROE            string          `json:"roe"`
	Contacts       json.RawMessage `json:"contacts"`
	WindowTZ       string          `json:"window_tz"`
	WindowStart    *time.Time      `json:"window_start,omitempty"`
	WindowEnd      *time.Time      `json:"window_end,omitempty"`
	DailyWindows   json.RawMessage `json:"daily_windows"`
	WindowEnforce  bool            `json:"window_enforce"`
	BudgetEUR      *float64        `json:"budget_eur,omitempty"`
	BudgetAlertPct []int           `json:"budget_alert_pct"`
	BudgetAction   string          `json:"budget_action"`
	TaskCount      int             `json:"task_count"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// Window converts the stored window columns into the pure engagement.Window.
func (e *Engagement) Window() (engagement.Window, error) {
	w := engagement.Window{TZ: e.WindowTZ, Start: e.WindowStart, End: e.WindowEnd}
	if len(e.DailyWindows) > 0 {
		if err := json.Unmarshal(e.DailyWindows, &w.Slots); err != nil {
			return w, fmt.Errorf("daily_windows is not valid: %w", err)
		}
	}
	return w, nil
}

// Validate checks the writable fields.
func (e *Engagement) Validate() error {
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" {
		return errors.New("engagement name is required")
	}
	if len([]rune(e.Name)) > 200 {
		return errors.New("engagement name is too long (max 200 characters)")
	}
	if e.Status == "" {
		e.Status = "scoping"
	}
	if !ValidEngagementStatus(e.Status) {
		return fmt.Errorf("invalid status %q (available: %s)", e.Status, strings.Join(EngagementStatuses, ", "))
	}
	if e.BudgetAction == "" {
		e.BudgetAction = "alert"
	}
	if e.BudgetAction != "alert" && e.BudgetAction != "pause" {
		return errors.New("budget_action must be alert or pause")
	}
	if e.BudgetEUR != nil && *e.BudgetEUR < 0 {
		return errors.New("budget must not be negative")
	}
	if e.BudgetAlertPct == nil {
		e.BudgetAlertPct = []int{80, 100}
	}
	for _, p := range e.BudgetAlertPct {
		if p < 1 || p > 1000 {
			return errors.New("budget alert thresholds must be percentages between 1 and 1000")
		}
	}
	sort.Ints(e.BudgetAlertPct)
	if len(e.Contacts) == 0 {
		e.Contacts = json.RawMessage(`[]`)
	}
	if len(e.DailyWindows) == 0 {
		e.DailyWindows = json.RawMessage(`[]`)
	}
	w, err := e.Window()
	if err != nil {
		return err
	}
	if err := w.Validate(); err != nil {
		return err
	}
	if e.WindowEnforce && e.WindowStart == nil && e.WindowEnd == nil && len(w.Slots) == 0 {
		return errors.New("enforcement needs a window: set a start/end or at least one daily slot")
	}
	return nil
}

const engagementCols = `e.id, e.name, e.company_id, COALESCE(c.name,''), e.status, e.roe, e.contacts, e.window_tz, e.window_start, e.window_end,
e.daily_windows, e.window_enforce, e.budget_eur, to_jsonb(e.budget_alert_pct), e.budget_action, e.created_at, e.updated_at,
(SELECT COUNT(*) FROM tasks t WHERE t.engagement_id = e.id AND t.deleted_at IS NULL)`

func scanEngagement(sc interface{ Scan(...any) error }) (*Engagement, error) {
	var e Engagement
	var contacts, daily, pct []byte
	if err := sc.Scan(&e.ID, &e.Name, &e.CompanyID, &e.CompanyName, &e.Status, &e.ROE, &contacts, &e.WindowTZ, &e.WindowStart, &e.WindowEnd,
		&daily, &e.WindowEnforce, &e.BudgetEUR, &pct, &e.BudgetAction, &e.CreatedAt, &e.UpdatedAt, &e.TaskCount); err != nil {
		return nil, err
	}
	e.Contacts, e.DailyWindows = json.RawMessage(contacts), json.RawMessage(daily)
	if err := json.Unmarshal(pct, &e.BudgetAlertPct); err != nil || e.BudgetAlertPct == nil {
		e.BudgetAlertPct = []int{}
	}
	return &e, nil
}

func (d *DB) ListEngagements() ([]*Engagement, error) {
	rows, err := d.Query(`SELECT ` + engagementCols + ` FROM engagements e LEFT JOIN companies c ON c.id = e.company_id ORDER BY e.created_at DESC, e.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Engagement{}
	for rows.Next() {
		e, err := scanEngagement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *DB) GetEngagement(id int64) (*Engagement, error) {
	e, err := scanEngagement(d.QueryRow(`SELECT `+engagementCols+` FROM engagements e LEFT JOIN companies c ON c.id = e.company_id WHERE e.id=$1`, id))
	if err == sql.ErrNoRows {
		return nil, ErrEngagementNotFound
	}
	return e, err
}

func engagementNKey(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), " ")
}

func isUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

// SaveEngagement inserts (ID==0) or updates an engagement.
func (d *DB) SaveEngagement(e *Engagement) (int64, error) {
	if err := e.Validate(); err != nil {
		return 0, err
	}
	pctJSON, _ := json.Marshal(e.BudgetAlertPct)
	pct := string(pctJSON)
	if e.ID == 0 {
		var id int64
		err := d.QueryRow(`INSERT INTO engagements(name,nkey,company_id,status,roe,contacts,window_tz,window_start,window_end,daily_windows,window_enforce,budget_eur,budget_alert_pct,budget_action)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,ARRAY(SELECT jsonb_array_elements_text($13::jsonb)::int),$14) RETURNING id`,
			e.Name, engagementNKey(e.Name), e.CompanyID, e.Status, e.ROE, string(e.Contacts), e.WindowTZ, e.WindowStart, e.WindowEnd, string(e.DailyWindows),
			e.WindowEnforce, e.BudgetEUR, pct, e.BudgetAction).Scan(&id)
		if isUniqueViolation(err) {
			return 0, ErrEngagementExists
		}
		return id, err
	}
	res, err := d.Exec(`UPDATE engagements SET name=$2,nkey=$3,company_id=$4,status=$5,roe=$6,contacts=$7,window_tz=$8,window_start=$9,window_end=$10,
daily_windows=$11,window_enforce=$12,budget_eur=$13,budget_alert_pct=ARRAY(SELECT jsonb_array_elements_text($14::jsonb)::int),budget_action=$15 WHERE id=$1`,
		e.ID, e.Name, engagementNKey(e.Name), e.CompanyID, e.Status, e.ROE, string(e.Contacts), e.WindowTZ, e.WindowStart, e.WindowEnd, string(e.DailyWindows),
		e.WindowEnforce, e.BudgetEUR, pct, e.BudgetAction)
	if isUniqueViolation(err) {
		return 0, ErrEngagementExists
	}
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrEngagementNotFound
	}
	return e.ID, nil
}

// DeleteEngagement removes an engagement; its tasks stay and are detached (FK SET NULL).
func (d *DB) DeleteEngagement(id int64) error {
	res, err := d.Exec(`DELETE FROM engagements WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrEngagementNotFound
	}
	return nil
}

// ---------------------------------------------------------------- scope

type EngagementScopeRow struct {
	ID       int64     `json:"id"`
	Kind     string    `json:"kind"`
	Value    string    `json:"value"`
	Excluded bool      `json:"excluded"`
	Reason   string    `json:"reason,omitempty"`
	Created  time.Time `json:"created_at"`
}

var hostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

// NormalizeEngagementScope validates and canonicalises one scope value. Domains are kept exactly as given (a
// subdomain stays a subdomain) minus scheme, path, port and a leading "*." — unlike company scope, which folds to the
// registrable domain — because a "do not test admin.example.com" entry must not silently widen to example.com.
func NormalizeEngagementScope(kind, value string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", errors.New("value must not be empty")
	}
	switch kind {
	case "domain":
		v = strings.ToLower(v)
		if i := strings.Index(v, "://"); i >= 0 {
			v = v[i+3:]
		}
		if i := strings.IndexAny(v, "/?#"); i >= 0 {
			v = v[:i]
		}
		if h, _, err := net.SplitHostPort(v); err == nil {
			v = h
		}
		v = strings.TrimSuffix(strings.TrimPrefix(v, "*."), ".")
		if !hostRe.MatchString(v) {
			return "", fmt.Errorf("%q is not a valid domain", value)
		}
		return v, nil
	case "ip":
		ip := net.ParseIP(v)
		if ip == nil {
			return "", fmt.Errorf("%q is not a valid IP address", value)
		}
		return ip.String(), nil
	case "cidr":
		_, n, err := net.ParseCIDR(v)
		if err != nil {
			return "", fmt.Errorf("%q is not a valid CIDR range", value)
		}
		return n.String(), nil
	case "keyword":
		v = strings.ToLower(strings.Join(strings.Fields(v), " "))
		if v == "" || len([]rune(v)) > 200 {
			return "", errors.New("keyword must be 1-200 characters")
		}
		return v, nil
	}
	return "", fmt.Errorf("unsupported scope kind %q (available: domain, ip, cidr, keyword)", kind)
}

func (d *DB) ListEngagementScope(engID int64) ([]EngagementScopeRow, error) {
	rows, err := d.Query(`SELECT id, kind, value, excluded, reason, created_at FROM engagement_scope WHERE engagement_id=$1 ORDER BY excluded, kind, value`, engID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EngagementScopeRow{}
	for rows.Next() {
		var r EngagementScopeRow
		if err := rows.Scan(&r.ID, &r.Kind, &r.Value, &r.Excluded, &r.Reason, &r.Created); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddEngagementScope adds one entry; a duplicate is reported as added=false, not an error.
func (d *DB) AddEngagementScope(engID int64, kind, value string, excluded bool, reason string) (added bool, norm string, err error) {
	if excluded && kind == "keyword" {
		return false, "", errors.New("a keyword cannot be excluded: the gate matches hosts and addresses only")
	}
	norm, err = NormalizeEngagementScope(kind, value)
	if err != nil {
		return false, "", err
	}
	res, err := d.Exec(`INSERT INTO engagement_scope(engagement_id,kind,value,excluded,reason) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`,
		engID, kind, norm, excluded, strings.TrimSpace(reason))
	if err != nil {
		return false, norm, err
	}
	n, _ := res.RowsAffected()
	return n > 0, norm, nil
}

func (d *DB) DeleteEngagementScope(engID, scopeID int64) (EngagementScopeRow, error) {
	var r EngagementScopeRow
	err := d.QueryRow(`DELETE FROM engagement_scope WHERE id=$1 AND engagement_id=$2 RETURNING id, kind, value, excluded, reason, created_at`, scopeID, engID).
		Scan(&r.ID, &r.Kind, &r.Value, &r.Excluded, &r.Reason, &r.Created)
	if err == sql.ErrNoRows {
		return r, errors.New("scope entry not found")
	}
	return r, err
}

// EngagementExclusionRules returns the engagement's "do not test" entries as asset block rules, so the existing
// target gate enforces them with its normal matching. A domain blocks the host itself and every subdomain.
func (d *DB) EngagementExclusionRules(engID int64) ([]AssetInterceptRule, error) {
	rows, err := d.Query(`SELECT kind, value, reason FROM engagement_scope WHERE engagement_id=$1 AND excluded`, engID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AssetInterceptRule
	for rows.Next() {
		var kind, value, reason string
		if err := rows.Scan(&kind, &value, &reason); err != nil {
			return nil, err
		}
		note := "engagement: do not test"
		if reason != "" {
			note += " (" + reason + ")"
		}
		base := AssetInterceptRule{Enabled: true, Note: note, Builtin: true, Action: "block"}
		switch kind {
		case "domain":
			a, b := base, base
			a.Kind, a.Pattern = "exact_domain", value
			b.Kind, b.Pattern = "fuzzy_domain", "."+value
			out = append(out, a, b)
		case "ip":
			base.Kind, base.Pattern = "exact_ip", value
			out = append(out, base)
		case "cidr":
			base.Kind, base.Pattern = "cidr", value
			out = append(out, base)
		}
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- tasks

// SetTaskEngagement attaches a task to an engagement (nil detaches).
func (d *DB) SetTaskEngagement(taskID int64, engID *int64) error {
	if engID != nil {
		if _, err := d.GetEngagement(*engID); err != nil {
			return err
		}
	}
	res, err := d.Exec(`UPDATE tasks SET engagement_id=$2 WHERE id=$1 AND deleted_at IS NULL`, taskID, engID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("task not found")
	}
	return nil
}

// EngagementTaskIDs lists the live tasks of an engagement.
func (d *DB) EngagementTaskIDs(engID int64) ([]int64, error) {
	rows, err := d.Query(`SELECT id FROM tasks WHERE engagement_id=$1 AND deleted_at IS NULL ORDER BY id`, engID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// EngagementForTask returns the engagement a task belongs to, or nil.
func (d *DB) EngagementForTask(taskID int64) (*Engagement, error) {
	e, err := scanEngagement(d.QueryRow(`SELECT `+engagementCols+` FROM tasks t JOIN engagements e ON e.id = t.engagement_id
LEFT JOIN companies c ON c.id = e.company_id WHERE t.id=$1`, taskID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// TaskWindowClosed reports whether the task's engagement enforces a testing window that is closed right now.
func (d *DB) TaskWindowClosed(taskID int64, now time.Time) (closed bool, reason string, err error) {
	e, err := d.EngagementForTask(taskID)
	if err != nil || e == nil || !e.WindowEnforce {
		return false, "", err
	}
	w, err := e.Window()
	if err != nil {
		return true, "the testing window is misconfigured: " + err.Error(), nil
	}
	st := w.At(now)
	if st.Open {
		return false, "", nil
	}
	reason = st.Reason
	if !st.NextChange.IsZero() {
		reason += "; it opens again at " + st.NextChange.UTC().Format(time.RFC3339)
	}
	return true, reason, nil
}

// ---------------------------------------------------------------- events

type EngagementEvent struct {
	ID     int64           `json:"id"`
	TS     time.Time       `json:"ts"`
	Actor  string          `json:"actor"`
	Kind   string          `json:"kind"`
	Detail json.RawMessage `json:"detail"`
}

// AddEngagementEvent appends an audit event. With a non-empty dedupKey the event is written once only
// (inserted=false when it already exists).
func (d *DB) AddEngagementEvent(engID int64, actor, kind string, detail any, dedupKey string) (inserted bool, err error) {
	if actor == "" {
		actor = "system"
	}
	b, err := json.Marshal(detail)
	if err != nil || detail == nil {
		b = []byte(`{}`)
	}
	var key any
	if dedupKey != "" {
		key = dedupKey
	}
	res, err := d.Exec(`INSERT INTO engagement_events(engagement_id,actor,kind,detail,dedup_key) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`,
		engID, actor, kind, string(b), key)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (d *DB) ListEngagementEvents(engID int64, limit int) ([]EngagementEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := d.Query(`SELECT id, ts, actor, kind, detail FROM engagement_events WHERE engagement_id=$1 ORDER BY ts DESC, id DESC LIMIT $2`, engID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EngagementEvent{}
	for rows.Next() {
		var e EngagementEvent
		var detail []byte
		if err := rows.Scan(&e.ID, &e.TS, &e.Actor, &e.Kind, &detail); err != nil {
			return nil, err
		}
		e.Detail = json.RawMessage(detail)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- usage / cost

// EngagementUsage is the token spend of an engagement's tasks, priced per profile where prices exist.
type EngagementUsage struct {
	Profiles []ProfileUsage `json:"profiles"`
	// CostEur sums the priced profiles only; UnpricedTokens counts tokens of profiles without a price, so an
	// unpriced model is visible instead of looking free.
	CostEur        float64 `json:"cost_eur"`
	UnpricedTokens int     `json:"unpriced_tokens"`
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
}

// UsageForTasks aggregates llm_usage over the given task ids (live rows only; archived tasks keep their
// aggregate elsewhere and are not part of an engagement's live spend).
func (d *DB) UsageForTasks(taskIDs []int64) (*EngagementUsage, error) {
	u := &EngagementUsage{Profiles: []ProfileUsage{}}
	if len(taskIDs) == 0 {
		return u, nil
	}
	ids := make([]string, len(taskIDs))
	for i, id := range taskIDs {
		ids[i] = strconv.FormatInt(id, 10)
	}
	idsJSON, _ := json.Marshal(ids)
	rows, err := d.Query(`
SELECT COALESCE(profile_name,''), COUNT(*), COUNT(DISTINCT task_id),
       COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(cache_read),0), COALESCE(SUM(cache_write),0)
FROM llm_usage WHERE task_id IN (SELECT jsonb_array_elements_text($1::jsonb)) GROUP BY profile_name`, string(idsJSON))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p ProfileUsage
		if err := rows.Scan(&p.ProfileName, &p.Calls, &p.Tasks, &p.InputTokens, &p.OutputTokens, &p.CacheReadTokens, &p.CacheWriteTokens); err != nil {
			rows.Close()
			return nil, err
		}
		u.Profiles = append(u.Profiles, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := applyCostEur(d, u.Profiles); err != nil {
		return nil, err
	}
	for _, p := range u.Profiles {
		u.InputTokens += p.InputTokens
		u.OutputTokens += p.OutputTokens
		if p.CostEur != nil {
			u.CostEur += *p.CostEur
		} else {
			u.UnpricedTokens += p.InputTokens + p.OutputTokens
		}
	}
	sort.Slice(u.Profiles, func(i, j int) bool {
		return u.Profiles[i].InputTokens+u.Profiles[i].OutputTokens > u.Profiles[j].InputTokens+u.Profiles[j].OutputTokens
	})
	return u, nil
}

// EngagementHasExclusions reports whether the engagement has any "do not test" entry.
func (d *DB) EngagementHasExclusions(engID int64) (bool, error) {
	var has bool
	err := d.QueryRow(`SELECT EXISTS(SELECT 1 FROM engagement_scope WHERE engagement_id=$1 AND excluded)`, engID).Scan(&has)
	return has, err
}

// EngagementFindingCount is one (severity, status) bucket.
type EngagementFindingCount struct {
	Severity string `json:"severity"`
	Status   string `json:"status"`
	Count    int    `json:"count"`
}

// EngagementFindingStats counts the findings of the given tasks by severity and status.
func (d *DB) EngagementFindingStats(taskIDs []int64) ([]EngagementFindingCount, error) {
	out := []EngagementFindingCount{}
	if len(taskIDs) == 0 {
		return out, nil
	}
	idsJSON, _ := json.Marshal(taskIDs)
	rows, err := d.Query(`SELECT severity, status, COUNT(*) FROM findings
WHERE task_id IN (SELECT jsonb_array_elements_text($1::jsonb)::bigint) GROUP BY severity, status ORDER BY severity, status`, string(idsJSON))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c EngagementFindingCount
		if err := rows.Scan(&c.Severity, &c.Status, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// EngagementRetestCounts counts the latest retest verdict per finding of the given tasks
// ("" = a retest without a verdict yet, "none" = never retested).
func (d *DB) EngagementRetestCounts(taskIDs []int64) (map[string]int, error) {
	out := map[string]int{}
	if len(taskIDs) == 0 {
		return out, nil
	}
	idsJSON, _ := json.Marshal(taskIDs)
	rows, err := d.Query(`
SELECT COALESCE(r.verdict, CASE WHEN r.id IS NULL THEN 'none' ELSE '' END), COUNT(*)
FROM findings f
LEFT JOIN LATERAL (SELECT id, verdict FROM finding_retests WHERE finding_id = f.id ORDER BY id DESC LIMIT 1) r ON true
WHERE f.task_id IN (SELECT jsonb_array_elements_text($1::jsonb)::bigint)
GROUP BY 1`, string(idsJSON))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}
