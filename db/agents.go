package db

import (
	"encoding/json"
	"fmt"

	"github.com/Pfgoriaux/clawring/crypto"
)

type Agent struct {
	ID             string   `json:"id"`
	Hostname       string   `json:"hostname"`
	AllowedVendors []string `json:"allowed_vendors"`
	CreatedAt      int64    `json:"created_at"`
}

// AddAgent registers an agent and returns its phantom token (shown once).
func (d *DB) AddAgent(hostname string, allowedVendors []string) (id, token string, err error) {
	token, err = crypto.GenerateToken()
	if err != nil {
		return "", "", err
	}
	id, err = crypto.GenerateToken()
	if err != nil {
		return "", "", err
	}
	id = id[:16]

	tokenHash := crypto.HashToken(token)
	vendorsJSON, err := json.Marshal(allowedVendors)
	if err != nil {
		return "", "", fmt.Errorf("marshaling vendors: %w", err)
	}

	_, err = d.conn.Exec(
		"INSERT INTO agents (id, hostname, token_hash, allowed_vendors) VALUES (?, ?, ?, ?)",
		id, hostname, tokenHash, string(vendorsJSON),
	)
	if err != nil {
		return "", "", fmt.Errorf("inserting agent: %w", err)
	}
	return id, token, nil
}

// GetAgentByTokenHash looks up an agent by the SHA-256 hash of its phantom token.
func (d *DB) GetAgentByTokenHash(tokenHash string) (*Agent, error) {
	var a Agent
	var vendorsJSON string
	err := d.conn.QueryRow(
		"SELECT id, hostname, allowed_vendors, created_at FROM agents WHERE token_hash = ?",
		tokenHash,
	).Scan(&a.ID, &a.Hostname, &vendorsJSON, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("querying agent: %w", err)
	}
	if err := json.Unmarshal([]byte(vendorsJSON), &a.AllowedVendors); err != nil {
		return nil, fmt.Errorf("parsing allowed vendors: %w", err)
	}
	return &a, nil
}

func (d *DB) ListAgents() ([]Agent, error) {
	rows, err := d.conn.Query("SELECT id, hostname, allowed_vendors, created_at FROM agents ORDER BY created_at DESC")
	if err != nil {
		return nil, fmt.Errorf("listing agents: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		var vendorsJSON string
		if err := rows.Scan(&a.ID, &a.Hostname, &vendorsJSON, &a.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(vendorsJSON), &a.AllowedVendors); err != nil {
			continue
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// RotateAgentToken generates a new phantom token for an existing agent.
func (d *DB) RotateAgentToken(id string) (string, error) {
	token, err := crypto.GenerateToken()
	if err != nil {
		return "", err
	}
	tokenHash := crypto.HashToken(token)
	res, err := d.conn.Exec("UPDATE agents SET token_hash = ? WHERE id = ?", tokenHash, id)
	if err != nil {
		return "", fmt.Errorf("rotating token: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", ErrNotFound
	}
	return token, nil
}

func (d *DB) DeleteAgent(id string) error {
	res, err := d.conn.Exec("DELETE FROM agents WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting agent: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
