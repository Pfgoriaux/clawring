package db

import (
	"path/filepath"
	"testing"

	"github.com/Pfgoriaux/clawring/crypto"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func testMasterKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

func TestOpenAndMigrate(t *testing.T) {
	d := setupTestDB(t)

	// Verify tables exist by running queries
	var count int
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM provider_keys").Scan(&count); err != nil {
		t.Fatalf("provider_keys table missing: %v", err)
	}
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM agents").Scan(&count); err != nil {
		t.Fatalf("agents table missing: %v", err)
	}
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM usage_log").Scan(&count); err != nil {
		t.Fatalf("usage_log table missing: %v", err)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	d := setupTestDB(t)

	// Running migrate again should not error
	if err := d.migrate(); err != nil {
		t.Fatalf("second migrate should be idempotent: %v", err)
	}
}

func TestKeyRoundTrip(t *testing.T) {
	d := setupTestDB(t)
	masterKey := testMasterKey()

	id, err := d.AddKey("anthropic", "sk-secret-123", "main key", masterKey)
	if err != nil {
		t.Fatalf("AddKey: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty key id")
	}

	gotID, secret, err := d.GetKeyByVendor("anthropic", masterKey)
	if err != nil {
		t.Fatalf("GetKeyByVendor: %v", err)
	}
	if gotID != id {
		t.Errorf("expected id %q, got %q", id, gotID)
	}
	if secret != "sk-secret-123" {
		t.Errorf("expected secret 'sk-secret-123', got %q", secret)
	}
}

func TestGetKeyByVendorReturnsNewest(t *testing.T) {
	d := setupTestDB(t)
	masterKey := testMasterKey()

	id1, _ := d.AddKey("anthropic", "old-key", "old", masterKey)
	id2, _ := d.AddKey("anthropic", "new-key", "new", masterKey)

	// Manually backdate the first key so ORDER BY created_at DESC is deterministic
	d.conn.Exec("UPDATE provider_keys SET created_at = created_at - 100 WHERE id = ?", id1)

	gotID, secret, err := d.GetKeyByVendor("anthropic", masterKey)
	if err != nil {
		t.Fatalf("GetKeyByVendor: %v", err)
	}
	if gotID != id2 {
		t.Errorf("expected newest key id %q, got %q", id2, gotID)
	}
	if secret != "new-key" {
		t.Errorf("expected 'new-key', got %q", secret)
	}
}

func TestGetKeyByVendorNotFound(t *testing.T) {
	d := setupTestDB(t)
	masterKey := testMasterKey()

	_, _, err := d.GetKeyByVendor("nonexistent", masterKey)
	if err == nil {
		t.Fatal("expected error for nonexistent vendor")
	}
}

func TestListKeysEmpty(t *testing.T) {
	d := setupTestDB(t)

	keys, err := d.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if keys != nil {
		t.Errorf("expected nil for empty list, got %v", keys)
	}
}

func TestDeleteKeyNotFound(t *testing.T) {
	d := setupTestDB(t)

	err := d.DeleteKey("nonexistent")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent key")
	}
}

func TestAgentRoundTrip(t *testing.T) {
	d := setupTestDB(t)

	id, token, err := d.AddAgent("test-host", []string{"anthropic", "openai"})
	if err != nil {
		t.Fatalf("AddAgent: %v", err)
	}
	if id == "" || token == "" {
		t.Fatal("expected non-empty id and token")
	}

	tokenHash := crypto.HashToken(token)
	agent, err := d.GetAgentByTokenHash(tokenHash)
	if err != nil {
		t.Fatalf("GetAgentByTokenHash: %v", err)
	}
	if agent.ID != id {
		t.Errorf("expected id %q, got %q", id, agent.ID)
	}
	if agent.Hostname != "test-host" {
		t.Errorf("expected hostname 'test-host', got %q", agent.Hostname)
	}
	if len(agent.AllowedVendors) != 2 {
		t.Errorf("expected 2 allowed vendors, got %d", len(agent.AllowedVendors))
	}
}

func TestGetAgentByTokenHashNotFound(t *testing.T) {
	d := setupTestDB(t)

	_, err := d.GetAgentByTokenHash("nonexistent-hash")
	if err == nil {
		t.Fatal("expected error for nonexistent token hash")
	}
}

func TestListAgentsEmpty(t *testing.T) {
	d := setupTestDB(t)

	agents, err := d.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if agents != nil {
		t.Errorf("expected nil for empty list, got %v", agents)
	}
}

func TestDeleteAgentNotFound(t *testing.T) {
	d := setupTestDB(t)

	err := d.DeleteAgent("nonexistent")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent agent")
	}
}

func TestRotateAgentToken(t *testing.T) {
	d := setupTestDB(t)

	id, oldToken, err := d.AddAgent("test-host", []string{"anthropic"})
	if err != nil {
		t.Fatalf("AddAgent: %v", err)
	}

	newToken, err := d.RotateAgentToken(id)
	if err != nil {
		t.Fatalf("RotateAgentToken: %v", err)
	}
	if newToken == "" {
		t.Fatal("expected non-empty new token")
	}
	if newToken == oldToken {
		t.Error("new token should differ from old token")
	}

	// Old token should no longer work
	oldHash := crypto.HashToken(oldToken)
	_, err = d.GetAgentByTokenHash(oldHash)
	if err == nil {
		t.Error("old token should no longer resolve")
	}

	// New token should work
	newHash := crypto.HashToken(newToken)
	agent, err := d.GetAgentByTokenHash(newHash)
	if err != nil {
		t.Fatalf("new token should resolve: %v", err)
	}
	if agent.ID != id {
		t.Errorf("expected agent id %q, got %q", id, agent.ID)
	}
}

func TestRotateAgentTokenNotFound(t *testing.T) {
	d := setupTestDB(t)

	_, err := d.RotateAgentToken("nonexistent")
	if err == nil {
		t.Fatal("expected error for rotating nonexistent agent")
	}
}

func TestLogAndGetUsage(t *testing.T) {
	d := setupTestDB(t)

	// Insert with explicit timestamps to ensure deterministic ordering
	d.conn.Exec(
		"INSERT INTO usage_log (agent_id, vendor, key_id, status_code, created_at) VALUES (?, ?, ?, ?, ?)",
		"agent-1", "anthropic", "key-1", 200, 1000,
	)
	d.conn.Exec(
		"INSERT INTO usage_log (agent_id, vendor, key_id, status_code, created_at) VALUES (?, ?, ?, ?, ?)",
		"agent-1", "openai", "key-2", 200, 2000,
	)
	d.conn.Exec(
		"INSERT INTO usage_log (agent_id, vendor, key_id, status_code, created_at) VALUES (?, ?, ?, ?, ?)",
		"agent-2", "anthropic", "key-1", 500, 3000,
	)

	entries, err := d.GetUsageByAgent("agent-1", 10)
	if err != nil {
		t.Fatalf("GetUsageByAgent: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for agent-1, got %d", len(entries))
	}
	// Should be ordered by created_at DESC
	if entries[0].Vendor != "openai" {
		t.Errorf("expected most recent entry first (openai), got %q", entries[0].Vendor)
	}
}

func TestGetUsageLimitClamping(t *testing.T) {
	d := setupTestDB(t)

	// Limit <= 0 defaults to 100
	entries, err := d.GetUsageByAgent("agent-1", 0)
	if err != nil {
		t.Fatalf("GetUsageByAgent: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil for empty results, got %v", entries)
	}

	// Limit > 1000 is clamped to 1000
	entries, err = d.GetUsageByAgent("agent-1", 5000)
	if err != nil {
		t.Fatalf("GetUsageByAgent: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil for empty results, got %v", entries)
	}
}

func TestPruneUsage(t *testing.T) {
	d := setupTestDB(t)

	// Insert an old entry manually
	_, err := d.conn.Exec(
		"INSERT INTO usage_log (agent_id, vendor, key_id, status_code, created_at) VALUES (?, ?, ?, ?, ?)",
		"agent-1", "anthropic", "key-1", 200, 1000000, // epoch 1970
	)
	if err != nil {
		t.Fatal(err)
	}

	// Insert a recent entry
	if err := d.LogUsage("agent-1", "anthropic", "key-1", 200); err != nil {
		t.Fatalf("LogUsage: %v", err)
	}

	n, err := d.PruneUsage(30)
	if err != nil {
		t.Fatalf("PruneUsage: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 pruned entry, got %d", n)
	}

	// Recent entry should still exist
	entries, err := d.GetUsageByAgent("agent-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 remaining entry, got %d", len(entries))
	}
}
