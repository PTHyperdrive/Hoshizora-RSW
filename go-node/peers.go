// peers.go
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/rand"
	"net"
	"os"
	"strconv"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

//
// === NEW: PeerStore constructor + methods ===
//

func newPeerStore() *PeerStore {
	return &PeerStore{
		peers: make(map[string]PeerInfo),
	}
}

// Upsert inserts or updates a peer by NodeID.
func (ps *PeerStore) Upsert(p PeerInfo) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.peers[p.NodeID] = p
}

// List returns a snapshot copy of all peers.
func (ps *PeerStore) List() []PeerInfo {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	out := make([]PeerInfo, 0, len(ps.peers))
	for _, v := range ps.peers {
		out = append(out, v)
	}
	return out
}

//
// === Existing helpers for encrypted peer snapshots (unchanged) ===
//

func deriveSymKeyFromPEM(pemPath string) ([]byte, error) {
	b, err := os.ReadFile(pemPath)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	return sum[:], nil // 32 bytes
}

func exportPeersSnapshot(selfID string, ps *PeerStore) PeerSnapshot {
	all := ps.List()
	out := make([]PeerBrief, 0, len(all))
	for _, p := range all {
		// PubKey stored as raw []byte in PeerInfo; export as base64url
		var pkb64 string
		if len(p.PubKey) == 32 {
			pkb64 = base64.RawURLEncoding.EncodeToString(p.PubKey)
		}
		out = append(out, PeerBrief{
			NodeID:    p.NodeID,
			Addr:      p.Addr,
			Hostname:  p.Hostname,
			LastSeen:  p.LastSeen,
			PubKeyB64: pkb64,
		})
	}
	return PeerSnapshot{
		Version: 1,
		NodeID:  selfID,
		Created: time.Now().UTC(),
		Peers:   out,
	}
}

func encryptSnapshot(key32 []byte, snap PeerSnapshot) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key32)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	plain, _ := json.Marshal(snap)
	ct := aead.Seal(nil, nonce, plain, nil)
	return append(nonce, ct...), nil
}

func decryptSnapshot(key32, nonceAndCT []byte) (PeerSnapshot, error) {
	var snap PeerSnapshot
	if len(nonceAndCT) < chacha20poly1305.NonceSizeX {
		return snap, errors.New("ciphertext too short")
	}
	aead, err := chacha20poly1305.NewX(key32)
	if err != nil {
		return snap, err
	}
	nonce := nonceAndCT[:chacha20poly1305.NonceSizeX]
	ct := nonceAndCT[chacha20poly1305.NonceSizeX:]
	pt, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return snap, err
	}
	if err := json.Unmarshal(pt, &snap); err != nil {
		return snap, err
	}
	return snap, nil
}

func savePeersEncrypted(path, pemPath, selfID string, ps *PeerStore) error {
	key, err := deriveSymKeyFromPEM(pemPath)
	if err != nil {
		return err
	}
	snap := exportPeersSnapshot(selfID, ps)
	blob, err := encryptSnapshot(key, snap)
	if err != nil {
		return err
	}
	return os.WriteFile(path, blob, 0600)
}

func loadPeersEncrypted(path, pemPath string) (PeerSnapshot, error) {
	var zero PeerSnapshot
	blob, err := os.ReadFile(path)
	if err != nil {
		return zero, err
	}
	key, err := deriveSymKeyFromPEM(pemPath)
	if err != nil {
		return zero, err
	}
	return decryptSnapshot(key, blob)
}

func mergeSnapshot(ps *PeerStore, snap PeerSnapshot) int {
	// Minimal de-dup merge
	count := 0
	for _, b := range snap.Peers {
		var pk []byte
		if b.PubKeyB64 != "" {
			if dec, err := base64.RawURLEncoding.DecodeString(b.PubKeyB64); err == nil && len(dec) == 32 {
				pk = dec
			}
		}
		ps.Upsert(PeerInfo{
			NodeID:   b.NodeID,
			Addr:     b.Addr,
			APIPort:  parsePortFromAddr(b.Addr),
			Hostname: b.Hostname,
			LastSeen: b.LastSeen,
			PubKey:   pk,
		})
		count++
	}
	return count
}

func parsePortFromAddr(addr string) int {
	if addr == "" {
		return 0
	}
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	p, err := strconv.Atoi(portStr)
	if err != nil || p < 0 || p > 65535 {
		return 0
	}
	return p
}
