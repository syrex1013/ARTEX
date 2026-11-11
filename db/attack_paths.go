package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrAttackPathNotFound is returned when a path doesn't exist.
var ErrAttackPathNotFound = errors.New("attack path not found")

// AttackPath is a manual or suggested kill-chain narrative.
type AttackPath struct {
	ID           int64      `json:"id"`
	EngagementID *int64     `json:"engagement_id,omitempty"`
	TaskID       *int64     `json:"task_id,omitempty"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Source       string     `json:"source"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	Steps        []PathStep `json:"steps,omitempty"`
}

// PathStep is one step in an attack path.
type PathStep struct {
	ID        int64  `json:"id"`
	PathID    int64  `json:"path_id"`
	Position  int    `json:"position"`
	Kind      string `json:"kind"`
	RefID     string `json:"ref_id"`
	Tactic    string `json:"tactic"`
	Technique string `json:"technique"`
	Note      string `json:"note"`
}

func (d *DB) scanPath(row interface{ Scan(...any) error }) (*AttackPath, error) {
	var p AttackPath
	var engID, taskID sql.NullInt64
	err := row.Scan(&p.ID, &engID, &taskID, &p.Name, &p.Description, &p.Source, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if engID.Valid {
		p.EngagementID = &engID.Int64
	}
	if taskID.Valid {
		p.TaskID = &taskID.Int64
	}
	return &p, nil
}

const pathCols = `id, engagement_id, task_id, name, description, source, created_at, updated_at`

// CreateAttackPath inserts a new attack path.
func (d *DB) CreateAttackPath(p AttackPath) (*AttackPath, error) {
	if p.Name == "" {
		return nil, errors.New("name is required")
	}
	if p.Source == "" {
		p.Source = "manual"
	}
	r := d.QueryRow(`INSERT INTO attack_paths(engagement_id,task_id,name,description,source) VALUES ($1,$2,$3,$4,$5) RETURNING `+pathCols,
		p.EngagementID, p.TaskID, p.Name, p.Description, p.Source)
	return d.scanPath(r)
}

// GetAttackPath fetches one attack path with steps.
func (d *DB) GetAttackPath(id int64) (*AttackPath, error) {
	p, err := d.scanPath(d.QueryRow(`SELECT `+pathCols+` FROM attack_paths WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAttackPathNotFound
	}
	if err != nil {
		return nil, err
	}
	steps, err := d.listPathSteps(id)
	if err != nil {
		return nil, err
	}
	p.Steps = steps
	return p, nil
}

// ListAttackPaths returns paths filtered by engagement and/or task.
func (d *DB) ListAttackPaths(engagementID, taskID *int64) ([]*AttackPath, error) {
	q := `SELECT ` + pathCols + ` FROM attack_paths WHERE 1=1`
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
	var out []*AttackPath
	for rows.Next() {
		p, err := d.scanPath(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdateAttackPath replaces name/description of a path.
func (d *DB) UpdateAttackPath(id int64, p AttackPath) (*AttackPath, error) {
	_, err := d.Exec(`UPDATE attack_paths SET name=$1,description=$2 WHERE id=$3`, p.Name, p.Description, id)
	if err != nil {
		return nil, err
	}
	return d.GetAttackPath(id)
}

// DeleteAttackPath removes a path and its steps.
func (d *DB) DeleteAttackPath(id int64) error {
	res, err := d.Exec(`DELETE FROM attack_paths WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrAttackPathNotFound
	}
	return nil
}

// SetPathSteps replaces all steps of a path.
func (d *DB) SetPathSteps(pathID int64, steps []PathStep) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM attack_path_steps WHERE path_id=$1`, pathID); err != nil {
		return err
	}
	for _, s := range steps {
		if _, err := tx.Exec(`INSERT INTO attack_path_steps(path_id,position,kind,ref_id,tactic,technique,note) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			pathID, s.Position, s.Kind, s.RefID, s.Tactic, s.Technique, s.Note); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) listPathSteps(pathID int64) ([]PathStep, error) {
	rows, err := d.Query(`SELECT id,path_id,position,kind,ref_id,tactic,technique,note FROM attack_path_steps WHERE path_id=$1 ORDER BY position`, pathID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PathStep
	for rows.Next() {
		var s PathStep
		if err := rows.Scan(&s.ID, &s.PathID, &s.Position, &s.Kind, &s.RefID, &s.Tactic, &s.Technique, &s.Note); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
