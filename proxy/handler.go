// Package proxy implements the credential-injecting reverse proxy that
// forwards AI agent requests to upstream vendor APIs.
package proxy

import (
	"log"
	"net/http"
	"strings"

	"github.com/Pfgoriaux/clawring/crypto"
	"github.com/Pfgoriaux/clawring/db"
)

// Handler serves proxied requests, swapping phantom tokens for real vendor keys.
type Handler struct {
	DB        *db.DB
	MasterKey []byte
	Vendors   map[string]VendorConfig
	Client    *http.Client
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse: /{vendor}/remaining/path
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, `{"error":"missing vendor in path"}`, http.StatusBadRequest)
		return
	}
	vendor := parts[0]
	remaining := ""
	if len(parts) > 1 {
		remaining = "/" + parts[1]
	}

	// Sanitize the remaining path to prevent directory traversal.
	// The path is used to build the upstream URL; rejecting ".." ensures
	// an agent cannot escape the vendor's API namespace.
	if strings.Contains(remaining, "..") {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}
	if remaining != "" && !strings.HasPrefix(remaining, "/") {
		remaining = "/" + remaining
	}

	vendorCfg, ok := h.Vendors[vendor]
	if !ok {
		http.Error(w, `{"error":"unknown vendor"}`, http.StatusBadRequest)
		return
	}

	// Extract phantom token from auth headers
	phantomToken := extractToken(r, vendor)
	if phantomToken == "" {
		log.Printf("auth: missing authentication from %s for vendor %s", r.RemoteAddr, vendor)
		http.Error(w, `{"error":"missing authentication"}`, http.StatusUnauthorized)
		return
	}

	// Look up agent by token hash
	tokenHash := crypto.HashToken(phantomToken)
	agent, err := h.DB.GetAgentByTokenHash(tokenHash)
	if err != nil {
		log.Printf("auth: invalid token from %s for vendor %s", r.RemoteAddr, vendor)
		http.Error(w, `{"error":"invalid token"}`, http.StatusForbidden)
		return
	}

	// Check vendor allowlist
	if !vendorAllowed(agent.AllowedVendors, vendor) {
		http.Error(w, `{"error":"vendor not allowed for this agent"}`, http.StatusForbidden)
		return
	}

	// Get real API key
	keyID, realKey, err := h.DB.GetKeyByVendor(vendor, h.MasterKey)
	if err != nil {
		log.Printf("no key for vendor %s: %v", vendor, err)
		http.Error(w, `{"error":"no API key configured for vendor"}`, http.StatusServiceUnavailable)
		return
	}

	// Build upstream URL
	upstreamURL := "https://" + vendorCfg.UpstreamHost + remaining
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	// Forward and stream
	statusCode, err := ForwardRequest(w, r, upstreamURL, vendorCfg, realKey, h.Client)
	if err != nil {
		log.Printf("upstream error for agent %s: %v", agent.ID, err)
		// Only write error if headers haven't been sent
		if statusCode == 0 {
			http.Error(w, `{"error":"upstream request failed"}`, http.StatusBadGateway)
			statusCode = http.StatusBadGateway
		}
	}

	// Log usage
	if err := h.DB.LogUsage(agent.ID, vendor, keyID, statusCode); err != nil {
		log.Printf("usage log write error: %v", err)
	}
}

// extractToken gets the phantom token from the appropriate header based on vendor.
func extractToken(r *http.Request, vendor string) string {
	// Anthropic sends x-api-key
	if vendor == "anthropic" {
		if v := r.Header.Get("X-Api-Key"); v != "" {
			return v
		}
	}
	// OpenAI (and fallback) sends Authorization: Bearer
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func vendorAllowed(allowed []string, vendor string) bool {
	for _, v := range allowed {
		if v == vendor {
			return true
		}
	}
	return false
}
