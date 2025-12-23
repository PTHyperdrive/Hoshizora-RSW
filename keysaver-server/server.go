package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// Server handles HTTP requests
type Server struct {
	storage *Storage
	cfg     *Config
}

// NewServer creates a new server instance
func NewServer(storage *Storage, cfg *Config) *Server {
	return &Server{
		storage: storage,
		cfg:     cfg,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Handler returns the HTTP handler with all routes
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", s.handleHealth)

	// Key operations
	mux.HandleFunc("/keys/save", s.handleSaveKey)
	mux.HandleFunc("/keys/get", s.handleGetKey)
	mux.HandleFunc("/keys/list", s.handleListKeys)
	mux.HandleFunc("/keys/delete", s.handleDeleteKey)

	// Wrap with auth middleware
	return AuthMiddleware(s.cfg.AuthTokens, mux)
}

// GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "keysaver-server",
	})
}

// POST /keys/save
func (s *Server) handleSaveKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req SaveKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, SaveKeyResponse{
			Status:  "error",
			Message: "invalid JSON: " + err.Error(),
		})
		return
	}

	// Validate
	if req.FileHash == "" || req.KeyB64 == "" || req.NodeID == "" {
		writeJSON(w, http.StatusBadRequest, SaveKeyResponse{
			Status:  "error",
			Message: "missing required fields: hash, key_b64, node_id",
		})
		return
	}

	// Save
	if err := s.storage.SaveKey(req.FileHash, req.NodeID, req.KeyB64, req.FileName); err != nil {
		log.Printf("[save] error: %v", err)
		writeJSON(w, http.StatusInternalServerError, SaveKeyResponse{
			Status:  "error",
			Message: "failed to save key",
		})
		return
	}

	log.Printf("[save] hash=%s node=%s name=%s", req.FileHash, req.NodeID, req.FileName)
	writeJSON(w, http.StatusOK, SaveKeyResponse{
		Status:   "ok",
		FileHash: req.FileHash,
	})
}

// GET /keys/get?hash=<hash>
func (s *Server) handleGetKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	hash := r.URL.Query().Get("hash")
	if hash == "" {
		writeJSON(w, http.StatusBadRequest, GetKeyResponse{
			Status: "error",
			Error:  "missing ?hash parameter",
		})
		return
	}

	rec, err := s.storage.GetKey(hash)
	if err != nil {
		log.Printf("[get] error: %v", err)
		writeJSON(w, http.StatusInternalServerError, GetKeyResponse{
			Status: "error",
			Error:  "failed to retrieve key",
		})
		return
	}

	if rec == nil {
		writeJSON(w, http.StatusNotFound, GetKeyResponse{
			Status:   "not_found",
			FileHash: hash,
		})
		return
	}

	log.Printf("[get] hash=%s node=%s", hash, rec.OriginNodeID)
	writeJSON(w, http.StatusOK, GetKeyResponse{
		Status:   "ok",
		FileHash: rec.FileHash,
		KeyB64:   rec.KeyB64,
		FileName: rec.FileName,
		NodeID:   rec.OriginNodeID,
	})
}

// GET /keys/list?node_id=<id>
func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	nodeID := r.URL.Query().Get("node_id")
	if nodeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"status": "error",
			"error":  "missing ?node_id parameter",
		})
		return
	}

	records, err := s.storage.ListKeys(nodeID)
	if err != nil {
		log.Printf("[list] error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"status": "error",
			"error":  "failed to list keys",
		})
		return
	}

	if records == nil {
		records = []FileKeyRecord{}
	}

	log.Printf("[list] node=%s count=%d", nodeID, len(records))
	writeJSON(w, http.StatusOK, ListKeysResponse{
		Status: "ok",
		NodeID: nodeID,
		Count:  len(records),
		Keys:   records,
	})
}

// DELETE /keys/delete?hash=<hash>&node_id=<id>
func (s *Server) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	hash := r.URL.Query().Get("hash")
	nodeID := r.URL.Query().Get("node_id")

	if hash == "" || nodeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"status": "error",
			"error":  "missing ?hash and ?node_id parameters",
		})
		return
	}

	deleted, err := s.storage.DeleteKey(hash, nodeID)
	if err != nil {
		log.Printf("[delete] error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"status": "error",
			"error":  "failed to delete key",
		})
		return
	}

	if !deleted {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"status": "not_found",
			"error":  "key not found or not owned by this node",
		})
		return
	}

	log.Printf("[delete] hash=%s node=%s", hash, nodeID)
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"hash":   hash,
	})
}
