package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Autumn-27/artex/classify"
	"github.com/Autumn-27/artex/cvss"
)

// FindingClassification is the reporting metadata of a finding.
type FindingClassification struct {
	FindingID    int64    `json:"finding_id"`
	CVSSVector   string   `json:"cvss_vector"`
	CVSSScore    *float64 `json:"cvss_score,omitempty"`
	CVSSSeverity string   `json:"cvss_severity,omitempty"`
	CWE          []string `json:"cwe"`
	OWASP        []string `json:"owasp"`
	Attack       []string `json:"attack"`
	Remediation  string   `json:"remediation"`
}

// Normalize validates and canonicalises the writable fields. The score of a CVSS 3.x vector is always computed
// here; a CVSS 4.0 vector is stored as written and needs a score between 0 and 10 supplied by the caller.
func (c *FindingClassification) Normalize() error {
	var bad string
	if c.CWE, bad = classify.NormalizeList("cwe", c.CWE); bad != "" {
		return errors.New("invalid CWE id: " + bad)
	}
	if c.OWASP, bad = classify.NormalizeList("owasp", c.OWASP); bad != "" {
		return errors.New("invalid OWASP Top 10 id: " + bad + " (want e.g. A03:2021)")
	}
	if c.Attack, bad = classify.NormalizeList("attack", c.Attack); bad != "" {
		return errors.New("invalid ATT&CK technique id: " + bad + " (want e.g. T1190 or T1110.003)")
	}
	c.CVSSVector = strings.TrimSpace(c.CVSSVector)
	c.CVSSSeverity = ""
	switch {
	case c.CVSSVector == "":
		c.CVSSScore = nil
	case strings.HasPrefix(c.CVSSVector, "CVSS:4.0/"):
		if c.CVSSScore == nil || *c.CVSSScore < 0 || *c.CVSSScore > 10 {
			return errors.New("a CVSS 4.0 vector needs a score between 0 and 10 (scoring 4.0 vectors is not built in)")
		}
		c.CVSSSeverity = cvss.Severity(*c.CVSSScore)
	default:
		r, err := cvss.Score(c.CVSSVector)
		if err != nil {
			return err
		}
		c.CVSSVector, c.CVSSScore, c.CVSSSeverity = r.Vector, &r.Score, r.Severity
	}
	c.Remediation = strings.TrimSpace(c.Remediation)
	if len(c.Remediation) > 20000 {
		return errors.New("remediation text is too long (max 20000 characters)")
	}
	return nil
}

func (c *FindingClassification) fill() {
	if c.CWE == nil {
		c.CWE = []string{}
	}
	if c.OWASP == nil {
		c.OWASP = []string{}
	}
	if c.Attack == nil {
		c.Attack = []string{}
	}
	if c.CVSSScore != nil && c.CVSSSeverity == "" {
		c.CVSSSeverity = cvss.Severity(*c.CVSSScore)
	}
}

const classificationCols = `finding_id, cvss_vector, cvss_score::float8, cwe, owasp, attack, remediation`

func scanClassification(sc interface{ Scan(...any) error }) (*FindingClassification, error) {
	var c FindingClassification
	var cwe, owasp, attack []byte
	if err := sc.Scan(&c.FindingID, &c.CVSSVector, &c.CVSSScore, &cwe, &owasp, &attack, &c.Remediation); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(cwe, &c.CWE)
	_ = json.Unmarshal(owasp, &c.OWASP)
	_ = json.Unmarshal(attack, &c.Attack)
	c.fill()
	return &c, nil
}

// GetFindingClassification returns the classification of a finding (an empty one when never set).
func (d *DB) GetFindingClassification(findingID int64) (*FindingClassification, error) {
	c, err := scanClassification(d.QueryRow(`SELECT `+classificationCols+` FROM finding_classification WHERE finding_id=$1`, findingID))
	if err == sql.ErrNoRows {
		empty := &FindingClassification{FindingID: findingID}
		empty.fill()
		return empty, nil
	}
	return c, err
}

// SaveFindingClassification replaces the classification of an existing finding.
func (d *DB) SaveFindingClassification(c *FindingClassification) error {
	if err := c.Normalize(); err != nil {
		return err
	}
	cwe, _ := json.Marshal(c.CWE)
	owasp, _ := json.Marshal(c.OWASP)
	attack, _ := json.Marshal(c.Attack)
	var exists bool
	if err := d.QueryRow(`SELECT EXISTS(SELECT 1 FROM findings WHERE id=$1)`, c.FindingID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errors.New("finding not found")
	}
	_, err := d.Exec(`INSERT INTO finding_classification(finding_id,cvss_vector,cvss_score,cwe,owasp,attack,remediation,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,now())
ON CONFLICT (finding_id) DO UPDATE SET cvss_vector=EXCLUDED.cvss_vector, cvss_score=EXCLUDED.cvss_score, cwe=EXCLUDED.cwe,
  owasp=EXCLUDED.owasp, attack=EXCLUDED.attack, remediation=EXCLUDED.remediation, updated_at=now()`,
		c.FindingID, c.CVSSVector, c.CVSSScore, string(cwe), string(owasp), string(attack), c.Remediation)
	return err
}

// ListFindingClassifications loads the classifications of several findings at once.
func (d *DB) ListFindingClassifications(ids []int64) (map[int64]*FindingClassification, error) {
	out := map[int64]*FindingClassification{}
	if len(ids) == 0 {
		return out, nil
	}
	idsJSON, _ := json.Marshal(ids)
	rows, err := d.Query(`SELECT `+classificationCols+` FROM finding_classification WHERE finding_id IN (SELECT jsonb_array_elements_text($1::jsonb)::bigint)`, string(idsJSON))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		c, err := scanClassification(rows)
		if err != nil {
			return nil, err
		}
		out[c.FindingID] = c
	}
	return out, rows.Err()
}
