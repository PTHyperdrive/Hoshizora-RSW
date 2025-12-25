package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
	_ "modernc.org/sqlite"
)

// Storage handles encrypted key persistence
type Storage struct {
	db        *sql.DB
	masterKey [32]byte
}

// NewStorage creates a new storage with the given master key
func NewStorage(dbPath string, masterKeyStr string) (*Storage, error) {
	// Derive master key from string using SHA-256
	masterKey := sha256.Sum256([]byte(masterKeyStr))

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	s := &Storage{
		db:        db,
		masterKey: masterKey,
	}

	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return s, nil
}

func (s *Storage) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS file_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_hash TEXT UNIQUE NOT NULL,
		origin_node_id TEXT NOT NULL,
		key_encrypted BLOB NOT NULL,
		file_name TEXT,
		created_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_file_keys_node ON file_keys(origin_node_id);
	CREATE INDEX IF NOT EXISTS idx_file_keys_hash ON file_keys(file_hash);
	`
	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database connection
func (s *Storage) Close() error {
	return s.db.Close()
}

// encryptKey encrypts a raw key using the master key
func (s *Storage) encryptKey(rawKey []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(s.masterKey[:])
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	ciphertext := aead.Seal(nil, nonce, rawKey, nil)
	return append(nonce, ciphertext...), nil
}

// decryptKey decrypts a stored key using the master key
func (s *Storage) decryptKey(encryptedKey []byte) ([]byte, error) {
	if len(encryptedKey) < chacha20poly1305.NonceSizeX {
		return nil, errors.New("encrypted key too short")
	}

	aead, err := chacha20poly1305.NewX(s.masterKey[:])
	if err != nil {
		return nil, err
	}

	nonce := encryptedKey[:chacha20poly1305.NonceSizeX]
	ciphertext := encryptedKey[chacha20poly1305.NonceSizeX:]

	return aead.Open(nil, nonce, ciphertext, nil)
}

// SaveKey stores an encrypted key
func (s *Storage) SaveKey(fileHash, nodeID, keyB64, fileName string) error {
	// Decode the key
	rawKey, err := base64.RawURLEncoding.DecodeString(keyB64)
	if err != nil {
		// Try standard base64
		rawKey, err = base64.StdEncoding.DecodeString(keyB64)
		if err != nil {
			return fmt.Errorf("decode key: %w", err)
		}
	}

	// Encrypt with master key
	encryptedKey, err := s.encryptKey(rawKey)
	if err != nil {
		return fmt.Errorf("encrypt key: %w", err)
	}

	// Insert or update
	query := `
	INSERT INTO file_keys (file_hash, origin_node_id, key_encrypted, file_name, created_at)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(file_hash) DO UPDATE SET
		key_encrypted = excluded.key_encrypted,
		file_name = excluded.file_name
	`
	_, err = s.db.Exec(query, fileHash, nodeID, encryptedKey, fileName, time.Now().Unix())
	return err
}

// GetKey retrieves and decrypts a key by file hash
func (s *Storage) GetKey(fileHash string) (*FileKeyRecord, error) {
	query := `SELECT id, file_hash, origin_node_id, key_encrypted, file_name, created_at 
	          FROM file_keys WHERE file_hash = ?`

	var rec FileKeyRecord
	var encryptedKey []byte
	var createdUnix int64

	err := s.db.QueryRow(query, fileHash).Scan(
		&rec.ID, &rec.FileHash, &rec.OriginNodeID,
		&encryptedKey, &rec.FileName, &createdUnix,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Decrypt the key
	rawKey, err := s.decryptKey(encryptedKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt key: %w", err)
	}

	rec.KeyB64 = base64.StdEncoding.EncodeToString(rawKey)
	rec.CreatedAt = time.Unix(createdUnix, 0)
	return &rec, nil
}

// ListKeys returns all keys for a given node
func (s *Storage) ListKeys(nodeID string) ([]FileKeyRecord, error) {
	query := `SELECT id, file_hash, origin_node_id, file_name, created_at 
	          FROM file_keys WHERE origin_node_id = ? ORDER BY created_at DESC`

	rows, err := s.db.Query(query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []FileKeyRecord
	for rows.Next() {
		var rec FileKeyRecord
		var createdUnix int64
		if err := rows.Scan(&rec.ID, &rec.FileHash, &rec.OriginNodeID, &rec.FileName, &createdUnix); err != nil {
			return nil, err
		}
		rec.CreatedAt = time.Unix(createdUnix, 0)
		records = append(records, rec)
	}

	return records, rows.Err()
}

// DeleteKey removes a key by file hash (only if caller is owner)
func (s *Storage) DeleteKey(fileHash, nodeID string) (bool, error) {
	result, err := s.db.Exec(
		"DELETE FROM file_keys WHERE file_hash = ? AND origin_node_id = ?",
		fileHash, nodeID,
	)
	if err != nil {
		return false, err
	}

	affected, _ := result.RowsAffected()
	return affected > 0, nil
}
