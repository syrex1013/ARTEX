package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/llmrec"
	"github.com/google/uuid"
)

func (s *Server) registerDiscussionRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/collab/engagements/{eid}/threads", s.listDiscussionThreads)
	m.HandleFunc("POST /api/collab/engagements/{eid}/threads", s.createDiscussionThread)
	m.HandleFunc("GET /api/collab/engagements/{eid}/threads/{tid}/messages", s.listDiscussionMessages)
	m.HandleFunc("POST /api/collab/engagements/{eid}/threads/{tid}/messages", s.sendDiscussionMessage)
	m.HandleFunc("POST /api/collab/engagements/{eid}/threads/{tid}/ask", s.askDiscussionAI)
	m.HandleFunc("POST /api/collab/engagements/{eid}/threads/{tid}/messages/{mid}/cancel", s.cancelDiscussionAI)
	m.HandleFunc("GET /api/collab/engagements/{eid}/threads/{tid}/events", s.streamDiscussionEvents)
	m.HandleFunc("GET /api/collab/engagements/{eid}/threads/{tid}/attachments", s.listDiscussionAttachments)
	m.HandleFunc("POST /api/collab/engagements/{eid}/threads/{tid}/attachments", s.uploadDiscussionAttachment)
	m.HandleFunc("GET /api/collab/engagements/{eid}/threads/{tid}/attachments/{aid}", s.downloadDiscussionAttachment)
}

type discussionThread struct {
	ID           int64  `json:"id"`
	EngagementID int64  `json:"engagement_id"`
	OwnerID      int64  `json:"owner_id"`
	Title        string `json:"title"`
	Kind         string `json:"kind"`
}

func (s *Server) discussionThread(r *http.Request, eid int64) (discussionThread, error) {
	var t discussionThread
	id, err := strconv.ParseInt(r.PathValue("tid"), 10, 64)
	if err != nil {
		return t, sql.ErrNoRows
	}
	u := requestUser(r)
	err = s.m.pg.QueryRowContext(r.Context(), `SELECT id,engagement_id,owner_id,title,kind FROM discussion_threads WHERE id=$1 AND engagement_id=$2 AND (kind<>'private_ai' OR owner_id=$3 OR $4)`, id, eid, u.ID, u.Admin).Scan(&t.ID, &t.EngagementID, &t.OwnerID, &t.Title, &t.Kind)
	return t, err
}
func (s *Server) threadAccess(w http.ResponseWriter, r *http.Request) (discussionThread, bool) {
	eid, ok := s.engagement(w, r, 1)
	if !ok {
		return discussionThread{}, false
	}
	t, err := s.discussionThread(r, eid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "Not found")
		} else {
			writeErr(w, 503, errDataSourceUnavailable)
		}
		return t, false
	}
	return t, true
}
func (s *Server) listDiscussionThreads(w http.ResponseWriter, r *http.Request) {
	eid, ok := s.engagement(w, r, 1)
	if !ok {
		return
	}
	u := requestUser(r)
	limit, offset := collabPage(r)
	s.collabJSON(w, r, `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT t.id,t.title,t.kind,t.owner_id,t.created_at,u.display_name AS owner FROM discussion_threads t JOIN local_users u ON u.id=t.owner_id WHERE engagement_id=$1 AND (kind<>'private_ai' OR owner_id=$2 OR $3) ORDER BY id DESC LIMIT $4 OFFSET $5) x`, eid, u.ID, u.Admin, limit, offset)
}
func (s *Server) createDiscussionThread(w http.ResponseWriter, r *http.Request) {
	eid, ok := s.engagement(w, r, 1)
	if !ok {
		return
	}
	var q struct{ Title, Kind string }
	if decode(r, &q) != nil || strings.TrimSpace(q.Title) == "" || len(q.Title) > 200 || q.Kind != "discussion" && q.Kind != "shared_ai" && q.Kind != "private_ai" {
		writeErr(w, 400, "Title and valid thread kind required")
		return
	}
	s.collabMutation(w, r, eid, "thread.create", q.Title, `INSERT INTO discussion_threads(engagement_id,owner_id,title,kind) VALUES($1,$2,$3,$4)`, eid, requestUser(r).ID, q.Title, q.Kind)
}

const discussionMessageProjection = `jsonb_build_object('id',m.id,'author_id',m.author_id,'author',u.display_name,'client_id',m.client_id,'kind',m.kind,'body',m.body,'state',m.state,'error',m.error,'created_at',m.created_at,'attachment_id',m.attachment_id,'model',m.model,'usage',m.usage)`

func (s *Server) listDiscussionMessages(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadAccess(w, r)
	if !ok {
		return
	}
	limit, offset := collabPage(r)
	s.collabJSON(w, r, `SELECT COALESCE(jsonb_agg(x.item ORDER BY x.id),'[]') FROM (SELECT m.id,`+discussionMessageProjection+` AS item FROM discussion_messages m JOIN local_users u ON u.id=m.author_id WHERE thread_id=$1 ORDER BY m.id DESC LIMIT $2 OFFSET $3) x`, t.ID, limit, offset)
}
func (s *Server) sendDiscussionMessage(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadAccess(w, r)
	if !ok {
		return
	}
	var q struct {
		Body         string  `json:"body"`
		ClientID     string  `json:"client_id"`
		AttachmentID *string `json:"attachment_id"`
	}
	if decode(r, &q) != nil || strings.TrimSpace(q.Body) == "" || len(q.Body) > 100000 {
		writeErr(w, 400, "Message required (maximum 100000 bytes)")
		return
	}
	if _, err := uuid.Parse(q.ClientID); err != nil {
		writeErr(w, 400, "Valid client message ID required")
		return
	}
	if q.AttachmentID != nil {
		if _, err := uuid.Parse(*q.AttachmentID); err != nil {
			writeErr(w, 400, "Invalid attachment ID")
			return
		}
	}
	tx, err := s.m.pg.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	defer tx.Rollback()
	if err = s.lockEngagement(r, tx, t.EngagementID); err != nil {
		writeErr(w, 404, "Not found")
		return
	}
	var id int64
	err = tx.QueryRow(`INSERT INTO discussion_messages(thread_id,author_id,client_id,kind,body,attachment_id) SELECT $1,$2,$3,'human',$4,$5::uuid WHERE $5::uuid IS NULL OR EXISTS(SELECT 1 FROM discussion_attachments WHERE id=$5::uuid AND thread_id=$1) ON CONFLICT(thread_id,client_id) DO UPDATE SET client_id=EXCLUDED.client_id WHERE discussion_messages.author_id=EXCLUDED.author_id AND discussion_messages.body=EXCLUDED.body AND discussion_messages.kind='human' AND discussion_messages.attachment_id IS NOT DISTINCT FROM EXCLUDED.attachment_id RETURNING id`, t.ID, requestUser(r).ID, q.ClientID, q.Body, q.AttachmentID).Scan(&id)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeErr(w, 409, "Message conflicts with an existing submission or attachment")
		return
	}
	writeJSON(w, 201, map[string]any{"id": id})
}
func (s *Server) askDiscussionAI(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadAccess(w, r)
	if !ok {
		return
	}
	if t.Kind == "discussion" {
		writeErr(w, 409, "Create an AI thread to ask AI")
		return
	}
	var q struct {
		Question string  `json:"question"`
		ClientID string  `json:"client_id"`
		Tasks    []int64 `json:"task_ids"`
		Profile  int64   `json:"profile_id"`
	}
	if decode(r, &q) != nil || strings.TrimSpace(q.Question) == "" || len(q.Question) > 100000 {
		writeErr(w, 400, "Question required (maximum 100000 bytes)")
		return
	}
	if _, err := uuid.Parse(q.ClientID); err != nil {
		writeErr(w, 400, "Valid client message ID required")
		return
	}
	p, cfg, err := s.reportProvider(q.Profile)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	strs := make([]string, len(q.Tasks))
	for i, id := range q.Tasks {
		strs[i] = strconv.FormatInt(id, 10)
	}
	ids, err := parseReportTasks(strings.Join(strs, ","))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	snap, err := s.captureReportSnapshot(r.Context(), t.EngagementID, ids)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "Not found")
		} else {
			writeErr(w, 503, errDataSourceUnavailable)
		}
		return
	}
	limits, err := s.collaborationLimits()
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	tx, err := s.m.pg.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	defer tx.Rollback()
	if err = s.lockEngagement(r, tx, t.EngagementID); err != nil {
		writeErr(w, 404, "Not found")
		return
	}
	aiClient := uuid.NewSHA1(uuid.NameSpaceOID, []byte(q.ClientID+":ai")).String()
	var mid int64
	err = tx.QueryRow(`SELECT id FROM discussion_messages WHERE thread_id=$1 AND client_id=$2 AND author_id=$3 AND EXISTS(SELECT 1 FROM discussion_messages h WHERE h.thread_id=$1 AND h.client_id=$5 AND h.kind='human' AND h.body=$4)`, t.ID, aiClient, requestUser(r).ID, q.Question, q.ClientID).Scan(&mid)
	if err == nil {
		writeJSON(w, 200, map[string]any{"id": mid})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	// Include only this thread's history. Attachment bytes never enter AI context.
	history := []string{}
	rows, err := tx.Query(`SELECT kind,body FROM (SELECT id,kind,body FROM discussion_messages WHERE thread_id=$1 AND state='completed' ORDER BY id DESC LIMIT 40) h ORDER BY id`, t.ID)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	for rows.Next() {
		var kind, body string
		if err = rows.Scan(&kind, &body); err != nil {
			break
		}
		history = append(history, kind+": "+redactSource(body, nil))
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	secrets := []string{}
	secretRows, err := tx.Query(`SELECT password FROM task_credentials WHERE task_id=ANY($1) AND password<>''`, ids)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	for secretRows.Next() {
		var secret string
		if err = secretRows.Scan(&secret); err != nil {
			break
		}
		secrets = append(secrets, secret)
	}
	if err == nil {
		err = secretRows.Err()
	}
	secretRows.Close()
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	for i := range history {
		history[i] = redactSource(history[i], secrets)
	}
	input := map[string]any{"snapshot": snap, "question": redactSource(q.Question, secrets), "history": history}
	raw, _ := json.Marshal(input)
	_, err = tx.Exec(`INSERT INTO discussion_messages(thread_id,author_id,client_id,kind,body) VALUES($1,$2,$3,'human',$4)`, t.ID, requestUser(r).ID, q.ClientID, q.Question)
	if err == nil {
		err = tx.QueryRow(`INSERT INTO discussion_messages(thread_id,author_id,client_id,kind,body,state,context_json,model) VALUES($1,$2,$3,'ai','','running',$4,$5) RETURNING id`, t.ID, requestUser(r).ID, aiClient, string(raw), cfg.Model).Scan(&mid)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeErr(w, 409, "AI turn already running or message ID already used")
		return
	}
	sessionID := requestUser(r).SessionID
	uid := requestUser(r).ID
	go func() {
		ctx, cancel := context.WithTimeout(llmrec.WithWorker(s.ctx, "discussion"), time.Duration(limits.TimeoutSeconds)*time.Second)
		defer cancel()
		go func() {
			tick := time.NewTicker(time.Second)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					var allowed bool
					err := s.m.pg.QueryRowContext(ctx, `SELECT u.enabled AND s.revoked_at IS NULL AND s.expires_at>now() AND m.state='running' AND (u.admin OR EXISTS(SELECT 1 FROM engagement_members WHERE engagement_id=$4 AND user_id=u.id)) FROM user_sessions s JOIN local_users u ON u.id=s.user_id JOIN discussion_messages m ON m.id=$3 WHERE s.id=$1 AND u.id=$2`, sessionID, uid, mid, t.EngagementID).Scan(&allowed)
					if err != nil || !allowed {
						cancel()
						return
					}
				}
			}
		}()
		// Reuse the bounded, metered, tool-free report completion path. Conversation
		// context lives in metadata; the deterministic report appendix is discarded.
		var meta map[string]any
		_ = json.Unmarshal(snap.Engagement, &meta)
		meta["conversation"] = input
		chatSnap := reportSnapshot{Engagement: discussionJSON(meta), Limitations: snap.Limitations}
		answer, usage, err := s.synthesizeSnapshot(ctx, p, cfg, chatSnap, readOnlyReportInstruction+" Answer the explicit question using authorized stored context and thread history. Do not format a full report unless asked.", limits.TokenBudget)
		if i := strings.LastIndex(answer, "\n\n## Evidence appendix"); i >= 0 {
			answer = answer[:i]
		}
		state, reason := "completed", ""
		if err != nil {
			state = "failed"
			reason = "AI generation failed; submit a new Ask AI request"
			if errors.Is(err, context.DeadlineExceeded) {
				reason = "AI generation timed out"
			}
			if errors.Is(err, context.Canceled) {
				reason = "AI generation canceled or access revoked"
			}
		}
		rawUsage, _ := json.Marshal(usage)
		if _, err = s.m.pg.Exec(`UPDATE discussion_messages SET state=$2,body=$3,error=$4,usage=$5 WHERE id=$1 AND state='running'`, mid, state, answer, reason, string(rawUsage)); err != nil {
			log.Printf("[discussion] save: %v", err)
		}
	}()
	writeJSON(w, 202, map[string]any{"id": mid, "state": "running"})
}
func discussionJSON(v any) json.RawMessage { raw, _ := json.Marshal(v); return raw }
func (s *Server) cancelDiscussionAI(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadAccess(w, r)
	if !ok {
		return
	}
	mid, err := strconv.ParseInt(r.PathValue("mid"), 10, 64)
	if err != nil {
		writeErr(w, 404, "Not found")
		return
	}
	u := requestUser(r)
	s.collabMutation(w, r, t.EngagementID, "chat.cancel", r.PathValue("mid"), `UPDATE discussion_messages SET state='canceled',error='Canceled by user' WHERE id=$1 AND thread_id=$2 AND kind='ai' AND state='running' AND (author_id=$3 OR $4)`, mid, t.ID, u.ID, u.Admin)
}
func (s *Server) streamDiscussionEvents(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadAccess(w, r)
	if !ok {
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "Streaming unavailable")
		return
	}
	cursor, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if cursor == 0 {
		cursor, _ = strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	f.Flush()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		// Recheck enabled status, session and membership before every delivery batch.
		u, err := s.sessionUser(r)
		if err != nil {
			return
		}
		var allowed bool
		if err = s.m.pg.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM discussion_threads t WHERE t.id=$1 AND t.engagement_id=$2 AND (t.kind<>'private_ai' OR t.owner_id=$3 OR $4) AND ($4 OR EXISTS(SELECT 1 FROM engagement_members WHERE engagement_id=$2 AND user_id=$3)))`, t.ID, t.EngagementID, u.ID, u.Admin).Scan(&allowed); err != nil || !allowed {
			return
		}
		rows, err := s.m.pg.QueryContext(r.Context(), `SELECT e.id,`+discussionMessageProjection+` FROM discussion_events e JOIN discussion_messages m ON m.id=e.message_id JOIN local_users u ON u.id=m.author_id WHERE e.thread_id=$1 AND e.id>$2 ORDER BY e.id LIMIT 100`, t.ID, cursor)
		if err != nil {
			return
		}
		events := []struct {
			id  int64
			raw json.RawMessage
		}{}
		for rows.Next() {
			var event struct {
				id  int64
				raw json.RawMessage
			}
			if err = rows.Scan(&event.id, &event.raw); err != nil {
				break
			}
			events = append(events, event)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return
		}
		for _, event := range events {
			if _, err = fmt.Fprintf(w, "id: %d\nevent: message\ndata: %s\n\n", event.id, event.raw); err != nil {
				return
			}
			cursor = event.id
		}
		if len(events) == 0 {
			if _, err = fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
		}
		f.Flush()
		if len(events) == 100 {
			continue
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) listDiscussionAttachments(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadAccess(w, r)
	if !ok {
		return
	}
	limit, offset := collabPage(r)
	s.collabJSON(w, r, `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT id,name,author_id,created_at,octet_length(body) AS size FROM discussion_attachments WHERE thread_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3) x`, t.ID, limit, offset)
}
func (s *Server) uploadDiscussionAttachment(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadAccess(w, r)
	if !ok {
		return
	}
	limits, err := s.collaborationLimits()
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(limits.UploadMiB)*1024*1024)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, 413, "Attachment exceeds upload limit")
		return
	}
	name := filepath.Base(strings.TrimSpace(r.URL.Query().Get("name")))
	if name == "" || name == "." || len(name) > 255 {
		writeErr(w, 400, "Filename required (maximum 255 bytes)")
		return
	}
	id := uuid.NewString()
	s.collabMutation(w, r, t.EngagementID, "attachment.upload", id, `INSERT INTO discussion_attachments(id,thread_id,author_id,name,body) VALUES($1,$2,$3,$4,$5)`, id, t.ID, requestUser(r).ID, name, body)
}
func (s *Server) downloadDiscussionAttachment(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadAccess(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("aid"))
	if err != nil {
		writeErr(w, 404, "Not found")
		return
	}
	var name string
	var body []byte
	if err = s.m.pg.QueryRowContext(r.Context(), `SELECT name,body FROM discussion_attachments WHERE id=$1 AND thread_id=$2`, id.String(), t.ID).Scan(&name, &body); err != nil {
		writeErr(w, 404, "Not found")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	_, _ = w.Write(body)
}
