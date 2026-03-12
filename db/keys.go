package db

import (
	"fmt"

	"github.com/Pfgoriaux/clawring/crypto"
)

type ProviderKey struct {
	ID        string `json:"id"`
	Vendor    string `json:"vendor"`
	Label     string `json:"label"`
	CreatedAt int64  `json:"created_at"`
}

func (d *DB) AddKey(vendor, secret, label string, masterKey []byte) (string, error) {
	id, err := crypto.GenerateToken()
	if err != nil {
		return "", err
	}
	id = id[:16] // shorter ID is fine for keys

	encrypted, err := crypto.Encrypt(secret, masterKey)
	if err != nil {
		return "", fmt.Errorf("encrypting key: %w", err)
	}

	_, err = d.conn.Exec(
		"INSERT INTO provider_keys (id, vendor, secret_encrypted, label) VALUES (?, ?, ?, ?)",
		id, vendor, encrypted, label,
	)
	if err != nil {
		return "", fmt.Errorf("inserting key: %w", err)
	}
	return id, nil
}

func (d *DB) GetKeyByVendor(vendor string, masterKey []byte) (string, string, error) {
	var id string
	var encrypted []byte
	err := d.conn.QueryRow(
		"SELECT id, secret_encrypted FROM provider_keys WHERE vendor = ? ORDER BY created_at DESC LIMIT 1",
		vendor,
	).Scan(&id, &encrypted)
	if err != nil {
		return "", "", fmt.Errorf("querying key for vendor %s: %w", vendor, err)
	}
	secret, err := crypto.Decrypt(encrypted, masterKey)
	if err != nil {
		return "", "", fmt.Errorf("decrypting key: %w", err)
	}
	return id, secret, nil
}

func (d *DB) ListKeys() ([]ProviderKey, error) {
	rows, err := d.conn.Query("SELECT id, vendor, label, created_at FROM provider_keys ORDER BY created_at DESC")
	if err != nil {
		return nil, fmt.Errorf("listing keys: %w", err)
	}
	defer rows.Close()

	var keys []ProviderKey
	for rows.Next() {
		var k ProviderKey
		if err := rows.Scan(&k.ID, &k.Vendor, &k.Label, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (d *DB) DeleteKey(id string) error {
	res, err := d.conn.Exec("DELETE FROM provider_keys WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting key: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
