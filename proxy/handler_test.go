package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pfgoriaux/clawring/db"
)

func setupTestDB(t *testing.T) (*db.DB, []byte) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	masterKey := make([]byte, 32)
	for i := range masterKey {
		masterKey[i] = byte(i)
	}
	return database, masterKey
}

func TestInvalidVendor(t *testing.T) {
	database, masterKey := setupTestDB(t)
	h := &Handler{
		DB:        database,
		MasterKey: masterKey,
		Vendors:   DefaultVendors(),
		Client:    http.DefaultClient,
	}

	req := httptest.NewRequest("POST", "/unknown/v1/chat", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestMissingAuth(t *testing.T) {
	database, masterKey := setupTestDB(t)
	h := &Handler{
		DB:        database,
		MasterKey: masterKey,
		Vendors:   DefaultVendors(),
		Client:    http.DefaultClient,
	}

	req := httptest.NewRequest("POST", "/anthropic/v1/messages", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestInvalidToken(t *testing.T) {
	database, masterKey := setupTestDB(t)
	h := &Handler{
		DB:        database,
		MasterKey: masterKey,
		Vendors:   DefaultVendors(),
		Client:    http.DefaultClient,
	}

	req := httptest.NewRequest("POST", "/anthropic/v1/messages", nil)
	req.Header.Set("X-Api-Key", "invalid-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestVendorNotAllowed(t *testing.T) {
	database, masterKey := setupTestDB(t)

	// Register agent allowed only for openai
	_, token, err := database.AddAgent("test-host", []string{"openai"})
	if err != nil {
		t.Fatal(err)
	}

	h := &Handler{
		DB:        database,
		MasterKey: masterKey,
		Vendors:   DefaultVendors(),
		Client:    http.DefaultClient,
	}
	req := httptest.NewRequest("POST", "/anthropic/v1/messages", nil)
	req.Header.Set("X-Api-Key", token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestPathTraversal(t *testing.T) {
	database, masterKey := setupTestDB(t)
	h := &Handler{
		DB:        database,
		MasterKey: masterKey,
		Vendors:   DefaultVendors(),
		Client:    http.DefaultClient,
	}

	req := httptest.NewRequest("POST", "/anthropic/v1/../../../etc/passwd", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for path traversal, got %d", w.Code)
	}
}

func TestKeySwapAndForward(t *testing.T) {
	database, masterKey := setupTestDB(t)

	// Register agent and add key
	_, token, _ := database.AddAgent("test-host", []string{"anthropic"})
	database.AddKey("anthropic", "sk-real-key-123", "test", masterKey)

	// Mock upstream
	var receivedAuthHeader string
	var receivedPath string
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("X-Api-Key")
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"msg_123","content":[{"text":"Hello"}]}`)
	}))
	defer upstream.Close()

	h := &Handler{
		DB:        database,
		MasterKey: masterKey,
		Vendors: map[string]VendorConfig{
			"anthropic": {
				UpstreamHost: strings.TrimPrefix(upstream.URL, "https://"),
				AuthHeader:   "x-api-key",
				AuthFormat:   func(key string) string { return key },
			},
		},
		Client: upstream.Client(),
	}

	req := httptest.NewRequest("POST", "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-3-5-sonnet","messages":[]}`))
	req.Header.Set("X-Api-Key", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify real key was injected
	if receivedAuthHeader != "sk-real-key-123" {
		t.Errorf("expected real key in upstream, got %q", receivedAuthHeader)
	}

	// Verify path forwarded correctly
	if receivedPath != "/v1/messages" {
		t.Errorf("expected /v1/messages, got %q", receivedPath)
	}
}

func TestSSEStreaming(t *testing.T) {
	database, masterKey := setupTestDB(t)
	_, token, _ := database.AddAgent("test-host-sse", []string{"anthropic"})
	database.AddKey("anthropic", "sk-real-key", "test", masterKey)

	// Mock upstream that sends SSE
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: {\"chunk\":%d}\n\n", i)
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	h := &Handler{
		DB:        database,
		MasterKey: masterKey,
		Vendors: map[string]VendorConfig{
			"anthropic": {
				UpstreamHost: strings.TrimPrefix(upstream.URL, "https://"),
				AuthHeader:   "x-api-key",
				AuthFormat:   func(key string) string { return key },
			},
		},
		Client: upstream.Client(),
	}

	req := httptest.NewRequest("POST", "/anthropic/v1/messages", strings.NewReader(`{}`))
	req.Header.Set("X-Api-Key", token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"chunk":0`) || !strings.Contains(body, `"chunk":2`) {
		t.Errorf("expected SSE chunks in body, got %q", body)
	}
}

func TestOpenAIBearerAuth(t *testing.T) {
	database, masterKey := setupTestDB(t)
	_, token, _ := database.AddAgent("test-host-openai", []string{"openai"})
	database.AddKey("openai", "sk-openai-real", "test", masterKey)

	var receivedAuth string
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"choices":[]}`)
	}))
	defer upstream.Close()

	h := &Handler{
		DB:        database,
		MasterKey: masterKey,
		Vendors: map[string]VendorConfig{
			"openai": {
				UpstreamHost: strings.TrimPrefix(upstream.URL, "https://"),
				AuthHeader:   "Authorization",
				AuthFormat:   func(key string) string { return "Bearer " + key },
			},
		},
		Client: upstream.Client(),
	}

	req := httptest.NewRequest("POST", "/openai/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if receivedAuth != "Bearer sk-openai-real" {
		t.Errorf("expected Bearer auth with real key, got %q", receivedAuth)
	}
}
