package server

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/llmrec"
	"github.com/Autumn-27/norma/llm"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// This fixture deliberately uses a disposable PostgreSQL port, never the user's
// configured database. Start the container documented in docs/collaboration.md.
func collaborationFixture(t *testing.T) (*Server, string) {
	t.Helper()
	base := os.Getenv("ARTEX_COLLAB_TEST_DSN")
	if base == "" {
		base = os.Getenv("ARTEX_PG_DSN")
	}
	explicit := base != ""
	if !explicit {
		base = "postgres://postgres:collaboration-test@127.0.0.1:55432/artex_test?sslmode=disable"
	}
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = fmt.Sprintf("/collab_%d", time.Now().UnixNano())
	dsn := parsed.String()
	t.Setenv("ARTEX_PG_DSN", dsn)
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		if !explicit {
			t.Skip("No disposable PostgreSQL on localhost:55432; set ARTEX_COLLAB_TEST_DSN to run collaboration tests")
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := &Server{m: m, ctx: ctx, jwtKey: []byte("collaboration-fixture-signing-key-32"), loginLimit: newLoginLimiter(time.Now)}
	return s, dsn
}
func collaborationUser(t *testing.T, s *Server, name string, admin bool) (int64, string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct horse battery"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	err = s.m.pg.QueryRow(`INSERT INTO local_users(username,display_name,password_hash,admin) VALUES($1,$1,$2,$3) RETURNING id`, name, string(hash), admin).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.issueSession(httptest.NewRequest("POST", "/", nil), id)
	if err != nil {
		t.Fatal(err)
	}
	return id, token
}
func collaborationRequest(s *Server, token, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func collaborationEngagement(t *testing.T, s *Server, name string, uid int64, role string) int64 {
	t.Helper()
	var id int64
	if err := s.m.pg.QueryRow(`INSERT INTO engagements(name) VALUES($1) RETURNING id`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.Exec(`INSERT INTO engagement_members(engagement_id,user_id,role) VALUES($1,$2,$3)`, id, uid, role); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestCollaborationMigrationPreservesLegacy(t *testing.T) {
	s, dsn := collaborationFixture(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("legacy password"), bcrypt.MinCost)
	if err := s.m.pg.SetSetting(authPassKey, string(hash)); err != nil {
		t.Fatal(err)
	}
	task, err := s.m.pg.CreateTask("legacy task", "legacy goal", nil, 0, 300)
	if err != nil {
		t.Fatal(err)
	}
	var conv int64
	if err = s.m.pg.QueryRow(`INSERT INTO conversations(agent_key,title) VALUES('mainagent','private legacy') RETURNING id`).Scan(&conv); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		d, e := db.Open(dsn)
		if e != nil {
			t.Fatal(e)
		}
		d.Close()
	}
	var gotHash string
	var eid, owner int64
	if err = s.m.pg.QueryRow(`SELECT password_hash FROM local_users WHERE username='ARTEX'`).Scan(&gotHash); err != nil || gotHash != string(hash) {
		t.Fatalf("hash preserved=%v err=%v", gotHash == string(hash), err)
	}
	if err = s.m.pg.QueryRow(`SELECT engagement_id FROM tasks WHERE id=$1`, task.ID).Scan(&eid); err != nil {
		t.Fatal(err)
	}
	if err = s.m.pg.QueryRow(`SELECT owner_id FROM conversations WHERE id=$1 AND engagement_id=$2`, conv, eid).Scan(&owner); err != nil || owner <= 0 {
		t.Fatalf("legacy owner=%d err=%v", owner, err)
	}
	old, _ := signJWT(s.jwtKey)
	if w := collaborationRequest(s, old, "GET", "/api/collab/engagements", ""); w.Code != 401 {
		t.Fatalf("legacy shared token status=%d", w.Code)
	}
	// Fresh named login uses the copied hash without changing it.
	w := collaborationRequest(s, "", "POST", "/api/auth/login", `{"username":"ARTEX","password":"legacy password"}`)
	if w.Code != 200 {
		t.Fatalf("login status=%d body=%s", w.Code, w.Body.String())
	}
}
func TestCollaborationAccessAndImmediateRevocation(t *testing.T) {
	s, _ := collaborationFixture(t)
	a, at := collaborationUser(t, s, "alice", false)
	b, bt := collaborationUser(t, s, "bob", false)
	_, admin := collaborationUser(t, s, "administrator", true)
	ae := collaborationEngagement(t, s, "Alice only", a, "manager")
	be := collaborationEngagement(t, s, "Bob only", b, "manager")
	for _, path := range []string{fmt.Sprintf("/api/collab/engagements/%d", ae), fmt.Sprintf("/api/collab/engagements/%d/tasks", ae), fmt.Sprintf("/api/collab/engagements/%d/findings", ae), fmt.Sprintf("/api/collab/engagements/%d/reports", ae), fmt.Sprintf("/api/collab/engagements/%d/threads", ae), "/api/workspace/list", "/api/tasks", "/api/assets/counts", "/api/logs/stream", "/api/exploration/findings/export", "/api/settings", "/api/conversations"} {
		if w := collaborationRequest(s, bt, "GET", path, ""); w.Code != 404 {
			t.Errorf("Bob sees %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	list := collaborationRequest(s, bt, "GET", "/api/collab/engagements", "")
	if list.Code != 200 || strings.Contains(list.Body.String(), "Alice only") {
		t.Fatalf("list=%s", list.Body.String())
	}
	if w := collaborationRequest(s, at, "POST", "/api/tasks", `{}`); w.Code != 404 {
		t.Fatalf("nonadmin execution=%d", w.Code)
	}
	if w := collaborationRequest(s, admin, "DELETE", fmt.Sprintf("/api/collab/engagements/%d/members/%d", be, b), ""); w.Code != 200 {
		t.Fatalf("remove=%d %s", w.Code, w.Body.String())
	}
	if w := collaborationRequest(s, bt, "GET", fmt.Sprintf("/api/collab/engagements/%d", be), ""); w.Code != 404 {
		t.Fatalf("removed user=%d", w.Code)
	}
	if w := collaborationRequest(s, admin, "PATCH", fmt.Sprintf("/api/accounts/%d", a), `{"enabled":false}`); w.Code != 200 {
		t.Fatalf("disable=%d %s", w.Code, w.Body.String())
	}
	if w := collaborationRequest(s, at, "GET", "/api/auth/me", ""); w.Code != 401 {
		t.Fatalf("disabled user=%d", w.Code)
	}
	_, ct := collaborationUser(t, s, "carol", false)
	if w := collaborationRequest(s, ct, "POST", "/api/auth/logout", ""); w.Code != 200 {
		t.Fatalf("logout=%d", w.Code)
	}
	if w := collaborationRequest(s, ct, "GET", "/api/auth/me", ""); w.Code != 401 {
		t.Fatalf("revoked session=%d", w.Code)
	}
}

type collaborationProvider struct {
	calls   atomic.Int64
	gate    chan struct{}
	entered chan struct{}
	failure bool
}

func (p *collaborationProvider) Stream(context.Context, llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		yield(llm.StreamEvent{}, fmt.Errorf("unexpected streaming call"))
	}
}
func (p *collaborationProvider) Complete(ctx context.Context, r llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	p.calls.Add(1)
	if len(r.Tools) != 0 {
		return llm.Message{}, "", llm.Usage{}, fmt.Errorf("tools exposed")
	}
	if p.entered != nil {
		select {
		case p.entered <- struct{}{}:
		default:
		}
	}
	if p.gate != nil {
		select {
		case <-p.gate:
		case <-ctx.Done():
			return llm.Message{}, "", llm.Usage{}, ctx.Err()
		}
	}
	if p.failure {
		return llm.Message{}, "", llm.Usage{}, fmt.Errorf("provider failure containing secret")
	}
	return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock("## Executive summary\n\nStored evidence reviewed. See source IDs and limitations.")}}, "end_turn", llm.Usage{InputTokens: 100, OutputTokens: 30}, nil
}
func TestCollaborationReportsCompleteScopedImmutable(t *testing.T) {
	s, _ := collaborationFixture(t)
	uid, token := collaborationUser(t, s, "operator", false)
	viewer, vt := collaborationUser(t, s, "viewer", false)
	manager, mt := collaborationUser(t, s, "manager", false)
	eid := collaborationEngagement(t, s, "Report fixture", uid, "operator")
	for _, m := range []struct {
		id   int64
		role string
	}{{viewer, "viewer"}, {manager, "manager"}} {
		if _, err := s.m.pg.Exec(`INSERT INTO engagement_members VALUES($1,$2,$3)`, eid, m.id, m.role); err != nil {
			t.Fatal(err)
		}
	}
	task, err := s.m.pg.CreateTaskWithOptions("Scoped task", "Review evidence", db.TaskCreateOptions{EngagementID: eid})
	if err != nil {
		t.Fatal(err)
	}
	outside, err := s.m.pg.CreateTask("Unrelated task", "Not in scope", nil, 0, 300)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.m.pg.Exec(`INSERT INTO findings(task_id,vulnclass,name,severity,summary,evidence,status,verification_status) SELECT $1,'fixture','Finding '||g,'high','Recorded summary','Authorization: Bearer credential-must-not-leak','pending','unverified' FROM generate_series(1,1205) g`, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.m.pg.AddFinding(outside.ID, 0, "outside", "UNRELATED-FINDING", "critical", "private", "private", "fixture", nil); err != nil {
		t.Fatal(err)
	}
	snap, err := s.captureReportSnapshot(context.Background(), eid, []int64{task.ID})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(snap)
	if len(snap.Findings) != 1205 || strings.Contains(string(raw), "credential-must-not-leak") || strings.Contains(string(raw), "UNRELATED-FINDING") {
		t.Fatalf("snapshot findings=%d or data leak", len(snap.Findings))
	}
	if _, err = s.captureReportSnapshot(context.Background(), eid, []int64{outside.ID}); err == nil {
		t.Fatal("cross-engagement task accepted")
	}
	body := fmt.Sprintf(`{"title":"Saved fixture","task_ids":[%d]}`, task.ID)
	if w := collaborationRequest(s, vt, "POST", fmt.Sprintf("/api/collab/engagements/%d/reports", eid), body); w.Code != 403 {
		t.Fatalf("viewer generation=%d", w.Code)
	}
	if w := collaborationRequest(s, token, "POST", fmt.Sprintf("/api/collab/engagements/%d/reports", eid), body); w.Code != 422 {
		t.Fatalf("missing model=%d", w.Code)
	}
	provider := &collaborationProvider{}
	s.llmDirect = llmrec.Wrap(provider, s.m.pg, "fixture", "fixture", "", "", func() bool { return false })
	s.llmOn = true
	s.llmCfg = agent.Config{Model: "fixture", ContextWindowK: 32, MaxTokens: 1000}
	w := collaborationRequest(s, token, "POST", fmt.Sprintf("/api/collab/engagements/%d/reports", eid), body)
	if w.Code != 202 {
		t.Fatalf("generate=%d %s", w.Code, w.Body.String())
	}
	var job struct{ ID int64 }
	_ = json.Unmarshal(w.Body.Bytes(), &job)
	if _, err = s.m.pg.Exec(`UPDATE saved_reports SET status='running' WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	s.runReportJob(job.ID, collaborationLimits{TimeoutSeconds: 30, TokenBudget: 1000000})
	var state, output string
	if err = s.m.pg.QueryRow(`SELECT status,output FROM saved_reports WHERE id=$1`, job.ID).Scan(&state, &output); err != nil {
		t.Fatal(err)
	}
	if state != "completed" || strings.Count(output, "### F-") != 1205 || provider.calls.Load() < 2 {
		t.Fatalf("state=%s appendix=%d calls=%d", state, strings.Count(output, "### F-"), provider.calls.Load())
	}
	var ledgerCalls int
	if err = s.m.pg.QueryRow(`SELECT count(*) FROM llm_usage WHERE worker='reports'`).Scan(&ledgerCalls); err != nil || int64(ledgerCalls) != provider.calls.Load() {
		t.Fatalf("usage calls=%d provider=%d err=%v", ledgerCalls, provider.calls.Load(), err)
	}
	path := fmt.Sprintf("/api/collab/engagements/%d/reports/%d", eid, job.ID)
	if w = collaborationRequest(s, token, "POST", path+"/approve", ""); w.Code != 403 {
		t.Fatalf("operator approval=%d", w.Code)
	}
	if w = collaborationRequest(s, mt, "POST", path+"/approve", ""); w.Code != 200 {
		t.Fatalf("manager approval=%d %s", w.Code, w.Body.String())
	}
	if _, err = s.m.pg.Exec(`UPDATE saved_reports SET output='changed' WHERE id=$1`, job.ID); err == nil {
		t.Fatal("immutable output changed")
	}
	if w = collaborationRequest(s, vt, "GET", path+"/export?format=csv", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "APPROVED version 1") || strings.Count(w.Body.String(), "unverified") != 1205 {
		t.Fatalf("CSV status=%d", w.Code)
	}
	if _, err = s.m.pg.Exec(`UPDATE findings SET evidence_version=evidence_version+1 WHERE task_id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}
	w = collaborationRequest(s, vt, "GET", path, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"stale":true`) {
		t.Fatalf("stale=%d", w.Code)
	}
}
func TestCollaborationHumanChatPrivateAndDeduplicated(t *testing.T) {
	s, _ := collaborationFixture(t)
	a, at := collaborationUser(t, s, "owner", false)
	b, bt := collaborationUser(t, s, "member", false)
	eid := collaborationEngagement(t, s, "Discussion", a, "viewer")
	if _, err := s.m.pg.Exec(`INSERT INTO engagement_members VALUES($1,$2,'viewer')`, eid, b); err != nil {
		t.Fatal(err)
	}
	var private, shared int64
	if err := s.m.pg.QueryRow(`INSERT INTO discussion_threads(engagement_id,owner_id,title,kind) VALUES($1,$2,'Private','private_ai') RETURNING id`, eid, a).Scan(&private); err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.QueryRow(`INSERT INTO discussion_threads(engagement_id,owner_id,title,kind) VALUES($1,$2,'Shared','discussion') RETURNING id`, eid, a).Scan(&shared); err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("/api/collab/engagements/%d/threads", eid)
	for _, tail := range []string{"messages", "attachments", "events"} {
		if w := collaborationRequest(s, bt, "GET", fmt.Sprintf("%s/%d/%s", base, private, tail), ""); w.Code != 404 {
			t.Fatalf("private %s=%d", tail, w.Code)
		}
	}
	provider := &collaborationProvider{}
	s.llmDirect = provider
	s.llmOn = true
	s.llmCfg = agent.Config{Model: "fixture"}
	body := fmt.Sprintf(`{"body":"Human discussion only","client_id":%q}`, uuid.NewString())
	path := fmt.Sprintf("%s/%d/messages", base, shared)
	for i := 0; i < 2; i++ {
		if w := collaborationRequest(s, bt, "POST", path, body); w.Code != 201 {
			t.Fatalf("send=%d %s", w.Code, w.Body.String())
		}
	}
	var count int
	if err := s.m.pg.QueryRow(`SELECT count(*) FROM discussion_messages WHERE thread_id=$1`, shared).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 || provider.calls.Load() != 0 {
		t.Fatalf("messages=%d provider calls=%d", count, provider.calls.Load())
	}
	aid := uuid.NewString()
	if _, err := s.m.pg.Exec(`INSERT INTO discussion_attachments VALUES($1,$2,$3,'private.txt',$4,now())`, aid, private, a, []byte("private bytes")); err != nil {
		t.Fatal(err)
	}
	if w := collaborationRequest(s, bt, "GET", fmt.Sprintf("%s/%d/attachments/%s", base, private, aid), ""); w.Code != 404 {
		t.Fatalf("private download=%d", w.Code)
	}
	if w := collaborationRequest(s, at, "GET", fmt.Sprintf("%s/%d/attachments/%s", base, shared, aid), ""); w.Code != 404 {
		t.Fatalf("wrong thread download=%d", w.Code)
	}
}
func TestCollaborationRedactionAndPreferences(t *testing.T) {
	input := []byte(`{"id":9007199254740993,"text":"Cookie: private-cookie\nAuthorization: Bearer topsecret\npassword=also-secret","url":"https://user:pass@example.test/path","known":"stored-password"}`)
	raw, err := redactJSON(input, []string{"stored-password"})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-cookie", "topsecret", "also-secret", "user:pass", "stored-password"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("secret survived: %s", secret)
		}
	}
	if !strings.Contains(string(raw), "9007199254740993") {
		t.Fatal("source ID precision lost")
	}
	if validPreferences(json.RawMessage(`{"theme":"script"}`)) || validPreferences(json.RawMessage(`{"unknown":"x"}`)) {
		t.Fatal("invalid preferences accepted")
	}
	if csvSafe(" =HYPERLINK(1)")[0] != '\'' {
		t.Fatal("CSV formula not escaped")
	}
}

func testSessionJWT(s *Server) (string, error) {
	var id int64
	err := s.m.pg.QueryRow(`INSERT INTO local_users(username,display_name,password_hash,admin) VALUES('ARTEX','ARTEX','test-fixture-unusable-password',true) ON CONFLICT(username) DO UPDATE SET admin=true RETURNING id`).Scan(&id)
	if err != nil {
		return "", err
	}
	return s.issueSession(httptest.NewRequest("POST", "/", nil), id)
}

func TestCollaborationBrowserServer(t *testing.T) {
	if os.Getenv("ARTEX_COLLAB_BROWSER") != "1" {
		t.Skip("manual browser fixture")
	}
	s, _ := collaborationFixture(t)
	for _, u := range []struct{ name, role string }{{"manager", "manager"}, {"operator", "operator"}, {"viewer", "viewer"}} {
		uid, _ := collaborationUser(t, s, u.name, false)
		var eid int64
		err := s.m.pg.QueryRow(`SELECT id FROM engagements WHERE name='Browser acceptance'`).Scan(&eid)
		if err != nil {
			eid = collaborationEngagement(t, s, "Browser acceptance", uid, u.role)
		} else {
			_, err = s.m.pg.Exec(`INSERT INTO engagement_members VALUES($1,$2,$3)`, eid, uid, u.role)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	collaborationUser(t, s, "admin", true)
	var eid int64
	_ = s.m.pg.QueryRow(`SELECT id FROM engagements WHERE name='Browser acceptance'`).Scan(&eid)
	task, err := s.m.CreateTaskWithOptions("Acceptance task", "Review stored defensive evidence", db.TaskCreateOptions{EngagementID: eid})
	if err != nil {
		t.Fatal(err)
	}
	tid, _ := strconv.ParseInt(task.ID, 10, 64)
	_, err = s.m.pg.AddFinding(tid, 0, "configuration", "Stored finding", "medium", "Configuration review evidence", "No credentials in evidence", "fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	s.engine = NewEngine(s.m)
	handler := s.Handler()
	files := http.FileServer(http.Dir("../web/out"))
	srv := &http.Server{Addr: "127.0.0.1:18787", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			handler.ServeHTTP(w, r)
		} else {
			files.ServeHTTP(w, r)
		}
	}), ReadHeaderTimeout: 10 * time.Second}
	defer srv.Close()
	t.Log("Browser fixture: http://127.0.0.1:18787; accounts admin/manager/operator/viewer; password correct horse battery")
	if err = srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}

func TestCollaborationAIDeduplicatesAndCancels(t *testing.T) {
	s, _ := collaborationFixture(t)
	uid, token := collaborationUser(t, s, "ask-owner", false)
	eid := collaborationEngagement(t, s, "AI fixture", uid, "viewer")
	task, err := s.m.pg.CreateTaskWithOptions("AI context", "Review stored evidence", db.TaskCreateOptions{EngagementID: eid})
	if err != nil {
		t.Fatal(err)
	}
	var tid int64
	if err = s.m.pg.QueryRow(`INSERT INTO discussion_threads(engagement_id,owner_id,title,kind) VALUES($1,$2,'Ask','private_ai') RETURNING id`, eid, uid).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	provider := &collaborationProvider{gate: make(chan struct{}), entered: make(chan struct{}, 1)}
	s.llmOn = true
	s.llmDirect = provider
	s.llmCfg = agent.Config{Model: "fixture", ContextWindowK: 32, MaxTokens: 1000}
	path := fmt.Sprintf("/api/collab/engagements/%d/threads/%d", eid, tid)
	body := fmt.Sprintf(`{"question":"Authorization: Bearer private-secret","task_ids":[%d],"client_id":%q}`, task.ID, uuid.NewString())
	w := collaborationRequest(s, token, "POST", path+"/ask", body)
	if w.Code != 202 {
		t.Fatalf("ask=%d %s", w.Code, w.Body.String())
	}
	var result struct{ ID int64 }
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	select {
	case <-provider.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("provider never called")
	}
	if w = collaborationRequest(s, token, "POST", path+"/ask", body); w.Code != 200 {
		t.Fatalf("deduplicated=%d %s", w.Code, w.Body.String())
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("duplicate provider calls=%d", provider.calls.Load())
	}
	if w = collaborationRequest(s, token, "POST", fmt.Sprintf("%s/messages/%d/cancel", path, result.ID), ""); w.Code != 200 {
		t.Fatalf("cancel=%d", w.Code)
	}
	close(provider.gate)
	time.Sleep(100 * time.Millisecond)
	var state string
	if err = s.m.pg.QueryRow(`SELECT state FROM discussion_messages WHERE id=$1`, result.ID).Scan(&state); err != nil || state != "canceled" {
		t.Fatalf("state=%s err=%v", state, err)
	}
}
func TestCollaborationProviderFailureAndRestart(t *testing.T) {
	s, _ := collaborationFixture(t)
	uid, token := collaborationUser(t, s, "report-owner", false)
	eid := collaborationEngagement(t, s, "Failure fixture", uid, "operator")
	task, err := s.m.pg.CreateTaskWithOptions("Failure source", "Read only", db.TaskCreateOptions{EngagementID: eid})
	if err != nil {
		t.Fatal(err)
	}
	p := &collaborationProvider{failure: true}
	s.llmOn = true
	s.llmDirect = p
	s.llmCfg = agent.Config{Model: "fixture", ContextWindowK: 32, MaxTokens: 1000}
	path := fmt.Sprintf("/api/collab/engagements/%d/reports", eid)
	body := fmt.Sprintf(`{"title":"Failure","task_ids":[%d]}`, task.ID)
	create := func() int64 {
		w := collaborationRequest(s, token, "POST", path, body)
		if w.Code != 202 {
			t.Fatalf("generate=%d %s", w.Code, w.Body.String())
		}
		var result struct{ ID int64 }
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		return result.ID
	}
	id := create()
	if _, err = s.m.pg.Exec(`UPDATE saved_reports SET status='running' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	s.runReportJob(id, collaborationLimits{TimeoutSeconds: 10, TokenBudget: 100000})
	var status, reason string
	if err = s.m.pg.QueryRow(`SELECT status,error FROM saved_reports WHERE id=$1`, id).Scan(&status, &reason); err != nil || status != "failed" || strings.Contains(reason, "containing secret") {
		t.Fatalf("failure=%s %q %v", status, reason, err)
	}
	queued := create()
	s.initCollaboration()
	if err = s.m.pg.QueryRow(`SELECT status,error FROM saved_reports WHERE id=$1`, queued).Scan(&status, &reason); err != nil || status != "failed" || !strings.Contains(reason, "restart") {
		t.Fatalf("restart=%s %q %v", status, reason, err)
	}
	p.failure = false
	p.calls.Store(0)
	_, _, err = s.synthesizeSnapshot(context.Background(), p, s.llmCfg, reportSnapshot{Limitations: strings.Repeat("x", 100000)}, readOnlyReportInstruction, 1000)
	if err == nil || p.calls.Load() != 0 {
		t.Fatalf("oversized source reached provider: %v calls=%d", err, p.calls.Load())
	}
}
func TestCollaborationDashboardBenchmark(t *testing.T) {
	s, _ := collaborationFixture(t)
	_, token := collaborationUser(t, s, "benchmark", true)
	s.engine = NewEngine(s.m)
	for i := 0; i < 60; i++ {
		if _, err := s.m.CreateTask(fmt.Sprintf("Seeded task %d", i), "Stored goal", nil, 0, 300); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.m.pg.Exec(`INSERT INTO findings(task_id,vulnclass,name,severity,summary,evidence) SELECT (SELECT min(id) FROM tasks),'benchmark','Finding '||g,'medium',repeat('summary ',20),repeat('evidence ',40) FROM generate_series(1,1205) g`); err != nil {
		t.Fatal(err)
	}
	measure := func(path string) (time.Duration, int) {
		t.Helper()
		w := collaborationRequest(s, token, "GET", path, "")
		if w.Code != 200 {
			t.Fatalf("%s=%d %s", path, w.Code, w.Body.String())
		}
		start := time.Now()
		bytes := 0
		for i := 0; i < 12; i++ {
			w = collaborationRequest(s, token, "GET", path, "")
			if w.Code != 200 {
				t.Fatalf("%s=%d", path, w.Code)
			}
			bytes = w.Body.Len()
		}
		return time.Since(start) / 12, bytes
	}
	for _, pair := range [][2]string{{"/api/tasks", "/api/dashboard/tasks"}, {"/api/exploration/findings", "/api/dashboard/findings"}} {
		oldTime, oldBytes := measure(pair[0])
		newTime, newBytes := measure(pair[1])
		t.Logf("%s baseline=%s/%d bytes; %s refreshed=%s/%d bytes", pair[0], oldTime, oldBytes, pair[1], newTime, newBytes)
		if newBytes >= oldBytes {
			t.Fatalf("dashboard payload did not shrink: %d >= %d", newBytes, oldBytes)
		}
	}
}
func TestCollaborationWorkspaceFiltersAndSummary(t *testing.T) {
	s, dsn := collaborationFixture(t)
	// Databases seeded before the rename pick up the new default name on the next start.
	if _, err := s.m.pg.Exec(`UPDATE engagements SET name='Legacy private workspace',description='Preserved pre-collaboration records' WHERE legacy`); err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
	var name string
	if err := s.m.pg.QueryRow(`SELECT name FROM engagements WHERE legacy`).Scan(&name); err != nil || name != "General" {
		t.Fatalf("default engagement name=%q err=%v", name, err)
	}
	uid, token := collaborationUser(t, s, "viewer", false)
	eid := collaborationEngagement(t, s, "Filters", uid, "viewer")
	task, err := s.m.pg.CreateTaskWithOptions("Alpha recon", "Map hosts", db.TaskCreateOptions{EngagementID: eid})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.m.pg.CreateTaskWithOptions("Beta exploit", "Exploit", db.TaskCreateOptions{EngagementID: eid}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.m.pg.Exec(`INSERT INTO findings(task_id,vulnclass,name,severity,summary,evidence,status,verification_status) VALUES ($1,'x','SQLi','high','s','e','confirmed','verified'),($1,'x','XSS','low','s','e','pending','unverified'),($1,'x','IDOR','high','s','e','pending','unverified')`, task.ID); err != nil {
		t.Fatal(err)
	}
	total := func(path string) float64 {
		t.Helper()
		w := collaborationRequest(s, token, "GET", fmt.Sprintf("/api/collab/engagements/%d/%s", eid, path), "")
		var out struct{ Total float64 }
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
		return out.Total
	}
	for path, want := range map[string]float64{
		"tasks?q=alpha": 1, "tasks?status=created": 2, "tasks?status=done": 0,
		"findings?severity=high": 2, "findings?severity=high&status=pending": 1, "findings?q=xss": 1, "findings": 3,
	} {
		if got := total(path); got != want {
			t.Fatalf("%s total=%v want %v", path, got, want)
		}
	}
	w := collaborationRequest(s, token, "GET", fmt.Sprintf("/api/collab/engagements/%d/summary", eid), "")
	var sum struct {
		Tasks, Severity, Verification map[string]int
		Members                       int
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &sum) != nil {
		t.Fatalf("summary status=%d body=%s", w.Code, w.Body.String())
	}
	if sum.Tasks["created"] != 2 || sum.Severity["high"] != 2 || sum.Severity["low"] != 1 || sum.Verification["verified"] != 1 || sum.Members != 1 {
		t.Fatalf("summary=%+v", sum)
	}
	_, other := collaborationUser(t, s, "outsider", false)
	if w := collaborationRequest(s, other, "GET", fmt.Sprintf("/api/collab/engagements/%d/summary", eid), ""); w.Code != 404 {
		t.Fatalf("outsider summary status=%d", w.Code)
	}
}
