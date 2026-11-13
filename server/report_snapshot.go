package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type reportSnapshot struct {
	Engagement  json.RawMessage   `json:"engagement"`
	Tasks       []json.RawMessage `json:"tasks"`
	Scope       []json.RawMessage `json:"scope"`
	Goals       []json.RawMessage `json:"goals"`
	History     []json.RawMessage `json:"history"`
	Timeline    []json.RawMessage `json:"timeline"`
	Assets      []json.RawMessage `json:"assets"`
	Findings    []snapshotFinding `json:"findings"`
	Limitations string            `json:"limitations"`
}
type snapshotFinding struct {
	ID           int64           `json:"id"`
	TaskID       int64           `json:"task_id"`
	Name         string          `json:"name"`
	Severity     string          `json:"severity"`
	Status       string          `json:"status"`
	Verification string          `json:"verification_status"`
	Note         string          `json:"verification_note"`
	Summary      string          `json:"summary"`
	Evidence     string          `json:"evidence"`
	Version      int64           `json:"evidence_version"`
	References   json.RawMessage `json:"references"`
}

var sensitiveKey = regexp.MustCompile(`(?i)^(authorization|proxy.authorization|cookie|set.cookie|password|passwd|api.?key|access.?token|refresh.?token|secret|credentials?)$`)
var sensitiveLine = regexp.MustCompile(`(?im)(authorization|proxy-authorization|cookie|set-cookie|password|passwd|api[_-]?key|access[_-]?token|refresh[_-]?token|secret)(["' \t]*[:=][ \t]*)([^\r\n]+)`)
var sensitiveURL = regexp.MustCompile(`(https?://)[^/\s:@]+:[^/\s@]+@`)
var sensitiveBearer = regexp.MustCompile(`(?i)\b(Bearer|Basic) [A-Za-z0-9._~+/=-]+`)

func redactSource(value string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	value = sensitiveLine.ReplaceAllString(value, "$1$2[REDACTED]")
	value = sensitiveURL.ReplaceAllString(value, "${1}[REDACTED]@")
	return sensitiveBearer.ReplaceAllString(value, "[REDACTED]")
}

// Redact values after decoding, preserving valid JSON and stable source IDs.
func redactJSON(raw []byte, secrets []string) ([]byte, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var walk func(any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case string:
			return redactSource(x, secrets)
		case []any:
			for i := range x {
				x[i] = walk(x[i])
			}
		case map[string]any:
			for k, v := range x {
				if sensitiveKey.MatchString(k) {
					x[k] = "[REDACTED]"
				} else {
					x[k] = walk(v)
				}
			}
		}
		return v
	}
	return json.Marshal(walk(value))
}
func snapshotHash(snapshot reportSnapshot) string {
	raw, _ := json.Marshal(snapshot)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func (s *Server) captureReportSnapshot(ctx context.Context, eid int64, ids []int64) (reportSnapshot, error) {
	out := reportSnapshot{Tasks: []json.RawMessage{}, Scope: []json.RawMessage{}, Goals: []json.RawMessage{}, Timeline: []json.RawMessage{}, History: []json.RawMessage{}, Assets: []json.RawMessage{}, Findings: []snapshotFinding{}, Limitations: "Stored records only; no independent verification. Credentials and private conversations excluded. Archived tasks may have unavailable source evidence. Coverage reflects recorded goals and failures, not proof of complete testing."}
	tx, err := s.m.pg.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var count int
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM tasks WHERE engagement_id=$1 AND id=ANY($2) AND deleted_at IS NULL`, eid, ids).Scan(&count)
	if err != nil {
		return out, err
	}
	if len(ids) == 0 || count != len(ids) {
		return out, sql.ErrNoRows
	}
	// Engagement settings may contain operational preferences. Reports receive only reporting metadata.
	err = tx.QueryRowContext(ctx, `SELECT jsonb_build_object('id',id,'name',name,'description',description,'starts_on',starts_on,'ends_on',ends_on,'status',status,'metadata',settings->'reporting_metadata') FROM engagements WHERE id=$1`, eid).Scan(&out.Engagement)
	if err != nil {
		return out, err
	}
	queries := []struct {
		query  string
		target *[]json.RawMessage
	}{
		{`SELECT jsonb_build_object('id',id,'name',name,'description',description,'goal',goal,'status',status,'created_at',created_at,'completed_at',completed_at,'archived_at',archived_at) FROM tasks WHERE id=ANY($1) ORDER BY id`, &out.Tasks},
		{`SELECT jsonb_build_object('id',id,'task_id',task_id,'kind',kind,'domain',domain,'network',net,'value',value,'source',source,'reason',reason) FROM task_scope WHERE task_id=ANY($1) ORDER BY id`, &out.Scope},
		{`SELECT jsonb_build_object('id',n.id,'task_id',t.id,'state',n.state,'payload',n.payload,'updated_at',n.updated_at) FROM exploration_nodes n JOIN tasks t ON t.exploration_id=n.exploration_id WHERE t.id=ANY($1) AND n.kind='goal' ORDER BY n.id`, &out.Goals},
		{`SELECT jsonb_build_object('id',n.id,'task_id',t.id,'kind',n.kind,'state',n.state,'created_at',n.created_at,'completed_at',n.completed_at,'blocked_reason',n.blocked_reason) FROM exploration_nodes n JOIN tasks t ON t.exploration_id=n.exploration_id WHERE t.id=ANY($1) AND n.kind='intent' ORDER BY n.id`, &out.Timeline},
		{`SELECT jsonb_build_object('id',a.id,'task_id',t.id,'kind',a.kind,'worker',a.worker,'tool',a.tool,'summary',a.summary,'is_error',a.is_error,'created_at',a.created_at) FROM activity a JOIN tasks t ON t.exploration_id=a.exploration_id WHERE t.id=ANY($1) ORDER BY a.id`, &out.History},
		{`SELECT jsonb_build_object('id',a.id,'type',a.type,'domain',a.domain,'ip',a.ip,'port',a.port,'url',a.url,'task_ids',(SELECT jsonb_agg(tid ORDER BY tid) FROM unnest(a.task_ids) tid WHERE tid=ANY($1))) FROM assets a WHERE a.task_ids && $1::bigint[] ORDER BY a.id`, &out.Assets},
	}
	for _, q := range queries {
		rows, e := tx.QueryContext(ctx, q.query, ids)
		if e != nil {
			return out, e
		}
		for rows.Next() {
			var raw json.RawMessage
			if e = rows.Scan(&raw); e != nil {
				break
			}
			*q.target = append(*q.target, raw)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return out, e
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT f.id,f.task_id,COALESCE(NULLIF(f.name,''),f.vulnclass),f.severity,f.status,f.verification_status,f.verification_note,f.summary,f.evidence,f.evidence_version,COALESCE((SELECT jsonb_agg(jsonb_build_object('binding_id',b.id,'snapshot_id',b.snapshot_id,'role',b.role,'note',b.note) ORDER BY b.id) FROM finding_traffic_bindings b WHERE b.finding_id=f.id),'[]') FROM findings f WHERE f.task_id=ANY($1) ORDER BY f.id`, ids)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var f snapshotFinding
		if err = rows.Scan(&f.ID, &f.TaskID, &f.Name, &f.Severity, &f.Status, &f.Verification, &f.Note, &f.Summary, &f.Evidence, &f.Version, &f.References); err != nil {
			break
		}
		out.Findings = append(out.Findings, f)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	secrets := []string{}
	rows, err = tx.QueryContext(ctx, `SELECT password FROM task_credentials WHERE task_id=ANY($1) AND password<>''`, ids)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var secret string
		if err = rows.Scan(&secret); err != nil {
			break
		}
		secrets = append(secrets, secret)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	raw, err = redactJSON(raw, secrets)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func parseReportTasks(raw string) ([]int64, error) {
	ids := []int64{}
	seen := map[int64]bool{}
	for _, part := range strings.Split(raw, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("Select valid task IDs")
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if len(ids) > 100 {
		return nil, errors.New("Select at most 100 tasks per report")
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}
func (s *Server) getEngagementSnapshot(w http.ResponseWriter, r *http.Request) {
	eid, ok := s.engagement(w, r, 1)
	if !ok {
		return
	}
	ids, err := parseReportTasks(r.URL.Query().Get("tasks"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	snap, err := s.captureReportSnapshot(r.Context(), eid, ids)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "Not found")
		} else {
			writeErr(w, 503, errDataSourceUnavailable)
		}
		return
	}
	writeJSON(w, 200, snap)
}
func deterministicAppendix(snap reportSnapshot) string {
	var b strings.Builder
	b.WriteString("\n\n## Evidence appendix\n\n")
	for _, f := range snap.Findings {
		fmt.Fprintf(&b, "### F-%d: %s\n\nTask: %d | Severity: %s | Disposition: %s | Verification: %s | Evidence version: %d\n\n%s\n\n%s\n\nEvidence: %s\n\nReferences: %s\n\n", f.ID, f.Name, f.TaskID, f.Severity, f.Status, f.Verification, f.Version, f.Note, f.Summary, f.Evidence, f.References)
	}
	b.WriteString("## Limitations\n\n" + snap.Limitations + "\n")
	return b.String()
}
