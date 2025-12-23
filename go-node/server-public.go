package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

func newServer(cfg *Config, id NodeIdentity, peers *PeerStore, dht DHT, nk *NodeKeypair, paths *EnvPaths, secrets *EnvSecrets) *Server {
	return &Server{
		cfg:      cfg,
		id:       id,
		peers:    peers,
		dht:      dht,
		nodeKeys: nk,
		paths:    paths,
		secrets:  secrets,
		kv:       make(map[string][]byte),
		seen:     make(map[string]struct{}),
	}
}

// ReplicateEnvelope is the exact blob we propagate (no re-encrypt on hops).
type ReplicateEnvelope struct {
	MsgID     string `json:"msgid"`
	OriginID  string `json:"origin_id"`
	Name      string `json:"name"`
	HashHex   string `json:"hash_hex"`
	PrevHash  string `json:"prev_hash"` // NEW: chain link
	CipherB64 string `json:"cipher_b64"`
	EncKeyB64 string `json:"enckey_b64"`
	Created   int64  `json:"created_unix"`
	Hops      int    `json:"hops"`
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// PublicHandler exposes peer-facing endpoints on NIC IP.
// Includes: /fetch (blob fetch), /mix/relay (mix hops), /replicate (blockchain-style fanout), /dht/*
func (s *Server) PublicHandler() http.Handler {
	mux := http.NewServeMux()

	// Public fetch: peers get stored blob by key (used by DHT pulls / replication)
	mux.HandleFunc("/fetch", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "missing ?key", http.StatusBadRequest)
			return
		}
		s.mu.RLock()
		val, ok := s.kv[key]
		s.mu.RUnlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(val)
	})

	// Mixnet relay (peer-to-peer onion hops)
	mux.HandleFunc("/mix/relay", relayHandler(s.nodeKeys, s))

	// Replication endpoint: receive SAME ciphertext, verify hash, store, forward-once
	mux.HandleFunc("/replicate", func(w http.ResponseWriter, r *http.Request) {
		localTip := s.getChainTip()
		if r.Method != http.MethodPost {
			http.Error(w, "use POST", http.StatusMethodNotAllowed)
			return
		}
		var env ReplicateEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			http.Error(w, "bad envelope", http.StatusBadRequest)
			return
		}

		if env.PrevHash != localTip {
			http.Error(w, "chain mismatch: local tip "+localTip+" != prev "+env.PrevHash, http.StatusConflict)
			return
		}
		// loop prevention
		s.seenMu.Lock()
		if _, ok := s.seen[env.MsgID]; ok {
			s.seenMu.Unlock()
			writeJSON(w, map[string]any{"status": "seen"})
			return
		}
		s.seen[env.MsgID] = struct{}{}
		s.seenMu.Unlock()

		// verify ciphertext hash (no decryption)
		ctRaw, err := base64.RawURLEncoding.DecodeString(env.CipherB64)
		if err != nil {
			http.Error(w, "bad cipher b64", http.StatusBadRequest)
			return
		}
		if sha256Hex(ctRaw) != env.HashHex {
			http.Error(w, "hash mismatch", http.StatusBadRequest)
			return
		}
		blk := Block{
			Hash:     env.HashHex,
			PrevHash: env.PrevHash,
			Name:     env.Name,
			Size:     len(ctRaw),
			Created:  env.Created,
			OriginID: env.OriginID,
		}
		if err := s.appendBlock(blk); err != nil {
			http.Error(w, "append block fail: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// store envelope (deterministic key)
		storeKey := "blob-" + env.HashHex + "-" + env.Name
		env.Hops++
		envBytes, _ := json.Marshal(env)

		s.mu.Lock()
		s.kv[storeKey] = envBytes
		s.mu.Unlock()
		chunkPath := filepath.Join(s.paths.ChunksDir, env.HashHex+".bin")
		if err := os.WriteFile(chunkPath, ctRaw, 0600); err != nil {
			http.Error(w, "chunk write fail: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// forward to other peers (no re-encrypt, same envelope)
		sent := 0
		for _, p := range s.peers.List() {
			if p.NodeID == s.id.NodeID || p.Addr == "" {
				continue
			}
			url := fmt.Sprintf("http://%s/replicate", p.Addr)
			resp, err := http.Post(url, "application/json", bytes.NewReader(envBytes))
			if err != nil {
				log.Printf("[replicate] fwd -> %s fail: %v", p.Addr, err)
				continue
			}
			_ = resp.Body.Close()
			sent++
		}

		writeJSON(w, map[string]any{
			"status": "stored",
			"key":    storeKey,
			"sent":   sent,
			"hops":   env.Hops,
			"tip":    s.getChainTip(),
		})
	})

	// Minimal DHT endpoints for peers
	mux.HandleFunc("/dht/put", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "use POST", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Key       string   `json:"key"`
			Providers []string `json:"providers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.Key == "" || len(body.Providers) == 0 {
			http.Error(w, "need key + providers[]", http.StatusBadRequest)
			return
		}
		s.dht.Put(body.Key, body.Providers)
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/dht/get", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "missing ?key=", http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"key": key, "providers": s.dht.Get(key)})
	})

	// Public log wrapper
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		log.Printf("[public] %s %s from %s", r.Method, r.URL.Path, ip)
		mux.ServeHTTP(w, r)
	})
}
func (s *Server) getChainTip() string {
	s.chainMu.Lock()
	defer s.chainMu.Unlock()
	return s.chainTip
}
func (s *Server) appendBlock(b Block) error {
	s.chainMu.Lock()
	defer s.chainMu.Unlock()

	// ensure chain dir
	chainDir := filepath.Join(s.paths.BaseDir, "chain")
	if err := os.MkdirAll(chainDir, 0700); err != nil {
		return err
	}

	// write JSONL
	line, _ := json.Marshal(b)
	line = append(line, '\n')
	logPath := filepath.Join(chainDir, "chain.jsonl")
	if err := appendFile(logPath, line); err != nil {
		return err
	}

	// update tip
	s.chainTip = b.Hash
	return nil
}

// appendFile appends bytes atomically-ish.
func appendFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
