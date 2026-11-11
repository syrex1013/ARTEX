package db

import (
	"database/sql"
	"errors"
	"time"
)

// ErrCampaignNotFound is returned when a campaign doesn't exist.
var ErrCampaignNotFound = errors.New("social engineering campaign not found")

// SocialEngCampaign represents a phishing/vishing/smishing campaign.
type SocialEngCampaign struct {
	ID           int64      `json:"id"`
	EngagementID *int64     `json:"engagement_id,omitempty"`
	Name         string     `json:"name"`
	Kind         string     `json:"kind"`
	Status       string     `json:"status"`
	Objective    string     `json:"objective"`
	TemplateName string     `json:"template_name"`
	RedirectURL  string     `json:"redirect_url"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// Computed stats
	TotalTargets    int `json:"total_targets"`
	SentCount       int `json:"sent_count"`
	OpenedCount     int `json:"opened_count"`
	ClickedCount    int `json:"clicked_count"`
	SubmittedCount  int `json:"submitted_count"`
}

// SocialEngTarget is one target in a campaign.
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
	Notes       string     `json:"notes"`
}

func (d *DB) scanCampaign(row interface{ Scan(...any) error }) (*SocialEngCampaign, error) {
	var c SocialEngCampaign
	var engID sql.NullInt64
	err := row.Scan(&c.ID, &engID, &c.Name, &c.Kind, &c.Status, &c.Objective, &c.TemplateName, &c.RedirectURL, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if engID.Valid {
		c.EngagementID = &engID.Int64
	}
	return &c, nil
}

const campaignCols = `id, engagement_id, name, kind, status, objective, template_name, redirect_url, created_at, updated_at`

// CreateCampaign inserts a new campaign.
func (d *DB) CreateCampaign(c SocialEngCampaign) (*SocialEngCampaign, error) {
	if c.Name == "" {
		return nil, errors.New("name is required")
	}
	if c.Kind == "" {
		c.Kind = "phishing"
	}
	r := d.QueryRow(`INSERT INTO social_eng_campaigns(engagement_id,name,kind,objective,template_name,redirect_url) VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+campaignCols,
		c.EngagementID, c.Name, c.Kind, c.Objective, c.TemplateName, c.RedirectURL)
	return d.scanCampaign(r)
}

// GetCampaign fetches one campaign with stats.
func (d *DB) GetCampaign(id int64) (*SocialEngCampaign, error) {
	c, err := d.scanCampaign(d.QueryRow(`SELECT `+campaignCols+` FROM social_eng_campaigns WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCampaignNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = d.QueryRow(`SELECT COUNT(*), COUNT(sent_at), COUNT(opened_at), COUNT(clicked_at), COUNT(submitted_at) FROM social_eng_targets WHERE campaign_id=$1`, id).
		Scan(&c.TotalTargets, &c.SentCount, &c.OpenedCount, &c.ClickedCount, &c.SubmittedCount)
	return c, nil
}

// ListCampaigns returns campaigns optionally filtered by engagement.
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

// UpdateCampaignStatus sets the campaign status.
func (d *DB) UpdateCampaignStatus(id int64, status string) error {
	_, err := d.Exec(`UPDATE social_eng_campaigns SET status=$1 WHERE id=$2`, status, id)
	return err
}

// DeleteCampaign removes a campaign and its targets.
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

// AddTarget adds a target to a campaign.
func (d *DB) AddTarget(t SocialEngTarget) (*SocialEngTarget, error) {
	err := d.QueryRow(`INSERT INTO social_eng_targets(campaign_id,email,phone,name,department,notes) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		t.CampaignID, t.Email, t.Phone, t.Name, t.Department, t.Notes).Scan(&t.ID)
	return &t, err
}

// ListTargets returns targets for a campaign.
func (d *DB) ListTargets(campaignID int64) ([]SocialEngTarget, error) {
	rows, err := d.Query(`SELECT id,campaign_id,email,phone,name,department,sent_at,opened_at,clicked_at,submitted_at,notes FROM social_eng_targets WHERE campaign_id=$1 ORDER BY id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SocialEngTarget
	for rows.Next() {
		var t SocialEngTarget
		var sent, opened, clicked, submitted sql.NullTime
		if err := rows.Scan(&t.ID, &t.CampaignID, &t.Email, &t.Phone, &t.Name, &t.Department, &sent, &opened, &clicked, &submitted, &t.Notes); err != nil {
			return nil, err
		}
		if sent.Valid {
			t.SentAt = &sent.Time
		}
		if opened.Valid {
			t.OpenedAt = &opened.Time
		}
		if clicked.Valid {
			t.ClickedAt = &clicked.Time
		}
		if submitted.Valid {
			t.SubmittedAt = &submitted.Time
		}
		out = append(out, t)
	}
	return out, rows.Err()
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
