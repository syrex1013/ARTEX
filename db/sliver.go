package db

import (
	"database/sql"
	"errors"
	"time"
)

// ErrSliverConfigNotFound is returned when a Sliver config doesn't exist.
var ErrSliverConfigNotFound = errors.New("sliver config not found")

// SliverConfig holds connection details for a Sliver C2 server.
type SliverConfig struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Host       string     `json:"host"`
	Port       int        `json:"port"`
	Operator   string     `json:"operator"`
	CACert     string     `json:"ca_cert,omitempty"`
	ClientCert string     `json:"client_cert,omitempty"`
	ClientKey  string     `json:"client_key,omitempty"` // never sent to client
	Token      string     `json:"token,omitempty"`
	Enabled    bool       `json:"enabled"`
	LastSeen   *time.Time `json:"last_seen,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (d *DB) scanSliverConfig(row interface{ Scan(...any) error }) (*SliverConfig, error) {
	var c SliverConfig
	var lastSeen sql.NullTime
	var caCert, clientCert, clientKey, token sql.NullString
	err := row.Scan(&c.ID, &c.Name, &c.Host, &c.Port, &c.Operator, &caCert, &clientCert, &clientKey, &token, &c.Enabled, &lastSeen, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	c.CACert = caCert.String
	c.ClientCert = clientCert.String
	c.ClientKey = ""   // never expose key
	c.Token = token.String
	if lastSeen.Valid {
		c.LastSeen = &lastSeen.Time
	}
	return &c, nil
}

const sliverCols = `id, name, host, port, operator, ca_cert, client_cert, client_key, token, enabled, last_seen, created_at`

// CreateSliverConfig adds a new Sliver server config.
func (d *DB) CreateSliverConfig(c SliverConfig) (*SliverConfig, error) {
	if c.Name == "" || c.Host == "" {
		return nil, errors.New("name and host are required")
	}
	if c.Port == 0 {
		c.Port = 31337
	}
	r := d.QueryRow(`INSERT INTO sliver_configs(name,host,port,operator,ca_cert,client_cert,client_key,token) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+sliverCols,
		c.Name, c.Host, c.Port, c.Operator, c.CACert, c.ClientCert, c.ClientKey, c.Token)
	return d.scanSliverConfig(r)
}

// GetSliverConfig returns one config by id.
func (d *DB) GetSliverConfig(id int64) (*SliverConfig, error) {
	c, err := d.scanSliverConfig(d.QueryRow(`SELECT `+sliverCols+` FROM sliver_configs WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSliverConfigNotFound
	}
	return c, err
}

// GetSliverConfigByName fetches a config including the client key (for internal use).
func (d *DB) GetSliverConfigByName(name string) (*SliverConfig, string, error) {
	var c SliverConfig
	var lastSeen sql.NullTime
	var key string
	err := d.QueryRow(`SELECT id,name,host,port,operator,ca_cert,client_cert,client_key,token,enabled,last_seen,created_at FROM sliver_configs WHERE name=$1 AND enabled=true`, name).
		Scan(&c.ID, &c.Name, &c.Host, &c.Port, &c.Operator, &c.CACert, &c.ClientCert, &key, &c.Token, &c.Enabled, &lastSeen, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrSliverConfigNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if lastSeen.Valid {
		c.LastSeen = &lastSeen.Time
	}
	return &c, key, nil
}

// ListSliverConfigs returns all configs (keys excluded).
func (d *DB) ListSliverConfigs() ([]*SliverConfig, error) {
	rows, err := d.Query(`SELECT ` + sliverCols + ` FROM sliver_configs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SliverConfig
	for rows.Next() {
		c, err := d.scanSliverConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteSliverConfig removes a config.
func (d *DB) DeleteSliverConfig(id int64) error {
	res, err := d.Exec(`DELETE FROM sliver_configs WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSliverConfigNotFound
	}
	return nil
}

// UpdateSliverLastSeen updates the last_seen timestamp.
func (d *DB) UpdateSliverLastSeen(id int64) error {
	_, err := d.Exec(`UPDATE sliver_configs SET last_seen=now() WHERE id=$1`, id)
	return err
}
