package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pfgoriaux/clawring/db"
	"github.com/Pfgoriaux/clawring/proxy"
)

func setupTestAdmin(t *testing.T) *Handler {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	masterKey := make([]byte, 32)
	for i := range masterKey {
		masterKey[i] = byte(i)
	}

	return &Handler{
		DB:         database,
		MasterKey:  masterKey,
		AdminToken: "test-admin-token",
		Vendors:    proxy.DefaultVendors(),
	}
}

func TestHealthNoAuth(t *testing.T) {
	h := setupTestAdmin(t)
	req := httptest.NewRequest("GET", "/admin/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHealthMethodNotAllowed(t *testing.T) {
	h := setupTestAdmin(t)
	req := httptest.NewRequest("POST", "/admin/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestUnauthorized(t *testing.T) {
	h := setupTestAdmin(t)
	req := httptest.NewRequest("GET", "/admin/keys", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestWrongToken(t *testing.T) {
	h := setupTestAdmin(t)
	req := httptest.NewRequest("GET", "/admin/keys", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestKeysCRUD(t *testing.T) {
	h := setupTestAdmin(t)

	// Add key
	body := `{"vendor":"anthropic","secret":"sk-test-123","label":"main"}`
	req := httptest.NewRequest("POST", "/admin/keys", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-admin-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("add key: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var addResp map[string]string
	json.NewDecoder(w.Body).Decode(&addResp)
	keyID := addResp["id"]
	if keyID == "" {
		t.Fatal("expected key id in response")
	}

	// List keys
	req = httptest.NewRequest("GET", "/admin/keys", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("list keys: expected 200, got %d", w.Code)
	}

	var keys []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&keys)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if _, hasSecret := keys[0]["secret"]; hasSecret {
		t.Error("secret should not be in list response")
	}

	// Delete key
	req = httptest.NewRequest("DELETE", "/admin/keys/"+keyID, nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("delete key: expected 200, got %d", w.Code)
	}
}

func TestAgentsCRUD(t *testing.T) {
	h := setupTestAdmin(t)

	// Add agent
	body := `{"hostname":"agent-1","allowed_vendors":["anthropic","openai"]}`
	req := httptest.NewRequest("POST", "/admin/agents", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-admin-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("add agent: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var agentResp map[string]string
	json.NewDecoder(w.Body).Decode(&agentResp)
	if agentResp["id"] == "" || agentResp["token"] == "" {
		t.Fatal("expected id and token in response")
	}

	// List agents
	req = httptest.NewRequest("GET", "/admin/agents", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("list agents: expected 200, got %d", w.Code)
	}

	var agents []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&agents)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	// Delete agent
	req = httptest.NewRequest("DELETE", "/admin/agents/"+agentResp["id"], nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("delete agent: expected 200, got %d", w.Code)
	}
}

func TestAddKeyMissingFields(t *testing.T) {
	h := setupTestAdmin(t)

	body := `{"vendor":"anthropic"}`
	req := httptest.NewRequest("POST", "/admin/keys", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-admin-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAddAgentMissingHostname(t *testing.T) {
	h := setupTestAdmin(t)

	body := `{"allowed_vendors":["anthropic"]}`
	req := httptest.NewRequest("POST", "/admin/agents", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-admin-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAddAgentUnknownVendor(t *testing.T) {
	h := setupTestAdmin(t)

	body := `{"hostname":"agent-bad","allowed_vendors":["unknown-vendor"]}`
	req := httptest.NewRequest("POST", "/admin/agents", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-admin-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown vendor, got %d", w.Code)
	}
}

func TestDeleteKeyWithSlashInID(t *testing.T) {
	h := setupTestAdmin(t)

	req := httptest.NewRequest("DELETE", "/admin/keys/abc/extra", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for slash in id, got %d", w.Code)
	}
}

func TestAddKeyUnknownVendor(t *testing.T) {
	h := setupTestAdmin(t)

	body := `{"vendor":"unknown-vendor","secret":"sk-test"}`
	req := httptest.NewRequest("POST", "/admin/keys", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-admin-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown vendor in addKey, got %d", w.Code)
	}
}

func TestRotateAgentToken(t *testing.T) {
	h := setupTestAdmin(t)

	// First, create an agent
	body := `{"hostname":"rotate-test","allowed_vendors":["anthropic"]}`
	req := httptest.NewRequest("POST", "/admin/agents", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-admin-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("add agent: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var agentResp map[string]string
	json.NewDecoder(w.Body).Decode(&agentResp)
	agentID := agentResp["id"]
	oldToken := agentResp["token"]

	// Rotate token
	req = httptest.NewRequest("POST", "/admin/agents/"+agentID+"/rotate", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("rotate token: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var rotateResp map[string]string
	json.NewDecoder(w.Body).Decode(&rotateResp)
	newToken := rotateResp["token"]

	if newToken == "" {
		t.Fatal("expected new token in response")
	}
	if newToken == oldToken {
		t.Error("new token should differ from old token")
	}
	if rotateResp["id"] != agentID {
		t.Errorf("expected id %q, got %q", agentID, rotateResp["id"])
	}
}

func TestRotateAgentTokenNotFound(t *testing.T) {
	h := setupTestAdmin(t)

	req := httptest.NewRequest("POST", "/admin/agents/nonexistent/rotate", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRotateAgentTokenRequiresAuth(t *testing.T) {
	h := setupTestAdmin(t)

	req := httptest.NewRequest("POST", "/admin/agents/some-id/rotate", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAddKeyMissingContentType(t *testing.T) {
	h := setupTestAdmin(t)

	body := `{"vendor":"anthropic","secret":"sk-test-123","label":"main"}`
	req := httptest.NewRequest("POST", "/admin/keys", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-admin-token")
	// No Content-Type header
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d", w.Code)
	}
}

func TestAddAgentMissingContentType(t *testing.T) {
	h := setupTestAdmin(t)

	body := `{"hostname":"agent-1","allowed_vendors":["anthropic"]}`
	req := httptest.NewRequest("POST", "/admin/agents", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-admin-token")
	// No Content-Type header
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d", w.Code)
	}
}
