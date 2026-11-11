package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrReportNotFound is returned when a report doesn't exist.
var ErrReportNotFound = errors.New("report not found")

// ErrReportImmutable is returned when trying to edit an approved report.
var ErrReportImmutable = errors.New("approved reports are read-only; create a new version instead")

// ErrReportOpenComments is returned when trying to approve a report with unresolved comments.
var ErrReportOpenComments = errors.New("all comments must be resolved before approving")

// ReportBranding holds display customisation.
type ReportBranding struct {
	Client     string `json:"client"`
	Author     string `json:"author"`
	LogoDataURL string `json:"logo_data_url,omitempty"`
	Footer     string `json:"footer"`
}

// Report is a formal deliverable.
type Report struct {
	ID           int64          `json:"id"`
	EngagementID *int64         `json:"engagement_id,omitempty"`
	TaskID       *int64         `json:"task_id,omitempty"`
	Title        string         `json:"title"`
	Template     string         `json:"template"`
	Status       string         `json:"status"`
	Branding     ReportBranding `json:"branding"`
	ExecSummary  string         `json:"exec_summary"`
	Version      int            `json:"version"`
	ApprovedBy   string         `json:"approved_by,omitempty"`
	ApprovedAt   *time.Time     `json:"approved_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// ReportFinding is an entry in a report's finding list.
type ReportFinding struct {
	ReportID        int64  `json:"report_id"`
	FindingID       int64  `json:"finding_id"`
	Position        int    `json:"position"`
	Included        bool   `json:"included"`
	SummaryOverride string `json:"summary_override"`
}

// ReportVersion is an immutable snapshot.
type ReportVersion struct {
	ID        int64           `json:"id"`
	ReportID  int64           `json:"report_id"`
	Version   int             `json:"version"`
	Snapshot  json.RawMessage `json:"snapshot"`
	CreatedAt time.Time       `json:"created_at"`
	CreatedBy string          `json:"created_by"`
}

// ReportComment is a review comment.
type ReportComment struct {
	ID        int64      `json:"id"`
	ReportID  int64      `json:"report_id"`
	FindingID *int64     `json:"finding_id,omitempty"`
	Author    string     `json:"author"`
	Body      string     `json:"body"`
	Resolved  bool       `json:"resolved"`
	CreatedAt time.Time  `json:"created_at"`
}

// ReportInput is used for create and update.
type ReportInput struct {
	Title        string         `json:"title"`
	Template     string         `json:"template"`
	EngagementID *int64         `json:"engagement_id,omitempty"`
	TaskID       *int64         `json:"task_id,omitempty"`
	Branding     ReportBranding `json:"branding"`
	ExecSummary  string         `json:"exec_summary"`
}

func (d *DB) scanReport(row interface{ Scan(...any) error }) (*Report, error) {
	var r Report
	var brandJSON string
	var engID, taskID sql.NullInt64
	var approvedBy sql.NullString
	var approvedAt sql.NullTime
	err := row.Scan(&r.ID, &engID, &taskID, &r.Title, &r.Template, &r.Status,
		&brandJSON, &r.ExecSummary, &r.Version, &approvedBy, &approvedAt, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(brandJSON), &r.Branding)
	if engID.Valid {
		r.EngagementID = &engID.Int64
	}
	if taskID.Valid {
		r.TaskID = &taskID.Int64
	}
	if approvedBy.Valid {
		r.ApprovedBy = approvedBy.String
	}
	if approvedAt.Valid {
		r.ApprovedAt = &approvedAt.Time
	}
	return &r, nil
}

const reportCols = `id, engagement_id, task_id, title, template, status, branding, exec_summary, version, approved_by, approved_at, created_at, updated_at`

// CreateReport inserts a new draft report.
func (d *DB) CreateReport(in ReportInput) (*Report, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return nil, errors.New("title is required")
	}
	if in.Template == "" {
		in.Template = "technical"
	}
	brand, _ := json.Marshal(in.Branding)
	r := d.QueryRow(`INSERT INTO reports(engagement_id,task_id,title,template,branding,exec_summary)
VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+reportCols,
		in.EngagementID, in.TaskID, in.Title, in.Template, string(brand), in.ExecSummary)
	return d.scanReport(r)
}

// GetReport fetches one report by id.
func (d *DB) GetReport(id int64) (*Report, error) {
	r, err := d.scanReport(d.QueryRow(`SELECT `+reportCols+` FROM reports WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReportNotFound
	}
	return r, err
}

// ListReports returns reports, optionally filtered by engagement or task.
func (d *DB) ListReports(engagementID, taskID *int64) ([]*Report, error) {
	q := `SELECT ` + reportCols + ` FROM reports WHERE 1=1`
	var args []any
	if engagementID != nil {
		args = append(args, *engagementID)
		q += fmt.Sprintf(` AND engagement_id=$%d`, len(args))
	}
	if taskID != nil {
		args = append(args, *taskID)
		q += fmt.Sprintf(` AND task_id=$%d`, len(args))
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Report
	for rows.Next() {
		r, err := d.scanReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateReport patches title/template/branding/exec_summary (not status/version).
func (d *DB) UpdateReport(id int64, in ReportInput) (*Report, error) {
	r, err := d.GetReport(id)
	if err != nil {
		return nil, err
	}
	if r.Status == "approved" {
		return nil, ErrReportImmutable
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		in.Title = r.Title
	}
	if in.Template == "" {
		in.Template = r.Template
	}
	brand, _ := json.Marshal(in.Branding)
	_, err = d.Exec(`UPDATE reports SET title=$1,template=$2,branding=$3,exec_summary=$4 WHERE id=$5`,
		in.Title, in.Template, string(brand), in.ExecSummary, id)
	if err != nil {
		return nil, err
	}
	return d.GetReport(id)
}

// DeleteReport deletes a report and all its data.
func (d *DB) DeleteReport(id int64) error {
	res, err := d.Exec(`DELETE FROM reports WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrReportNotFound
	}
	return nil
}

// TransitionReport advances a report's status (draft → in_review → approved).
func (d *DB) TransitionReport(id int64, to, actor string) (*Report, error) {
	r, err := d.GetReport(id)
	if err != nil {
		return nil, err
	}
	allowed := map[string][]string{
		"draft":     {"in_review"},
		"in_review": {"approved", "draft"},
		"approved":  {},
	}
	ok := false
	for _, s := range allowed[r.Status] {
		if s == to {
			ok = true
			break
		}
	}
	if !ok {
		return nil, fmt.Errorf("cannot transition from %s to %s", r.Status, to)
	}
	if to == "approved" {
		var open int
		if err := d.QueryRow(`SELECT COUNT(*) FROM report_comments WHERE report_id=$1 AND resolved=false`, id).Scan(&open); err != nil {
			return nil, err
		}
		if open > 0 {
			return nil, ErrReportOpenComments
		}
	}
	// Snapshot before transitioning.
	snap, _ := json.Marshal(r)
	if _, err := d.Exec(`INSERT INTO report_versions(report_id,version,snapshot,created_by) VALUES ($1,$2,$3,$4)`,
		id, r.Version, string(snap), actor); err != nil {
		return nil, err
	}
	newVer := r.Version + 1
	if to == "approved" {
		_, err = d.Exec(`UPDATE reports SET status=$1, version=$2, approved_by=$3, approved_at=now() WHERE id=$4`,
			to, newVer, actor, id)
	} else {
		_, err = d.Exec(`UPDATE reports SET status=$1, version=$2 WHERE id=$3`, to, newVer, id)
	}
	if err != nil {
		return nil, err
	}
	return d.GetReport(id)
}

// SetReportFindings replaces the finding list for a report.
func (d *DB) SetReportFindings(reportID int64, entries []ReportFinding) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM report_findings WHERE report_id=$1`, reportID); err != nil {
		return err
	}
	for _, e := range entries {
		if _, err := tx.Exec(`INSERT INTO report_findings(report_id,finding_id,position,included,summary_override) VALUES ($1,$2,$3,$4,$5)`,
			reportID, e.FindingID, e.Position, e.Included, e.SummaryOverride); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListReportFindings returns the ordered finding list for a report.
func (d *DB) ListReportFindings(reportID int64) ([]ReportFinding, error) {
	rows, err := d.Query(`SELECT report_id,finding_id,position,included,summary_override FROM report_findings WHERE report_id=$1 ORDER BY position`, reportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReportFinding
	for rows.Next() {
		var e ReportFinding
		if err := rows.Scan(&e.ReportID, &e.FindingID, &e.Position, &e.Included, &e.SummaryOverride); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddReportComment adds a comment.
func (d *DB) AddReportComment(c ReportComment) (*ReportComment, error) {
	c.Body = strings.TrimSpace(c.Body)
	if c.Body == "" {
		return nil, errors.New("comment body is required")
	}
	err := d.QueryRow(`INSERT INTO report_comments(report_id,finding_id,author,body) VALUES ($1,$2,$3,$4) RETURNING id,created_at`,
		c.ReportID, c.FindingID, c.Author, c.Body).Scan(&c.ID, &c.CreatedAt)
	return &c, err
}

// ListReportComments returns comments for a report.
func (d *DB) ListReportComments(reportID int64) ([]ReportComment, error) {
	rows, err := d.Query(`SELECT id,report_id,finding_id,author,body,resolved,created_at FROM report_comments WHERE report_id=$1 ORDER BY created_at`, reportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReportComment
	for rows.Next() {
		var c ReportComment
		var fid sql.NullInt64
		if err := rows.Scan(&c.ID, &c.ReportID, &fid, &c.Author, &c.Body, &c.Resolved, &c.CreatedAt); err != nil {
			return nil, err
		}
		if fid.Valid {
			c.FindingID = &fid.Int64
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ResolveComment marks a comment resolved/unresolved.
func (d *DB) ResolveComment(commentID int64, resolved bool) error {
	res, err := d.Exec(`UPDATE report_comments SET resolved=$1 WHERE id=$2`, resolved, commentID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("comment not found")
	}
	return nil
}

// ListReportVersions returns snapshots for a report.
func (d *DB) ListReportVersions(reportID int64) ([]ReportVersion, error) {
	rows, err := d.Query(`SELECT id,report_id,version,snapshot,created_at,created_by FROM report_versions WHERE report_id=$1 ORDER BY version DESC`, reportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReportVersion
	for rows.Next() {
		var v ReportVersion
		if err := rows.Scan(&v.ID, &v.ReportID, &v.Version, &v.Snapshot, &v.CreatedAt, &v.CreatedBy); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
