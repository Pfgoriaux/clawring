package db

import (
	"fmt"
)

type UsageEntry struct {
	ID         int64  `json:"id"`
	AgentID    string `json:"agent_id"`
	Vendor     string `json:"vendor"`
	KeyID      string `json:"key_id"`
	StatusCode int    `json:"status_code"`
	CreatedAt  int64  `json:"created_at"`
}

func (d *DB) LogUsage(agentID, vendor, keyID string, statusCode int) error {
	_, err := d.conn.Exec(
		"INSERT INTO usage_log (agent_id, vendor, key_id, status_code) VALUES (?, ?, ?, ?)",
		agentID, vendor, keyID, statusCode,
	)
	return err
}

func (d *DB) GetUsageByAgent(agentID string, limit int) ([]UsageEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := d.conn.Query(
		"SELECT id, agent_id, vendor, key_id, status_code, created_at FROM usage_log WHERE agent_id = ? ORDER BY created_at DESC LIMIT ?",
		agentID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying usage: %w", err)
	}
	defer rows.Close()

	var entries []UsageEntry
	for rows.Next() {
		var e UsageEntry
		if err := rows.Scan(&e.ID, &e.AgentID, &e.Vendor, &e.KeyID, &e.StatusCode, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// PruneUsage deletes usage entries older than the given number of days.
func (d *DB) PruneUsage(retentionDays int) (int64, error) {
	res, err := d.conn.Exec(
		"DELETE FROM usage_log WHERE created_at < unixepoch() - ?",
		retentionDays*86400,
	)
	if err != nil {
		return 0, fmt.Errorf("pruning usage: %w", err)
	}
	return res.RowsAffected()
}
