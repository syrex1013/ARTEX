package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type localUser struct {
	ID          int64           `json:"id"`
	Username    string          `json:"username"`
	DisplayName string          `json:"display_name"`
	Admin       bool            `json:"admin"`
	Enabled     bool            `json:"enabled"`
	Preferences json.RawMessage `json:"preferences"`
	SessionID   string          `json:"session_id,omitempty"`
}
type identityKey struct{}

func requestUser(r *http.Request) *localUser {
	u, _ := r.Context().Value(identityKey{}).(*localUser)
	return u
}

// Every authenticated request consults persisted sessions and enabled state.
// Legacy shared-identity tokens deliberately have no matching session.
func (s *Server) sessionUser(r *http.Request) (*localUser, error) {
	if s.m == nil || s.m.pg == nil {
		return nil, sql.ErrNoRows
	}
	var c jwt.RegisteredClaims
	t, err := jwt.ParseWithClaims(extractToken(r), &c, func(*jwt.Token) (any, error) { return s.jwtKey, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil || !t.Valid || c.ID == "" {
		return nil, sql.ErrNoRows
	}
	if _, err = uuid.Parse(c.ID); err != nil {
		return nil, sql.ErrNoRows
	}
	u := &localUser{SessionID: c.ID}
	err = s.m.pg.QueryRowContext(r.Context(), `SELECT u.id,u.username,u.display_name,u.admin,u.enabled,u.preferences
 FROM user_sessions s JOIN local_users u ON u.id=s.user_id
 WHERE s.id=$1 AND s.revoked_at IS NULL AND s.expires_at>now() AND u.enabled AND u.id::text=$2`, c.ID, c.Subject).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.Admin, &u.Enabled, &u.Preferences)
	return u, err
}
func (s *Server) issueSession(r *http.Request, id int64) (string, error) {
	limits, err := s.collaborationLimits()
	if err != nil {
		return "", err
	}
	sid := uuid.NewString()
	expiry := time.Now().Add(time.Duration(limits.SessionDays) * 24 * time.Hour)
	if _, err := s.m.pg.ExecContext(r.Context(), `INSERT INTO user_sessions(id,user_id,expires_at,user_agent) VALUES($1,$2,$3,$4)`, sid, id, expiry, r.UserAgent()); err != nil {
		return "", err
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: strconv.FormatInt(id, 10), ID: sid, IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(expiry)}).SignedString(s.jwtKey)
}
func (s *Server) accountLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r, s.trustedProxies)
	if wait := s.loginLimit.allow(ip); wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeErr(w, 429, "Too many login attempts")
		return
	}
	var q struct{ Username, Password string }
	if decode(r, &q) != nil {
		writeErr(w, 400, "Invalid request format")
		return
	}
	var id int64
	var hash string
	var enabled bool
	err := s.m.pg.QueryRowContext(r.Context(), `SELECT id,password_hash,enabled FROM local_users WHERE username=$1`, q.Username).Scan(&id, &hash, &enabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if err != nil || !enabled || bcrypt.CompareHashAndPassword([]byte(hash), []byte(q.Password)) != nil {
		writeErr(w, 401, "Incorrect username or password")
		return
	}
	tok, err := s.issueSession(r, id)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	s.loginLimit.reset(ip)
	writeJSON(w, 200, map[string]any{"token": tok})
}

func (s *Server) registerAccountRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/auth/me", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, requestUser(r)) })
	m.HandleFunc("PATCH /api/auth/me", s.updateAccount)
	m.HandleFunc("POST /api/auth/logout", s.revokeSession)
	m.HandleFunc("GET /api/auth/sessions", s.listSessions)
	m.HandleFunc("DELETE /api/auth/sessions/{sid}", s.revokeSession)
	m.HandleFunc("GET /api/accounts", s.listAccounts)
	m.HandleFunc("POST /api/accounts", s.createAccount)
	m.HandleFunc("PATCH /api/accounts/{uid}", s.updateAccount)
	m.HandleFunc("POST /api/accounts/{uid}/reset-password", s.resetAccountPassword)
}

const publicUserColumns = `id,username,display_name,admin,enabled,preferences,created_at`

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	s.collabJSON(w, r, `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT `+publicUserColumns+` FROM local_users ORDER BY id) x`)
}
func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	s.collabJSON(w, r, `SELECT COALESCE(jsonb_agg(x),'[]') FROM (SELECT id,created_at,expires_at,user_agent,id::text=$2 AS current FROM user_sessions WHERE user_id=$1 AND revoked_at IS NULL AND expires_at>now() ORDER BY created_at DESC) x`, requestUser(r).ID, requestUser(r).SessionID)
}
func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	u := requestUser(r)
	sid := r.PathValue("sid")
	if sid == "" {
		sid = u.SessionID
	}
	if _, err := uuid.Parse(sid); err != nil {
		writeErr(w, 404, "Not found")
		return
	}
	s.collabMutation(w, r, 0, "session.revoke", sid, `UPDATE user_sessions SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, sid, u.ID)
}
func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
		Admin       bool   `json:"admin"`
	}
	if decode(r, &q) != nil || strings.TrimSpace(q.Username) == "" || len(q.Username) > 80 || strings.TrimSpace(q.DisplayName) == "" {
		writeErr(w, 400, "Username and display name required")
		return
	}
	if msg := validatePassword(q.Password); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(q.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "Password encryption failed")
		return
	}
	s.collabMutation(w, r, 0, "account.create", q.Username, `INSERT INTO local_users(username,display_name,password_hash,admin) VALUES($1,$2,$3,$4)`, q.Username, q.DisplayName, string(hash), q.Admin)
}
func validPreferences(raw json.RawMessage) bool {
	var p map[string]any
	if len(raw) > 4096 || json.Unmarshal(raw, &p) != nil || p == nil {
		return false
	}
	for k, v := range p {
		str, ok := v.(string)
		if !ok {
			return false
		}
		switch k {
		case "timezone":
			if _, err := time.LoadLocation(str); err != nil {
				return false
			}
		case "send_key":
			if str != "enter" && str != "mod-enter" {
				return false
			}
		case "theme":
			if str != "light" && str != "dark" && str != "system" {
				return false
			}
		case "density":
			if str != "comfortable" && str != "compact" {
				return false
			}
		default:
			return false
		}
	}
	return true
}
func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	u := requestUser(r)
	id := u.ID
	if raw := r.PathValue("uid"); raw != "" {
		var err error
		id, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			writeErr(w, 404, "Not found")
			return
		}
	}
	var q struct {
		DisplayName *string         `json:"display_name"`
		Enabled     *bool           `json:"enabled"`
		Preferences json.RawMessage `json:"preferences"`
	}
	if decode(r, &q) != nil || q.DisplayName != nil && (strings.TrimSpace(*q.DisplayName) == "" || len(*q.DisplayName) > 200) || len(q.Preferences) > 0 && !validPreferences(q.Preferences) {
		writeErr(w, 400, "Invalid account preferences")
		return
	}
	if q.Enabled != nil && (!u.Admin || id == u.ID) {
		writeErr(w, 403, "Cannot change this enabled state")
		return
	}
	var prefs any
	if len(q.Preferences) > 0 {
		prefs = string(q.Preferences)
	}
	s.collabMutation(w, r, 0, "account.update", strconv.FormatInt(id, 10), `UPDATE local_users SET display_name=COALESCE($2,display_name),enabled=COALESCE($3,enabled),preferences=COALESCE($4::jsonb,preferences) WHERE id=$1`, id, q.DisplayName, q.Enabled, prefs)
}
func (s *Server) resetAccountPassword(w http.ResponseWriter, r *http.Request) {
	s.changeLocalPassword(w, r, true)
}
func (s *Server) changeLocalPassword(w http.ResponseWriter, r *http.Request, reset bool) {
	u := requestUser(r)
	if u == nil {
		var err error
		u, err = s.sessionUser(r)
		if err != nil {
			writeErr(w, 401, "Unauthorized")
			return
		}
	}
	id := u.ID
	if reset {
		var err error
		id, err = strconv.ParseInt(r.PathValue("uid"), 10, 64)
		if err != nil {
			writeErr(w, 404, "Not found")
			return
		}
	}
	var q struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if decode(r, &q) != nil {
		writeErr(w, 400, "Invalid request format")
		return
	}
	if msg := validatePassword(q.NewPassword); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	tx, err := s.m.pg.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	defer tx.Rollback()
	var hash string
	err = tx.QueryRow(`SELECT password_hash FROM local_users WHERE id=$1 FOR UPDATE`, id).Scan(&hash)
	if err != nil {
		writeErr(w, 404, "Not found")
		return
	}
	if !reset && bcrypt.CompareHashAndPassword([]byte(hash), []byte(q.OldPassword)) != nil {
		writeErr(w, 403, "Current password is wrong")
		return
	}
	next, err := bcrypt.GenerateFromPassword([]byte(q.NewPassword), bcrypt.DefaultCost)
	if err == nil {
		_, err = tx.Exec(`UPDATE local_users SET password_hash=$2 WHERE id=$1`, id, string(next))
	}
	if err == nil {
		_, err = tx.Exec(`UPDATE user_sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id)
	}
	// Preserve compatibility with the existing local recovery script.
	if err == nil {
		_, err = tx.Exec(`UPDATE settings SET value=$2 WHERE key='auth.password_hash' AND EXISTS(SELECT 1 FROM local_users WHERE id=$1 AND username='ARTEX' AND admin)`, id, string(next))
	}
	if err == nil {
		_, err = tx.Exec(`INSERT INTO collaboration_audit(actor_id,action,subject) VALUES($1,'account.password',$2)`, u.ID, strconv.FormatInt(id, 10))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) initializeLocalAdmin(r *http.Request, hash string) (int64, error) {
	tx, err := s.m.pg.BeginTx(r.Context(), nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO settings(key,value) VALUES('auth.password_hash',$1)`, hash); err != nil {
		return 0, err
	}
	var id int64
	if err = tx.QueryRow(`INSERT INTO local_users(username,display_name,password_hash,admin) VALUES('ARTEX','ARTEX',$1,true) RETURNING id`, hash).Scan(&id); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`INSERT INTO engagement_members(engagement_id,user_id,role) VALUES(legacy_engagement_id(),$1,'manager');`, id); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`UPDATE conversations SET owner_id=$1 WHERE owner_id IS NULL`, id); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`INSERT INTO collaboration_audit(actor_id,action) VALUES($1,'account.initialize')`, id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}
