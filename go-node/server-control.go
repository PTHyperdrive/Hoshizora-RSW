package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ---- Local-only helpers ----

// ---- Control-plane actions (localhost only) ----

// POST /mix/send-text?to=<DEST_NODE_ID>
// Body: raw text (encrypted with demo key), routed via mixnet to the final hop.
func (s *Server) handleSendText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}
	destID := r.URL.Query().Get("to")
	if destID == "" {
		http.Error(w, "missing ?to=<destNodeID>", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB cap
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	ctB64, err := encryptTextHardcoded(body)
	if err != nil {
		http.Error(w, "encrypt fail: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// msg id
	msgidBytes := make([]byte, 12)
	if _, err := rand.Read(msgidBytes); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	msgid := base64.RawURLEncoding.EncodeToString(msgidBytes)

	env := FinalEnvelope{
		Type:       "text",
		SenderID:   s.id.NodeID,
		ReceiverID: destID,
		MsgID:      msgid,
		DataB64:    ctB64,
	}
	envBytes, _ := json.Marshal(env)

	// choose path (furthest, ends at dest)
	peers := s.peers.List()
	hops, err := chooseHopsFurthest(s.id.NodeID, destID, peers, 4)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	onion, err := buildOnion(hops, envBytes, 8)
	if err != nil {
		http.Error(w, "onion build failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	first := hops[0].Addr
	resp, err := http.Post(fmt.Sprintf("http://%s/mix/relay", first), "application/json", bytes.NewReader(onion))
	if err != nil {
		http.Error(w, "inject fail: "+err.Error(), http.StatusBadGateway)
		return
	}
	_ = resp.Body.Close()

	writeJSON(w, map[string]any{
		"status":    "sent",
		"type":      "text",
		"msgid":     msgid,
		"first_hop": first,
		"hops":      len(hops),
	})
}

// POST /mix/send-file?name=<filename>
// Body: file bytes. Encrypt once, hash ciphertext, store locally, then fanout SAME blob to all peers.
// POST /mix/send-file?name=<filename>
// Body: file bytes. Encrypt once with a fresh per-file key, hash ciphertext,
// store locally, append to chain, then fanout SAME blob to all peers.
func (s *Server) handleSendFileDistribute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "missing ?name=<filename>", http.StatusBadRequest)
		return
	}

	data, err := io.ReadAll(io.LimitReader(r.Body, 128<<20)) // 128MB cap; tune as needed
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// ---- Encrypt ONCE with a fresh per-file key (anti-ransomware design)
	fileKey, err := newFileKey()
	if err != nil {
		http.Error(w, "file key gen fail: "+err.Error(), http.StatusInternalServerError)
		return
	}
	ctRaw, err := aeadSealWithKey(fileKey[:], data) // nonce||ct
	if err != nil {
		http.Error(w, "encrypt fail: "+err.Error(), http.StatusInternalServerError)
		return
	}
	hashHex := sha256Hex(ctRaw)

	// Key filename: <first16_of_hash>.<ext>.fkey (stored locally only)
	ext := "bin"
	if dot := strings.LastIndex(name, "."); dot >= 0 && dot+1 < len(name) {
		ext = name[dot+1:]
	}
	keyFileName := fmt.Sprintf("%s.%s.fkey", hashHex[:16], ext)
	if _, err := saveFileKey(s.paths, keyFileName, &fileKey); err != nil {
		log.Printf("[keyfile] save failed: %v", err)
	}

	// ---- Build envelope (no keys inside), link to current chain tip
	msgidBytes := make([]byte, 16)
	_, _ = rand.Read(msgidBytes)
	msgid := base64.RawURLEncoding.EncodeToString(msgidBytes)
	prev := s.getChainTip()

	env := ReplicateEnvelope{
		MsgID:     msgid,
		OriginID:  s.id.NodeID,
		Name:      name,
		HashHex:   hashHex,
		PrevHash:  prev,
		CipherB64: base64.RawURLEncoding.EncodeToString(ctRaw),
		Created:   time.Now().Unix(),
		Hops:      0,
	}
	storeKey := "blob-" + hashHex + "-" + name
	envBytes, _ := json.Marshal(env)

	// ---- Cache envelope and persist chunk locally
	s.mu.Lock()
	s.kv[storeKey] = envBytes
	s.mu.Unlock()

	chunkPath := filepath.Join(s.paths.ChunksDir, hashHex+".bin")
	if err := os.WriteFile(chunkPath, ctRaw, 0600); err != nil {
		log.Printf("[chunk-save] failed: %v", err)
	} else {
		log.Printf("[chunk-save] saved chunk %s (%d bytes)", chunkPath, len(ctRaw))
	}

	// ---- Append block to local chain
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

	// mark seen
	s.seenMu.Lock()
	s.seen[msgid] = struct{}{}
	s.seenMu.Unlock()

	// ---- Fanout SAME ciphertext to ALL peers (no re-encrypt)
	peers := s.peers.List()
	sent := 0
	for _, p := range peers {
		if p.NodeID == s.id.NodeID || p.Addr == "" {
			continue
		}
		url := fmt.Sprintf("http://%s/replicate", p.Addr)
		resp, err := http.Post(url, "application/json", bytes.NewReader(envBytes))
		if err != nil {
			log.Printf("[replicate] to %s fail: %v", p.Addr, err)
			continue
		}
		_ = resp.Body.Close()
		sent++
	}

	writeJSON(w, map[string]any{
		"status":     "ok",
		"msgid":      msgid,
		"name":       name,
		"hash":       hashHex,
		"store_key":  storeKey,
		"fanout":     sent,
		"peers_seen": len(peers),
		"key_file":   keyFileName,
	})
}

// ControlHandler (127.0.0.1 only): status, peers, send-text, send-file, backup/peers ops.
func (s *Server) ControlHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/chunks/decrypt", func(w http.ResponseWriter, r *http.Request) {
		hash := r.URL.Query().Get("hash")
		name := r.URL.Query().Get("name") // optional but helps ext lookup
		if hash == "" {
			http.Error(w, "missing ?hash=<sha256>", http.StatusBadRequest)
			return
		}
		chunkPath := filepath.Join(s.paths.ChunksDir, hash+".bin")
		ctRaw, err := os.ReadFile(chunkPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("cannot read chunk: %v", err), http.StatusNotFound)
			return
		}

		// get key: prefer ?keyB64= override, else infer from keys dir using ext
		var k [32]byte
		if kb := r.URL.Query().Get("keyB64"); kb != "" {
			b, err := base64.RawURLEncoding.DecodeString(kb)
			if err != nil || len(b) != 32 {
				http.Error(w, "bad keyB64", http.StatusBadRequest)
				return
			}
			copy(k[:], b)
		} else {
			ext := "bin"
			if dot := strings.LastIndex(name, "."); dot >= 0 && dot+1 < len(name) {
				ext = name[dot+1:]
			}
			keyFileName := fmt.Sprintf("%s.%s.fkey", hash[:16], ext)
			k, err = loadFileKey(s.paths, keyFileName)
			if err != nil {
				http.Error(w, "key file not found; provide ?keyB64=", http.StatusNotFound)
				return
			}
		}

		plain, err := aeadOpenWithKey(k[:], ctRaw)
		if err != nil {
			http.Error(w, "decrypt fail: "+err.Error(), http.StatusForbidden)
			return
		}

		// optional: save to file
		if outName := r.URL.Query().Get("out"); outName != "" {
			outPath := filepath.Join(s.paths.ChunksDir, outName)
			if err := os.WriteFile(outPath, plain, 0600); err != nil {
				http.Error(w, "write fail: "+err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"status": "saved", "path": outPath, "bytes": len(plain)})
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(plain)
	})

	// Basic info
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"node_id":  s.id.NodeID,
			"hostname": s.id.Hostname,
			"attrs":    s.id.Attrs,
			"api_port": s.cfg.APIPort,
			"control":  true,
			"time":     time.Now().UTC(),
		})
	})

	// See discovered peers
	mux.HandleFunc("/peers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.peers.List())
	})

	// Sync status - comprehensive sync information
	mux.HandleFunc("/sync/status", func(w http.ResponseWriter, r *http.Request) {
		// Count blocks from chain.jsonl
		blocksCount := 0
		var lastBlockTime int64
		chainPath := filepath.Join(s.paths.BaseDir, "chain", "chain.jsonl")
		if data, err := os.ReadFile(chainPath); err == nil {
			lines := bytes.Split(data, []byte("\n"))
			for _, line := range lines {
				if len(bytes.TrimSpace(line)) > 0 {
					blocksCount++
					// Parse last block for timestamp
					var blk Block
					if json.Unmarshal(line, &blk) == nil {
						lastBlockTime = blk.Created
					}
				}
			}
		}

		// Count chunks in chunks directory
		chunksCount := 0
		if entries, err := os.ReadDir(s.paths.ChunksDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".bin") {
					chunksCount++
				}
			}
		}

		// Peers count
		peersCount := len(s.peers.List())

		// Chain tip
		chainTip := s.getChainTip()

		// Determine sync status
		synced := peersCount > 0 || blocksCount > 0

		writeJSON(w, map[string]any{
			"blocks_count":    blocksCount,
			"chunks_count":    chunksCount,
			"peers_count":     peersCount,
			"chain_tip":       chainTip,
			"node_id":         s.id.NodeID,
			"last_block_time": lastBlockTime,
			"synced":          synced,
			"time":            time.Now().Unix(),
		})
	})

	// Chain list - list all blocks in the chain
	mux.HandleFunc("/chain/list", func(w http.ResponseWriter, r *http.Request) {
		var blocks []Block
		chainPath := filepath.Join(s.paths.BaseDir, "chain", "chain.jsonl")
		if data, err := os.ReadFile(chainPath); err == nil {
			lines := bytes.Split(data, []byte("\n"))
			for _, line := range lines {
				if len(bytes.TrimSpace(line)) > 0 {
					var blk Block
					if json.Unmarshal(line, &blk) == nil {
						blocks = append(blocks, blk)
					}
				}
			}
		}
		writeJSON(w, blocks)
	})

	// Send actions on localhost
	mux.HandleFunc("/mix/send-text", s.handleSendText)
	mux.HandleFunc("/mix/send-file", s.handleSendFileDistribute)

	// Backup / peers save/load/publish/fetch (if you already added them)
	mux.HandleFunc("/backup/get", func(w http.ResponseWriter, r *http.Request) {
		k := r.URL.Query().Get("key")
		if k == "" {
			http.Error(w, "missing key", http.StatusBadRequest)
			return
		}
		s.mu.RLock()
		blob, ok := s.kv[k]
		s.mu.RUnlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(blob)
	})

	// peers save
	mux.HandleFunc("/peers/save", func(w http.ResponseWriter, r *http.Request) {
		pem := r.URL.Query().Get("pem")
		if pem == "" {
			http.Error(w, "missing ?pem", http.StatusBadRequest)
			return
		}
		out := r.URL.Query().Get("out")
		if out == "" {
			out = "peers.enc"
		}
		if err := savePeersEncrypted(out, pem, s.id.NodeID, s.peers); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": "ok", "file": out})
	})

	// peers load
	mux.HandleFunc("/peers/load", func(w http.ResponseWriter, r *http.Request) {
		pem := r.URL.Query().Get("pem")
		if pem == "" {
			http.Error(w, "missing ?pem", http.StatusBadRequest)
			return
		}
		in := r.URL.Query().Get("in")
		if in == "" {
			in = "peers.enc"
		}
		snap, err := loadPeersEncrypted(in, pem)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		n := mergeSnapshot(s.peers, snap)
		writeJSON(w, map[string]any{"status": "ok", "merged": n, "from": in})
	})

	// peers publish
	mux.HandleFunc("/peers/publish", func(w http.ResponseWriter, r *http.Request) {
		pem := r.URL.Query().Get("pem")
		if pem == "" {
			http.Error(w, "missing ?pem", http.StatusBadRequest)
			return
		}
		key, err := deriveSymKeyFromPEM(pem)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		snap := exportPeersSnapshot(s.id.NodeID, s.peers)
		blob, err := encryptSnapshot(key, snap)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		storeKey := "peers:" + s.id.NodeID
		s.mu.Lock()
		s.kv[storeKey] = blob
		s.mu.Unlock()
		s.dht.Put(storeKey, []string{s.id.NodeID})
		writeJSON(w, map[string]any{"status": "ok", "dht_key": storeKey, "size": len(blob)})
	})

	// peers fetch from DHT
	mux.HandleFunc("/peers/fetch", func(w http.ResponseWriter, r *http.Request) {
		from := r.URL.Query().Get("from")
		pem := r.URL.Query().Get("pem")
		if from == "" || pem == "" {
			http.Error(w, "missing ?from and/or ?pem", http.StatusBadRequest)
			return
		}
		storeKey := "peers:" + from
		providers := s.dht.Get(storeKey)
		if len(providers) == 0 {
			http.Error(w, "no providers", http.StatusNotFound)
			return
		}
		var addr string
		for _, p := range s.peers.List() {
			if p.NodeID == providers[0] && p.Addr != "" {
				addr = p.Addr
				break
			}
		}
		if addr == "" {
			http.Error(w, "provider address unknown", http.StatusBadRequest)
			return
		}
		resp, err := http.Get("http://" + addr + "/fetch?key=" + storeKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			http.Error(w, "provider fetch failed: "+string(b), http.StatusBadGateway)
			return
		}
		cipherBlob, _ := io.ReadAll(resp.Body)
		key, err := deriveSymKeyFromPEM(pem)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		snap, err := decryptSnapshot(key, cipherBlob)
		if err != nil {
			http.Error(w, "decrypt fail: "+err.Error(), http.StatusForbidden)
			return
		}
		n := mergeSnapshot(s.peers, snap)
		writeJSON(w, map[string]any{"status": "ok", "merged": n, "from_provider": addr})
	})

	// Local-only guard (defense in depth)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if host != "127.0.0.1" && host != "::1" {
			http.Error(w, "local-only", http.StatusForbidden)
			return
		}
		log.Printf("[control] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		mux.ServeHTTP(w, r)
	})
}
