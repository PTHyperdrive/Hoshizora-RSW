package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log"
	"os"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

// loadPeersOnStart decrypts and restores peers from ~/.mixnets/peers.enc at startup.
// Uses the FILE KEY from env.enc (not a PEM).
func loadPeersOnStart(ps *PeerStore, encPath string, key []byte) {
	data, err := os.ReadFile(encPath)
	if err != nil {
		return // file missing on first run is normal
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		log.Printf("[autosave] AEAD init fail: %v", err)
		return
	}
	if len(data) < aead.NonceSize() {
		log.Printf("[autosave] corrupted peers.enc (too short)")
		return
	}
	nonce, ct := data[:aead.NonceSize()], data[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		log.Printf("[autosave] decrypt peers.enc fail: %v", err)
		return
	}

	var peers []PeerInfo
	if err := json.Unmarshal(plain, &peers); err != nil {
		log.Printf("[autosave] unmarshal fail: %v", err)
		return
	}
	for _, p := range peers {
		ps.Upsert(p)
	}
	log.Printf("[autosave] restored %d peers from %s", len(peers), encPath)
}

// startAutoSavePeersLoop periodically encrypts and saves the peer list (every 5m).
func startAutoSavePeersLoop(ctx context.Context, ps *PeerStore, encPath string, key []byte) {
	// save immediately once
	savePeersOnce(ps, encPath, key)

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			savePeersOnce(ps, encPath, key)
		}
	}
}

// savePeersOnce serializes peers and writes to ~/.mixnets/peers.enc using the FILE KEY.
func savePeersOnce(ps *PeerStore, encPath string, key []byte) {
	peers := ps.List()
	if len(peers) == 0 {
		return // nothing to save
	}
	data, err := json.MarshalIndent(peers, "", "  ")
	if err != nil {
		log.Printf("[autosave] marshal peers: %v", err)
		return
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		log.Printf("[autosave] AEAD init fail: %v", err)
		return
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		log.Printf("[autosave] nonce gen fail: %v", err)
		return
	}
	ct := aead.Seal(nil, nonce, data, nil)
	out := append(nonce, ct...)

	if err := os.WriteFile(encPath, out, 0o600); err != nil {
		log.Printf("[autosave] write fail: %v", err)
		return
	}
	log.Printf("[autosave] peers saved -> %s (%d peers)", encPath, len(peers))
}
