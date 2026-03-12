// Package admin implements the administrative API for managing agents and
// provider keys in the openclaw proxy.
package admin

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/Pfgoriaux/clawring/db"
	"github.com/Pfgoriaux/clawring/proxy"
)

const maxBodySize = 1 << 20 // 1 MB

type Handler struct {
	DB         *db.DB
	MasterKey  []byte
	AdminToken string
	Vendors    map[string]proxy.VendorConfig
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Health check — no auth required, GET/HEAD only
	if r.URL.Path == "/admin/health" {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	// Validate admin token
	if !h.checkAuth(r) {
		log.Printf("auth: failed admin authentication from %s", r.RemoteAddr)
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	switch {
	case r.URL.Path == "/admin/keys" && r.Method == http.MethodPost:
		h.addKey(w, r)
	case r.URL.Path == "/admin/keys" && r.Method == http.MethodGet:
		h.listKeys(w, r)
	case strings.HasPrefix(r.URL.Path, "/admin/keys/") && r.Method == http.MethodDelete:
		h.deleteKey(w, r)
	case r.URL.Path == "/admin/agents" && r.Method == http.MethodPost:
		h.addAgent(w, r)
	case r.URL.Path == "/admin/agents" && r.Method == http.MethodGet:
		h.listAgents(w, r)
	case strings.HasPrefix(r.URL.Path, "/admin/agents/") && strings.HasSuffix(r.URL.Path, "/rotate") && r.Method == http.MethodPost:
		h.rotateAgentToken(w, r)
	case strings.HasPrefix(r.URL.Path, "/admin/agents/") && r.Method == http.MethodDelete:
		h.deleteAgent(w, r)
	default:
		writeError(w, "not found", http.StatusNotFound)
	}
}

func (h *Handler) checkAuth(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == auth { // no "Bearer " prefix
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(h.AdminToken)) == 1
}

func writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// requireJSON checks that the request Content-Type is application/json.
// Returns true if valid, false (and writes 415 error) if not.
func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		writeError(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return false
	}
	return true
}

type addKeyRequest struct {
	Vendor string `json:"vendor"`
	Secret string `json:"secret"`
	Label  string `json:"label"`
}

func (h *Handler) addKey(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	var req addKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Vendor == "" || req.Secret == "" {
		writeError(w, "vendor and secret are required", http.StatusBadRequest)
		return
	}
	if _, ok := h.Vendors[req.Vendor]; !ok {
		writeError(w, "unknown vendor: "+req.Vendor, http.StatusBadRequest)
		return
	}

	id, err := h.DB.AddKey(req.Vendor, req.Secret, req.Label, h.MasterKey)
	if err != nil {
		log.Printf("add key error: %v", err)
		writeError(w, "failed to add key", http.StatusInternalServerError)
		return
	}
	log.Printf("audit: key %s added for vendor %s (label=%s)", id, req.Vendor, req.Label)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func (h *Handler) listKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.DB.ListKeys()
	if err != nil {
		log.Printf("list keys error: %v", err)
		writeError(w, "failed to list keys", http.StatusInternalServerError)
		return
	}
	if keys == nil {
		keys = []db.ProviderKey{}
	}
	json.NewEncoder(w).Encode(keys)
}

func (h *Handler) deleteKey(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/admin/keys/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, "invalid key id", http.StatusBadRequest)
		return
	}
	if err := h.DB.DeleteKey(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, "key not found", http.StatusNotFound)
		} else {
			log.Printf("delete key error: %v", err)
			writeError(w, "failed to delete key", http.StatusInternalServerError)
		}
		return
	}
	log.Printf("audit: key %s deleted", id)
	json.NewEncoder(w).Encode(map[string]string{"deleted": id})
}

type addAgentRequest struct {
	Hostname       string   `json:"hostname"`
	AllowedVendors []string `json:"allowed_vendors"`
}

func (h *Handler) addAgent(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	var req addAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Hostname == "" {
		writeError(w, "hostname is required", http.StatusBadRequest)
		return
	}
	if len(req.AllowedVendors) == 0 {
		writeError(w, "allowed_vendors is required", http.StatusBadRequest)
		return
	}
	// Validate vendor names
	for _, v := range req.AllowedVendors {
		if _, ok := h.Vendors[v]; !ok {
			writeError(w, "unknown vendor: "+v, http.StatusBadRequest)
			return
		}
	}

	id, token, err := h.DB.AddAgent(req.Hostname, req.AllowedVendors)
	if err != nil {
		log.Printf("add agent error: %v", err)
		writeError(w, "failed to add agent", http.StatusInternalServerError)
		return
	}
	log.Printf("audit: agent %s registered (hostname=%s, vendors=%v)", id, req.Hostname, req.AllowedVendors)
	json.NewEncoder(w).Encode(map[string]string{
		"id":    id,
		"token": token,
	})
}

func (h *Handler) listAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.DB.ListAgents()
	if err != nil {
		log.Printf("list agents error: %v", err)
		writeError(w, "failed to list agents", http.StatusInternalServerError)
		return
	}
	if agents == nil {
		agents = []db.Agent{}
	}
	json.NewEncoder(w).Encode(agents)
}

func (h *Handler) rotateAgentToken(w http.ResponseWriter, r *http.Request) {
	// /admin/agents/{id}/rotate
	trimmed := strings.TrimPrefix(r.URL.Path, "/admin/agents/")
	id := strings.TrimSuffix(trimmed, "/rotate")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, "invalid agent id", http.StatusBadRequest)
		return
	}
	token, err := h.DB.RotateAgentToken(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, "agent not found", http.StatusNotFound)
		} else {
			log.Printf("rotate token error: %v", err)
			writeError(w, "failed to rotate token", http.StatusInternalServerError)
		}
		return
	}
	log.Printf("audit: token rotated for agent %s", id)
	json.NewEncoder(w).Encode(map[string]string{
		"id":    id,
		"token": token,
	})
}

func (h *Handler) deleteAgent(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/admin/agents/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, "invalid agent id", http.StatusBadRequest)
		return
	}
	if err := h.DB.DeleteAgent(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, "agent not found", http.StatusNotFound)
		} else {
			log.Printf("delete agent error: %v", err)
			writeError(w, "failed to delete agent", http.StatusInternalServerError)
		}
		return
	}
	log.Printf("audit: agent %s deleted", id)
	json.NewEncoder(w).Encode(map[string]string{"deleted": id})
}
