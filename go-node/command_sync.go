package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// SyncCommand represents a command to broadcast to all peers
type SyncCommand struct {
	Type       string `json:"type"`        // "encrypt" or "decrypt"
	FolderPath string `json:"folder_path"` // Local folder path on receiver
	Recursive  bool   `json:"recursive"`
	OriginNode string `json:"origin_node"`
	MsgID      string `json:"msgid"`
	Timestamp  int64  `json:"timestamp"`
}

// CommandCallback is called when receiving a command from peer
type CommandCallback func(cmd SyncCommand)

var (
	commandCallbacks   []CommandCallback
	commandCallbacksMu sync.RWMutex
	seenCommands       = make(map[string]struct{})
	seenCommandsMu     sync.Mutex
)

// RegisterCommandCallback registers a callback for incoming commands
func RegisterCommandCallback(cb CommandCallback) {
	commandCallbacksMu.Lock()
	defer commandCallbacksMu.Unlock()
	commandCallbacks = append(commandCallbacks, cb)
}

// handleP2PCommand receives command from peer and executes locally
func (s *Server) handleP2PCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}

	var cmd SyncCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Loop prevention
	seenCommandsMu.Lock()
	if _, ok := seenCommands[cmd.MsgID]; ok {
		seenCommandsMu.Unlock()
		writeJSON(w, map[string]any{"status": "seen"})
		return
	}
	seenCommands[cmd.MsgID] = struct{}{}
	seenCommandsMu.Unlock()

	log.Printf("[p2p-cmd] received %s from %s for folder: %s", cmd.Type, cmd.OriginNode, cmd.FolderPath)

	// Execute callbacks (for DLL mode / in-process handling)
	commandCallbacksMu.RLock()
	for _, cb := range commandCallbacks {
		go cb(cmd) // async so we don't block
	}
	commandCallbacksMu.RUnlock()

	// Forward to other peers
	go s.forwardCommand(cmd)

	writeJSON(w, map[string]any{
		"status": "received",
		"type":   cmd.Type,
		"msgid":  cmd.MsgID,
	})
}

// handleBroadcastCommand initiates command broadcast to all peers (localhost only)
func (s *Server) handleBroadcastCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}

	var cmd SyncCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Fill in origin and timestamp
	cmd.OriginNode = s.id.NodeID
	cmd.Timestamp = time.Now().Unix()
	if cmd.MsgID == "" {
		cmd.MsgID = randomMsgID()
	}

	// Mark as seen locally
	seenCommandsMu.Lock()
	seenCommands[cmd.MsgID] = struct{}{}
	seenCommandsMu.Unlock()

	// Broadcast to all peers
	sent := s.broadcastToPeers(cmd)

	log.Printf("[broadcast] sent %s command to %d peers", cmd.Type, sent)

	writeJSON(w, map[string]any{
		"status": "broadcast",
		"type":   cmd.Type,
		"msgid":  cmd.MsgID,
		"sent":   sent,
	})
}

// handleExportEnv exports env.enc for copying to other machines
func (s *Server) handleExportEnv(w http.ResponseWriter, r *http.Request) {
	envPath := s.paths.EnvFile
	if envPath == "" {
		envPath = "env.enc"
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		http.Error(w, "cannot read env.enc: "+err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=env.enc")
	w.Write(data)
}

// handleGetPendingCommand returns pending command for polling (subprocess mode)
func (s *Server) handleGetPendingCommand(w http.ResponseWriter, r *http.Request) {
	s.pendingCmdMu.Lock()
	cmd := s.pendingCmd
	s.pendingCmd = nil
	s.pendingCmdMu.Unlock()

	if cmd == nil {
		writeJSON(w, map[string]any{"status": "none"})
		return
	}

	writeJSON(w, map[string]any{
		"status":  "pending",
		"command": cmd,
	})
}

func (s *Server) broadcastToPeers(cmd SyncCommand) int {
	peers := s.peers.List()
	sent := 0
	cmdBytes, _ := json.Marshal(cmd)

	for _, p := range peers {
		if p.NodeID == s.id.NodeID || p.Addr == "" {
			continue
		}
		url := "http://" + p.Addr + "/p2p/command"
		resp, err := http.Post(url, "application/json", bytes.NewReader(cmdBytes))
		if err != nil {
			log.Printf("[broadcast] to %s failed: %v", p.Addr, err)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		sent++
	}
	return sent
}

func (s *Server) forwardCommand(cmd SyncCommand) {
	s.broadcastToPeers(cmd)
}

func randomMsgID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// storePendingCommand stores command for subprocess mode polling
func (s *Server) storePendingCommand(cmd SyncCommand) {
	s.pendingCmdMu.Lock()
	s.pendingCmd = &cmd
	s.pendingCmdMu.Unlock()
}
