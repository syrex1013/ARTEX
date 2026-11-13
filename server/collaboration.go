package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Autumn-27/artex/db"
	"net/http"
	"strconv"
	"strings"
)

type engagementAccess struct {
	ID      int64
	Minimum int
}
type engagementAccessKey struct{}

func roleRank(role string) int {
	switch role {
	case "manager":
		return 3
	case "operator":
		return 2
	case "viewer":
		return 1
	}
	return 0
}
func (s *Server) engagement(w http.ResponseWriter, r *http.Request, minimum int) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("eid"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 404, "Not found")
		return 0, false
	}
	u := requestUser(r)
	var role string
	err = s.m.pg.QueryRowContext(r.Context(), `SELECT COALESCE(m.role,'') FROM engagements e LEFT JOIN engagement_members m ON m.engagement_id=e.id AND m.user_id=$2 WHERE e.id=$1 AND ($3 OR m.user_id IS NOT NULL)`, id, u.ID, u.Admin).Scan(&role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "Not found")
		} else {
			writeErr(w, 503, errDataSourceUnavailable)
		}
		return 0, false
	}
	if !u.Admin && roleRank(role) < minimum {
		writeErr(w, 403, "Insufficient engagement role")
		return 0, false
	}
	*r = *r.WithContext(context.WithValue(r.Context(), engagementAccessKey{}, engagementAccess{id, minimum}))
	return id, true
}
func (s *Server) collabJSON(w http.ResponseWriter, r *http.Request, query string, args ...any) {
	var raw json.RawMessage
	if err := s.m.pg.QueryRowContext(r.Context(), query, args...).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "Not found")
		} else {
			writeErr(w, 503, errDataSourceUnavailable)
		}
		return
	}
	writeJSON(w, 200, raw)
}

// Mutations and their audit record commit together. Engagement row locks also
// serialize membership removal with permission-sensitive mutations.
func (s *Server) collabMutation(w http.ResponseWriter, r *http.Request, eid int64, action, subject, query string, args ...any) {
	tx, err := s.m.pg.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	defer tx.Rollback()
	if eid > 0 {
		if err = s.lockEngagement(r, tx, eid); err != nil {
			writeErr(w, 404, "Not found")
			return
		}
	}
	result, err := tx.ExecContext(r.Context(), query, args...)
	if err != nil {
		writeErr(w, 409, "Change rejected: invalid reference or conflicting state")
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if count == 0 {
		writeErr(w, 404, "Not found")
		return
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO collaboration_audit(actor_id,engagement_id,action,subject) VALUES($1,NULLIF($2,0),$3,$4)`, requestUser(r).ID, eid, action, subject)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) lockEngagement(r *http.Request, tx *sql.Tx, eid int64) error {
	var id int64
	if err := tx.QueryRowContext(r.Context(), `SELECT id FROM engagements WHERE id=$1 FOR UPDATE`, eid).Scan(&id); err != nil {
		return err
	}
	u := requestUser(r)
	var enabled, admin bool
	if err := tx.QueryRowContext(r.Context(), `SELECT enabled,admin FROM local_users WHERE id=$1`, u.ID).Scan(&enabled, &admin); err != nil {
		return err
	}
	if !enabled {
		return sql.ErrNoRows
	}
	if admin {
		return nil
	}
	var role string
	if err := tx.QueryRowContext(r.Context(), `SELECT role FROM engagement_members WHERE engagement_id=$1 AND user_id=$2`, eid, u.ID).Scan(&role); err != nil {
		return err
	}
	access, _ := r.Context().Value(engagementAccessKey{}).(engagementAccess)
	if access.ID != eid || roleRank(role) < access.Minimum {
		return sql.ErrNoRows
	}
	return nil
}
func collabPage(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		page = 1000000
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return limit, (page - 1) * limit
}
func (s *Server) registerCollaborationRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/collab/engagements", s.listEngagements)
	m.HandleFunc("POST /api/collab/engagements", s.createEngagement)
	m.HandleFunc("GET /api/collab/engagements/{eid}", s.getEngagement)
	m.HandleFunc("PATCH /api/collab/engagements/{eid}", s.updateEngagement)
	m.HandleFunc("GET /api/collab/engagements/{eid}/members", s.listMembers)
	m.HandleFunc("PUT /api/collab/engagements/{eid}/members/{uid}", s.setMember)
	m.HandleFunc("DELETE /api/collab/engagements/{eid}/members/{uid}", s.setMember)
	m.HandleFunc("GET /api/collab/engagements/{eid}/audit", s.engagementAudit)
	m.HandleFunc("GET /api/collab/engagements/{eid}/tasks", s.engagementTasks)
	m.HandleFunc("PUT /api/collab/engagements/{eid}/tasks/{tid}", s.assignEngagementTask)
	m.HandleFunc("GET /api/collab/engagements/{eid}/findings", s.engagementFindings)
	m.HandleFunc("GET /api/collab/engagements/{eid}/findings/{fid}", s.getEngagementFinding)
	m.HandleFunc("PATCH /api/collab/engagements/{eid}/findings/{fid}", s.updateEngagementFinding)
	m.HandleFunc("GET /api/collab/engagements/{eid}/assets", s.engagementAssets)
	m.HandleFunc("GET /api/collab/engagements/{eid}/snapshot", s.getEngagementSnapshot)
	m.HandleFunc("GET /api/collab/engagements/{eid}/summary", s.engagementSummary)
	s.registerReportRoutes(m)
	s.registerDiscussionRoutes(m)
}
func (s *Server) listEngagements(w http.ResponseWriter, r *http.Request) {
	u := requestUser(r)
	limit, offset := collabPage(r)
	s.collabJSON(w, r, `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT e.*,CASE WHEN $2 THEN 'manager' ELSE m.role END AS role FROM engagements e LEFT JOIN engagement_members m ON m.engagement_id=e.id AND m.user_id=$1 WHERE $2 OR m.user_id IS NOT NULL ORDER BY e.id DESC LIMIT $3 OFFSET $4) x`, u.ID, u.Admin, limit, offset)
}
func (s *Server) createEngagement(w http.ResponseWriter, r *http.Request) {
	u := requestUser(r)
	if !u.Admin {
		writeErr(w, 403, "Administrator required")
		return
	}
	var q struct{ Name, Description string }
	if decode(r, &q) != nil || strings.TrimSpace(q.Name) == "" || len(q.Name) > 200 {
		writeErr(w, 400, "Engagement name required (maximum 200 bytes)")
		return
	}
	tx, err := s.m.pg.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRow(`INSERT INTO engagements(name,description) VALUES($1,$2) RETURNING id`, q.Name, q.Description).Scan(&id)
	if err == nil {
		_, err = tx.Exec(`INSERT INTO engagement_members(engagement_id,user_id,role) VALUES($1,$2,'manager')`, id, u.ID)
	}
	if err == nil {
		_, err = tx.Exec(`INSERT INTO collaboration_audit(actor_id,engagement_id,action) VALUES($1,$2,'engagement.create')`, u.ID, id)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	writeJSON(w, 201, map[string]any{"id": id})
}
func (s *Server) getEngagement(w http.ResponseWriter, r *http.Request) {
	id, ok := s.engagement(w, r, 1)
	if !ok {
		return
	}
	u := requestUser(r)
	s.collabJSON(w, r, `SELECT to_jsonb(e)||jsonb_build_object('role',CASE WHEN $3 THEN 'manager' ELSE (SELECT role FROM engagement_members WHERE engagement_id=e.id AND user_id=$2) END) FROM engagements e WHERE id=$1`, id, u.ID, u.Admin)
}
func (s *Server) updateEngagement(w http.ResponseWriter, r *http.Request) {
	id, ok := s.engagement(w, r, 3)
	if !ok {
		return
	}
	var q struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Status      string          `json:"status"`
		CompanyID   *int64          `json:"company_id"`
		StartsOn    *string         `json:"starts_on"`
		EndsOn      *string         `json:"ends_on"`
		Settings    json.RawMessage `json:"settings"`
	}
	if decode(r, &q) != nil || strings.TrimSpace(q.Name) == "" || len(q.Name) > 200 || !validEngagementSettings(q.Settings) {
		writeErr(w, 400, "Invalid engagement settings")
		return
	}
	if q.CompanyID != nil && !requestUser(r).Admin {
		writeErr(w, 403, "Company association requires administrator")
		return
	}
	s.collabMutation(w, r, id, "engagement.update", strconv.FormatInt(id, 10), `UPDATE engagements SET name=$2,description=$3,status=$4,starts_on=$5::date,ends_on=$6::date,settings=$7,company_id=COALESCE($8,company_id) WHERE id=$1`, id, q.Name, q.Description, q.Status, q.StartsOn, q.EndsOn, string(q.Settings), q.CompanyID)
}
func validEngagementSettings(raw json.RawMessage) bool {
	var p map[string]json.RawMessage
	if len(raw) > 16384 || json.Unmarshal(raw, &p) != nil || p == nil {
		return false
	}
	for key, v := range p {
		switch key {
		case "reporting_metadata":
			var x map[string]string
			if json.Unmarshal(v, &x) != nil {
				return false
			}
		case "default_report_model":
			var x int64
			if json.Unmarshal(v, &x) != nil || x < 0 {
				return false
			}
		case "notifications":
			var x bool
			if json.Unmarshal(v, &x) != nil {
				return false
			}
		case "retention_days":
			var x int
			if json.Unmarshal(v, &x) != nil || x < 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := s.engagement(w, r, 1)
	if !ok {
		return
	}
	s.collabJSON(w, r, `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT m.user_id,u.username,u.display_name,m.role,u.enabled FROM engagement_members m JOIN local_users u ON u.id=m.user_id WHERE m.engagement_id=$1 ORDER BY u.username) x`, id)
}
func (s *Server) setMember(w http.ResponseWriter, r *http.Request) {
	id, ok := s.engagement(w, r, 3)
	if !ok {
		return
	}
	uid, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil || uid <= 0 {
		writeErr(w, 404, "Not found")
		return
	}
	if uid == requestUser(r).ID {
		writeErr(w, 409, "Cannot remove or demote your own membership")
		return
	}
	var legacy bool
	if err = s.m.pg.QueryRow(`SELECT legacy FROM engagements WHERE id=$1`, id).Scan(&legacy); err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if legacy {
		writeErr(w, 403, "The General engagement is visible to administrators only")
		return
	}
	if r.Method == "DELETE" {
		s.collabMutation(w, r, id, "membership.remove", r.PathValue("uid"), `DELETE FROM engagement_members WHERE engagement_id=$1 AND user_id=$2`, id, uid)
		return
	}
	var q struct{ Role string }
	if decode(r, &q) != nil || roleRank(q.Role) == 0 {
		writeErr(w, 400, "Invalid role")
		return
	}
	s.collabMutation(w, r, id, "membership.set", fmt.Sprintf("%d:%s", uid, q.Role), `INSERT INTO engagement_members(engagement_id,user_id,role) VALUES($1,$2,$3) ON CONFLICT(engagement_id,user_id) DO UPDATE SET role=EXCLUDED.role`, id, uid, q.Role)
}
func (s *Server) engagementAudit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.engagement(w, r, 3)
	if !ok {
		return
	}
	limit, offset := collabPage(r)
	s.collabJSON(w, r, `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT a.*,u.display_name AS actor FROM collaboration_audit a LEFT JOIN local_users u ON u.id=a.actor_id WHERE engagement_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3) x`, id, limit, offset)
}
func (s *Server) engagementTasks(w http.ResponseWriter, r *http.Request) {
	id, ok := s.engagement(w, r, 1)
	if !ok {
		return
	}
	limit, offset := collabPage(r)
	q := r.URL.Query()
	const where = `engagement_id=$1 AND deleted_at IS NULL AND (name ILIKE $4 OR description ILIKE $4 OR goal ILIKE $4) AND ($5='' OR status=$5)`
	s.collabJSON(w, r, `SELECT jsonb_build_object('total',(SELECT count(*) FROM tasks WHERE `+where+`),'items',COALESCE(jsonb_agg(x),'[]')) FROM (SELECT id,name,description,goal,status,created_at,updated_at,archived_at FROM tasks WHERE `+where+` ORDER BY id DESC LIMIT $2 OFFSET $3) x`, id, limit, offset, "%"+q.Get("q")+"%", q.Get("status"))
}
func (s *Server) assignEngagementTask(w http.ResponseWriter, r *http.Request) {
	id, ok := s.engagement(w, r, 3)
	if !ok {
		return
	}
	if !requestUser(r).Admin {
		writeErr(w, 403, "Task assignment requires administrator")
		return
	}
	tid, err := strconv.ParseInt(r.PathValue("tid"), 10, 64)
	if err != nil {
		writeErr(w, 404, "Not found")
		return
	}
	// Source relations can expose another engagement through execution context.
	// Only isolated, stopped tasks may move between engagements.
	s.collabMutation(w, r, id, "task.assign", r.PathValue("tid"), `UPDATE tasks t SET engagement_id=$2 WHERE id=$1 AND deleted_at IS NULL AND status<>'running' AND NOT EXISTS(SELECT 1 FROM task_relations WHERE task_id=t.id OR source_task_id=t.id)`, tid, id)
}

func (s *Server) engagementFindings(w http.ResponseWriter, r *http.Request) {
	eid, ok := s.engagement(w, r, 1)
	if !ok {
		return
	}
	limit, offset := collabPage(r)
	q := r.URL.Query()
	const where = `t.engagement_id=$1 AND t.deleted_at IS NULL AND (f.name ILIKE $4 OR f.summary ILIKE $4) AND ($5='' OR f.severity=$5) AND ($6='' OR f.status=$6)`
	s.collabJSON(w, r, `SELECT jsonb_build_object('total',(SELECT count(*) FROM findings f JOIN tasks t ON t.id=f.task_id WHERE `+where+`),'items',COALESCE(jsonb_agg(x),'[]')) FROM (SELECT f.id,f.task_id,f.name,f.severity,f.status,f.verification_status,f.verification_note,f.summary,f.evidence_version FROM findings f JOIN tasks t ON t.id=f.task_id WHERE `+where+` ORDER BY f.id DESC LIMIT $2 OFFSET $3) x`, eid, limit, offset, "%"+q.Get("q")+"%", q.Get("severity"), q.Get("status"))
}

// Per-engagement counts for the workspace overview; private AI threads follow listDiscussionThreads visibility.
func (s *Server) engagementSummary(w http.ResponseWriter, r *http.Request) {
	eid, ok := s.engagement(w, r, 1)
	if !ok {
		return
	}
	u := requestUser(r)
	s.collabJSON(w, r, `WITH t AS (SELECT id,status FROM tasks WHERE engagement_id=$1 AND deleted_at IS NULL),
f AS (SELECT f.severity,f.status,f.verification_status FROM findings f JOIN t ON t.id=f.task_id),
rp AS (SELECT status,approved_at FROM saved_reports WHERE engagement_id=$1)
SELECT jsonb_build_object(
 'tasks',(SELECT COALESCE(jsonb_object_agg(status,n),'{}') FROM (SELECT status,count(*) n FROM t GROUP BY status) x),
 'severity',(SELECT COALESCE(jsonb_object_agg(severity,n),'{}') FROM (SELECT severity,count(*) n FROM f GROUP BY severity) x),
 'findings',(SELECT COALESCE(jsonb_object_agg(status,n),'{}') FROM (SELECT status,count(*) n FROM f GROUP BY status) x),
 'verification',(SELECT COALESCE(jsonb_object_agg(verification_status,n),'{}') FROM (SELECT verification_status,count(*) n FROM f GROUP BY verification_status) x),
 'reports',(SELECT COALESCE(jsonb_object_agg(status,n),'{}') FROM (SELECT status,count(*) n FROM rp GROUP BY status) x),
 'approved_reports',(SELECT count(*) FROM rp WHERE approved_at IS NOT NULL),
 'threads',(SELECT count(*) FROM discussion_threads WHERE engagement_id=$1 AND (kind<>'private_ai' OR owner_id=$2 OR $3)),
 'members',(SELECT count(*) FROM engagement_members WHERE engagement_id=$1))`, eid, u.ID, u.Admin)
}
func (s *Server) engagementAssets(w http.ResponseWriter, r *http.Request) {
	eid, ok := s.engagement(w, r, 1)
	if !ok {
		return
	}
	limit, offset := collabPage(r)
	s.collabJSON(w, r, `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT a.id,a.type,a.domain,a.ip,a.port,a.url FROM assets a WHERE EXISTS(SELECT 1 FROM tasks t WHERE t.engagement_id=$1 AND t.deleted_at IS NULL AND t.id=ANY(a.task_ids)) ORDER BY a.id DESC LIMIT $2 OFFSET $3) x`, eid, limit, offset)
}
func (s *Server) updateEngagementFinding(w http.ResponseWriter, r *http.Request) {
	eid, ok := s.engagement(w, r, 2)
	if !ok {
		return
	}
	fid, err := strconv.ParseInt(r.PathValue("fid"), 10, 64)
	if err != nil {
		writeErr(w, 404, "Not found")
		return
	}
	var q struct {
		Status   string `json:"status"`
		Summary  string `json:"summary"`
		Evidence string `json:"evidence"`
	}
	if decode(r, &q) != nil || !db.ValidFindingStatus(q.Status) || len(q.Summary) > 100000 || len(q.Evidence) > 1000000 {
		writeErr(w, 400, "Invalid finding evidence or disposition")
		return
	}
	s.collabMutation(w, r, eid, "finding.evidence", r.PathValue("fid"), `WITH synced AS (UPDATE exploration_nodes n SET payload=jsonb_set(jsonb_set(n.payload,'{summary}',to_jsonb($4::text)),'{evidence}',COALESCE(n.payload->'evidence','{}')||jsonb_build_object('poc',$5::text)) FROM findings f JOIN tasks t ON t.id=f.task_id WHERE n.id=f.node_id AND f.id=$1 AND t.engagement_id=$2 AND t.deleted_at IS NULL RETURNING n.id) UPDATE findings f SET status=$3,summary=$4,evidence=$5,evidence_version=evidence_version+CASE WHEN f.summary IS DISTINCT FROM $4 OR f.evidence IS DISTINCT FROM $5 THEN 1 ELSE 0 END,verification_status=CASE WHEN f.summary IS DISTINCT FROM $4 OR f.evidence IS DISTINCT FROM $5 THEN 'unverified' ELSE f.verification_status END,verification_note=CASE WHEN f.summary IS DISTINCT FROM $4 OR f.evidence IS DISTINCT FROM $5 THEN 'Evidence updated; requires review' ELSE f.verification_note END FROM tasks t WHERE f.id=$1 AND f.task_id=t.id AND t.engagement_id=$2 AND t.deleted_at IS NULL`, fid, eid, q.Status, q.Summary, q.Evidence)
}

func collabReadError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "Not found")
	} else {
		writeErr(w, 503, errDataSourceUnavailable)
	}
}

func (s *Server) getEngagementFinding(w http.ResponseWriter, r *http.Request) {
	eid, ok := s.engagement(w, r, 1)
	if !ok {
		return
	}
	fid, err := strconv.ParseInt(r.PathValue("fid"), 10, 64)
	if err != nil {
		writeErr(w, 404, "Not found")
		return
	}
	s.collabJSON(w, r, `SELECT jsonb_build_object('id',f.id,'task_id',f.task_id,'name',f.name,'severity',f.severity,'status',f.status,'summary',f.summary,'evidence',f.evidence,'evidence_version',f.evidence_version,'verification_status',f.verification_status,'verification_note',f.verification_note) FROM findings f JOIN tasks t ON t.id=f.task_id WHERE f.id=$1 AND t.engagement_id=$2 AND t.deleted_at IS NULL`, fid, eid)
}
