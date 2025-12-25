package main

import "time"

// Config holds server configuration
type Config struct {
	Port       int      // HTTPS port (default: 8443)
	DBPath     string   // SQLite database path
	MasterKey  string   // Master key for encrypting stored keys (32 bytes)
	CertFile   string   // TLS certificate file
	KeyFile    string   // TLS private key file
	AuthTokens []string // Allowed API tokens
}

// FileKeyRecord represents a stored encryption key
type FileKeyRecord struct {
	ID           int64     `json:"id"`
	FileHash     string    `json:"file_hash"`
	OriginNodeID string    `json:"origin_node_id"`
	KeyEncrypted []byte    `json:"-"`                 // Stored encrypted, never exposed
	KeyB64       string    `json:"key_b64,omitempty"` // Decrypted key (only in responses)
	FileName     string    `json:"file_name"`
	CreatedAt    time.Time `json:"created_at"`
}

// SaveKeyRequest is the request body for /keys/save
type SaveKeyRequest struct {
	FileHash string `json:"hash"`
	KeyB64   string `json:"key_b64"` // Base64-encoded raw key
	NodeID   string `json:"node_id"`
	FileName string `json:"name"`
}

// SaveKeyResponse is the response for /keys/save
type SaveKeyResponse struct {
	Status   string `json:"status"`
	FileHash string `json:"hash"`
	Message  string `json:"message,omitempty"`
}

// GetKeyResponse is the response for /keys/get
type GetKeyResponse struct {
	Status   string `json:"status"`
	FileHash string `json:"hash"`
	KeyB64   string `json:"key_b64,omitempty"`
	FileName string `json:"name,omitempty"`
	NodeID   string `json:"node_id,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ListKeysResponse is the response for /keys/list
type ListKeysResponse struct {
	Status string          `json:"status"`
	NodeID string          `json:"node_id"`
	Count  int             `json:"count"`
	Keys   []FileKeyRecord `json:"keys"`
}

func defaultConfig() *Config {
	return &Config{
		Port:       80,
		DBPath:     "keys.db",
		MasterKey:  "",
		CertFile:   "server.crt",
		KeyFile:    "server.key",
		AuthTokens: []string{"hoshizora-api-token-changeme"}, // Default token - CHANGE IN PRODUCTION
	}
}
