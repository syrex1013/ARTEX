package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var ErrCampaignNotFound = errors.New("social engineering campaign not found")
var ErrSenderNotFound = errors.New("social engineering sender not found")
var ErrLandingPageNotFound = errors.New("landing page not found")

// ---- Senders ----------------------------------------------------------------

// SESender stores SMTP / AgentMail / webhook credentials.
type SESender struct {
	ID        int64             `json:"id"`
	Name      string            `json:"name"`
	Kind      string            `json:"kind"` // smtp | agentmail | webhook
	FromEmail string            `json:"from_email"`
	FromName  string            `json:"from_name"`
	Config    map[string]any    `json:"config"` // kind-specific fields (keys/passwords)
	Enabled   bool              `json:"enabled"`
	CreatedAt time.Time         `json:"created_at"`
}

func (d *DB) scanSender(row interface{ Scan(...any) error }) (*SESender, error) {
	var s SESender
	var configRaw []byte
	if err := row.Scan(&s.ID, &s.Name, &s.Kind, &s.FromEmail, &s.FromName, &configRaw, &s.Enabled, &s.CreatedAt); err != nil {
		return nil, err
	}
	if len(configRaw) > 0 {
		_ = json.Unmarshal(configRaw, &s.Config)
	}
	return &s, nil
}

const senderCols = `id, name, kind, from_email, from_name, config, enabled, created_at`

func (d *DB) CreateSender(s SESender) (*SESender, error) {
	if s.Name == "" || s.Kind == "" {
		return nil, errors.New("name and kind are required")
	}
	cfg, _ := json.Marshal(s.Config)
	r := d.QueryRow(`INSERT INTO social_eng_senders(name,kind,from_email,from_name,config) VALUES($1,$2,$3,$4,$5) RETURNING `+senderCols,
		s.Name, s.Kind, s.FromEmail, s.FromName, cfg)
	return d.scanSender(r)
}

func (d *DB) GetSender(id int64) (*SESender, error) {
	s, err := d.scanSender(d.QueryRow(`SELECT `+senderCols+` FROM social_eng_senders WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSenderNotFound
	}
	return s, err
}

func (d *DB) ListSenders() ([]*SESender, error) {
	rows, err := d.Query(`SELECT ` + senderCols + ` FROM social_eng_senders ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SESender
	for rows.Next() {
		s, err := d.scanSender(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (d *DB) UpdateSender(id int64, s SESender) (*SESender, error) {
	cfg, _ := json.Marshal(s.Config)
	r := d.QueryRow(`UPDATE social_eng_senders SET name=$1,kind=$2,from_email=$3,from_name=$4,config=$5,enabled=$6 WHERE id=$7 RETURNING `+senderCols,
		s.Name, s.Kind, s.FromEmail, s.FromName, cfg, s.Enabled, id)
	got, err := d.scanSender(r)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSenderNotFound
	}
	return got, err
}

func (d *DB) DeleteSender(id int64) error {
	res, err := d.Exec(`DELETE FROM social_eng_senders WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSenderNotFound
	}
	return nil
}

// ---- Landing Pages ----------------------------------------------------------

type SELandingPage struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	HTML               string    `json:"html"`
	CaptureCredentials bool      `json:"capture_credentials"`
	RedirectURL        string    `json:"redirect_url"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (d *DB) scanLandingPage(row interface{ Scan(...any) error }) (*SELandingPage, error) {
	var p SELandingPage
	err := row.Scan(&p.ID, &p.Name, &p.HTML, &p.CaptureCredentials, &p.RedirectURL, &p.CreatedAt, &p.UpdatedAt)
	return &p, err
}

const lpCols = `id, name, html, capture_credentials, redirect_url, created_at, updated_at`

func (d *DB) CreateLandingPage(p SELandingPage) (*SELandingPage, error) {
	if p.Name == "" {
		return nil, errors.New("name is required")
	}
	r := d.QueryRow(`INSERT INTO social_eng_landing_pages(name,html,capture_credentials,redirect_url) VALUES($1,$2,$3,$4) RETURNING `+lpCols,
		p.Name, p.HTML, p.CaptureCredentials, p.RedirectURL)
	return d.scanLandingPage(r)
}

func (d *DB) GetLandingPage(id int64) (*SELandingPage, error) {
	p, err := d.scanLandingPage(d.QueryRow(`SELECT `+lpCols+` FROM social_eng_landing_pages WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrLandingPageNotFound
	}
	return p, err
}

func (d *DB) ListLandingPages() ([]*SELandingPage, error) {
	rows, err := d.Query(`SELECT ` + lpCols + ` FROM social_eng_landing_pages ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SELandingPage
	for rows.Next() {
		p, err := d.scanLandingPage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) UpdateLandingPage(id int64, p SELandingPage) (*SELandingPage, error) {
	r := d.QueryRow(`UPDATE social_eng_landing_pages SET name=$1,html=$2,capture_credentials=$3,redirect_url=$4 WHERE id=$5 RETURNING `+lpCols,
		p.Name, p.HTML, p.CaptureCredentials, p.RedirectURL, id)
	got, err := d.scanLandingPage(r)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrLandingPageNotFound
	}
	return got, err
}

func (d *DB) DeleteLandingPage(id int64) error {
	res, err := d.Exec(`DELETE FROM social_eng_landing_pages WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrLandingPageNotFound
	}
	return nil
}

// ---- Captured Credentials ---------------------------------------------------

type SECredential struct {
	ID             int64          `json:"id"`
	CampaignID     *int64         `json:"campaign_id,omitempty"`
	TargetID       *int64         `json:"target_id,omitempty"`
	LandingPageID  *int64         `json:"landing_page_id,omitempty"`
	SubmittedAt    time.Time      `json:"submitted_at"`
	IP             string         `json:"ip"`
	UserAgent      string         `json:"user_agent"`
	Data           map[string]any `json:"data"`
}

func (d *DB) StoreCreds(cred SECredential) (*SECredential, error) {
	data, _ := json.Marshal(cred.Data)
	err := d.QueryRow(`INSERT INTO social_eng_credentials(campaign_id,target_id,landing_page_id,ip,user_agent,data) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,submitted_at`,
		cred.CampaignID, cred.TargetID, cred.LandingPageID, cred.IP, cred.UserAgent, data).
		Scan(&cred.ID, &cred.SubmittedAt)
	return &cred, err
}

func (d *DB) ListCreds(campaignID int64) ([]SECredential, error) {
	rows, err := d.Query(`SELECT id,campaign_id,target_id,landing_page_id,submitted_at,ip,user_agent,data FROM social_eng_credentials WHERE campaign_id=$1 ORDER BY submitted_at DESC`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SECredential
	for rows.Next() {
		var c SECredential
		var cid, tid, lpid sql.NullInt64
		var dataRaw []byte
		if err := rows.Scan(&c.ID, &cid, &tid, &lpid, &c.SubmittedAt, &c.IP, &c.UserAgent, &dataRaw); err != nil {
			return nil, err
		}
		if cid.Valid { c.CampaignID = &cid.Int64 }
		if tid.Valid { c.TargetID = &tid.Int64 }
		if lpid.Valid { c.LandingPageID = &lpid.Int64 }
		_ = json.Unmarshal(dataRaw, &c.Data)
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- Campaigns --------------------------------------------------------------

type SocialEngCampaign struct {
	ID             int64      `json:"id"`
	EngagementID   *int64     `json:"engagement_id,omitempty"`
	Name           string     `json:"name"`
	Kind           string     `json:"kind"`
	Status         string     `json:"status"`
	Objective      string     `json:"objective"`
	Technique      string     `json:"technique"`
	Pretext        string     `json:"pretext"`
	TemplateName   string     `json:"template_name"`
	RedirectURL    string     `json:"redirect_url"`
	SenderID       *int64     `json:"sender_id,omitempty"`
	LandingPageID  *int64     `json:"landing_page_id,omitempty"`
	FromEmail      string     `json:"from_email"`
	FromName       string     `json:"from_name"`
	SpoofFrom      string     `json:"spoof_from"`
	SpoofFromName  string     `json:"spoof_from_name"`
	Subject        string     `json:"subject"`
	HTMLBody       string     `json:"html_body"`
	TextBody       string     `json:"text_body"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	// Computed stats
	TotalTargets   int `json:"total_targets"`
	SentCount      int `json:"sent_count"`
	OpenedCount    int `json:"opened_count"`
	ClickedCount   int `json:"clicked_count"`
	SubmittedCount int `json:"submitted_count"`
	CredCount      int `json:"cred_count"`
}

type SocialEngTarget struct {
	ID          int64      `json:"id"`
	CampaignID  int64      `json:"campaign_id"`
	Email       string     `json:"email"`
	Phone       string     `json:"phone"`
	Name        string     `json:"name"`
	Department  string     `json:"department"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
	OpenedAt    *time.Time `json:"opened_at,omitempty"`
	ClickedAt   *time.Time `json:"clicked_at,omitempty"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	Notes       string     `json:"notes"`
}

const campaignCols = `id, engagement_id, name, kind, status, objective, technique, pretext,
  template_name, redirect_url, sender_id, landing_page_id,
  from_email, from_name, spoof_from, spoof_from_name,
  subject, html_body, text_body, created_at, updated_at`

func (d *DB) scanCampaign(row interface{ Scan(...any) error }) (*SocialEngCampaign, error) {
	var c SocialEngCampaign
	var engID, senderID, lpID sql.NullInt64
	err := row.Scan(
		&c.ID, &engID, &c.Name, &c.Kind, &c.Status, &c.Objective,
		&c.Technique, &c.Pretext, &c.TemplateName, &c.RedirectURL,
		&senderID, &lpID,
		&c.FromEmail, &c.FromName, &c.SpoofFrom, &c.SpoofFromName,
		&c.Subject, &c.HTMLBody, &c.TextBody, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if engID.Valid    { c.EngagementID = &engID.Int64 }
	if senderID.Valid { c.SenderID = &senderID.Int64 }
	if lpID.Valid     { c.LandingPageID = &lpID.Int64 }
	return &c, nil
}

func (d *DB) CreateCampaign(c SocialEngCampaign) (*SocialEngCampaign, error) {
	if c.Name == "" {
		return nil, errors.New("name is required")
	}
	if c.Kind == "" {
		c.Kind = "phishing"
	}
	r := d.QueryRow(`
INSERT INTO social_eng_campaigns(
  engagement_id,name,kind,objective,technique,pretext,
  template_name,redirect_url,sender_id,landing_page_id,
  from_email,from_name,spoof_from,spoof_from_name,
  subject,html_body,text_body
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
RETURNING `+campaignCols,
		c.EngagementID, c.Name, c.Kind, c.Objective, c.Technique, c.Pretext,
		c.TemplateName, c.RedirectURL, c.SenderID, c.LandingPageID,
		c.FromEmail, c.FromName, c.SpoofFrom, c.SpoofFromName,
		c.Subject, c.HTMLBody, c.TextBody,
	)
	return d.scanCampaign(r)
}

func (d *DB) GetCampaign(id int64) (*SocialEngCampaign, error) {
	c, err := d.scanCampaign(d.QueryRow(`SELECT `+campaignCols+` FROM social_eng_campaigns WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCampaignNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = d.QueryRow(`SELECT
    COUNT(*), COUNT(sent_at), COUNT(opened_at), COUNT(clicked_at), COUNT(submitted_at)
    FROM social_eng_targets WHERE campaign_id=$1`, id).
		Scan(&c.TotalTargets, &c.SentCount, &c.OpenedCount, &c.ClickedCount, &c.SubmittedCount)
	_ = d.QueryRow(`SELECT COUNT(*) FROM social_eng_credentials WHERE campaign_id=$1`, id).Scan(&c.CredCount)
	return c, nil
}

func (d *DB) ListCampaigns(engagementID *int64) ([]*SocialEngCampaign, error) {
	q := `SELECT ` + campaignCols + ` FROM social_eng_campaigns`
	var args []any
	if engagementID != nil {
		args = append(args, *engagementID)
		q += ` WHERE engagement_id=$1`
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SocialEngCampaign
	for rows.Next() {
		c, err := d.scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (d *DB) UpdateCampaign(id int64, c SocialEngCampaign) (*SocialEngCampaign, error) {
	r := d.QueryRow(`
UPDATE social_eng_campaigns SET
  name=$1, kind=$2, status=$3, objective=$4, technique=$5, pretext=$6,
  template_name=$7, redirect_url=$8, sender_id=$9, landing_page_id=$10,
  from_email=$11, from_name=$12, spoof_from=$13, spoof_from_name=$14,
  subject=$15, html_body=$16, text_body=$17
WHERE id=$18 RETURNING `+campaignCols,
		c.Name, c.Kind, c.Status, c.Objective, c.Technique, c.Pretext,
		c.TemplateName, c.RedirectURL, c.SenderID, c.LandingPageID,
		c.FromEmail, c.FromName, c.SpoofFrom, c.SpoofFromName,
		c.Subject, c.HTMLBody, c.TextBody, id,
	)
	got, err := d.scanCampaign(r)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCampaignNotFound
	}
	return got, err
}

func (d *DB) UpdateCampaignStatus(id int64, status string) error {
	_, err := d.Exec(`UPDATE social_eng_campaigns SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (d *DB) DeleteCampaign(id int64) error {
	res, err := d.Exec(`DELETE FROM social_eng_campaigns WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCampaignNotFound
	}
	return nil
}

// ---- Targets ----------------------------------------------------------------

func (d *DB) AddTarget(t SocialEngTarget) (*SocialEngTarget, error) {
	err := d.QueryRow(`INSERT INTO social_eng_targets(campaign_id,email,phone,name,department,notes)
VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
		t.CampaignID, t.Email, t.Phone, t.Name, t.Department, t.Notes).Scan(&t.ID)
	return &t, err
}

func (d *DB) ListTargets(campaignID int64) ([]SocialEngTarget, error) {
	rows, err := d.Query(`
SELECT id,campaign_id,email,phone,name,department,sent_at,opened_at,clicked_at,submitted_at,last_error,notes
FROM social_eng_targets WHERE campaign_id=$1 ORDER BY id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SocialEngTarget
	for rows.Next() {
		var t SocialEngTarget
		var sent, opened, clicked, submitted sql.NullTime
		if err := rows.Scan(&t.ID, &t.CampaignID, &t.Email, &t.Phone, &t.Name, &t.Department,
			&sent, &opened, &clicked, &submitted, &t.LastError, &t.Notes); err != nil {
			return nil, err
		}
		if sent.Valid     { t.SentAt = &sent.Time }
		if opened.Valid   { t.OpenedAt = &opened.Time }
		if clicked.Valid  { t.ClickedAt = &clicked.Time }
		if submitted.Valid { t.SubmittedAt = &submitted.Time }
		out = append(out, t)
	}
	return out, rows.Err()
}

// MarkTargetSent records send timestamp and clears last_error.
func (d *DB) MarkTargetSent(id int64) error {
	_, err := d.Exec(`UPDATE social_eng_targets SET sent_at=now(), last_error='' WHERE id=$1`, id)
	return err
}

// MarkTargetError records a send failure.
func (d *DB) MarkTargetError(id int64, msg string) error {
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, err := d.Exec(`UPDATE social_eng_targets SET last_error=$1 WHERE id=$2`, msg, id)
	return err
}

// RecordTargetEvent marks a tracking event (opened, clicked, submitted) for a target.
func (d *DB) RecordTargetEvent(targetID int64, event string) error {
	col := ""
	switch event {
	case "sent":
		col = "sent_at"
	case "opened":
		col = "opened_at"
	case "clicked":
		col = "clicked_at"
	case "submitted":
		col = "submitted_at"
	default:
		return errors.New("unknown event: " + event)
	}
	_, err := d.Exec(`UPDATE social_eng_targets SET `+col+`=now() WHERE id=$1`, targetID)
	return err
}
