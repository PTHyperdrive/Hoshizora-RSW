//go:build dll && cgo
// +build dll,cgo

package main

/*
#include <stdlib.h>
#include <string.h>

typedef struct {
	int success;
	char* message;
	char* node_id;
} P2PInitResult;

typedef struct {
	char* status;      // JSON status response
	char* peers;       // JSON peers array
	char* node_id;
	int api_port;
	int control_port;
} P2PStatus;
*/
import "C"
import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
	"unsafe"
)

// Global state for DLL mode
var (
	dllMu       sync.Mutex
	dllCtx      context.Context
	dllCancel   context.CancelFunc
	dllServer   *Server
	dllPeers    *PeerStore
	dllDHT      DHT
	dllNodeKeys *NodeKeypair
	dllSecrets  *EnvSecrets
	dllPaths    *EnvPaths
	dllID       NodeIdentity
	dllCfg      *Config
	dllRunning  bool
	dllPick     *ifacePick

	// HTTP servers
	dllPublicSrv  *http.Server
	dllControlSrv *http.Server
)

// cString creates a C string from Go string (caller must free)
func cString(s string) *C.char {
	return C.CString(s)
}

// goString converts C string to Go string
func goString(s *C.char) string {
	if s == nil {
		return ""
	}
	return C.GoString(s)
}

// P2P_Init initializes the p2p node with the given parameters.
// Returns 0 on success, non-zero on error.
// forceNewEnv: if 1, recreate env.enc even if it exists (like --new-net)
//
//export P2P_Init
func P2P_Init(envPass *C.char, apiPort C.int, controlPort C.int, mcGroup *C.char, mcPort C.int, keySaverUrl *C.char, forceNewEnv C.int) C.int {
	dllMu.Lock()
	defer dllMu.Unlock()

	if dllRunning {
		log.Println("[dll] already initialized")
		return -1
	}

	passphrase := goString(envPass)
	if passphrase == "" {
		log.Println("[dll] error: empty passphrase")
		return -2
	}

	// Build config
	dllCfg = defaultConfig()
	dllCfg.APIPort = int(apiPort)
	dllCfg.ControlPort = int(controlPort)
	if mcGroup != nil {
		dllCfg.MCGroup = goString(mcGroup)
	}
	if mcPort > 0 {
		dllCfg.MCPort = int(mcPort)
	}

	// Initialize storage environment
	var err error
	dllPaths, err = initStorageEnv()
	if err != nil {
		log.Printf("[dll] env init fail: %v", err)
		return -3
	}

	// Handle env.enc: load existing or create new
	envExists := false
	if _, err := os.Stat(dllPaths.EnvEnc); err == nil {
		envExists = true
	}

	if envExists && forceNewEnv == 0 {
		// Try to load existing env.enc
		dllSecrets, err = loadEnvSecrets(dllPaths, []byte(passphrase))
		if err != nil {
			log.Printf("[dll] env.enc load failed (wrong passphrase?): %v", err)
			log.Printf("[dll] TIP: Set forceNewEnv=1 to recreate with new passphrase")

			// Auto-backup and recreate if load fails
			backupPath := dllPaths.EnvEnc + ".backup"
			if errRename := os.Rename(dllPaths.EnvEnc, backupPath); errRename == nil {
				log.Printf("[dll] backed up old env.enc to %s", backupPath)
				// Create new with provided passphrase
				dllSecrets, err = createEnvSecrets(dllPaths, []byte(passphrase))
				if err != nil {
					log.Printf("[dll] env.enc create fail: %v", err)
					return -5
				}
				log.Printf("[dll] created new env.enc with provided passphrase")
			} else {
				log.Printf("[dll] failed to backup env.enc: %v", errRename)
				return -4
			}
		}
	} else {
		// forceNewEnv=1 or env.enc doesn't exist: create new
		if envExists {
			// Backup existing before overwrite
			backupPath := dllPaths.EnvEnc + ".backup"
			_ = os.Rename(dllPaths.EnvEnc, backupPath)
			log.Printf("[dll] backed up existing env.enc to %s", backupPath)
		}
		dllSecrets, err = createEnvSecrets(dllPaths, []byte(passphrase))
		if err != nil {
			log.Printf("[dll] env.enc create fail: %v", err)
			return -5
		}
		log.Printf("[dll] created new env.enc")
	}

	// Build identity and keypair
	dllID = buildNodeIdentity()
	dllNodeKeys, err = newNodeKeypair()
	if err != nil {
		log.Printf("[dll] keypair fail: %v", err)
		return -6
	}

	log.Printf("[dll] initialized node=%s", dllID.NodeID[:8])
	return 0
}

// P2P_Start starts the p2p node services (discovery, HTTP servers).
// Returns 0 on success.
//
//export P2P_Start
func P2P_Start() C.int {
	dllMu.Lock()
	defer dllMu.Unlock()

	if dllRunning {
		return -1
	}
	if dllCfg == nil {
		return -2 // not initialized
	}

	dllCtx, dllCancel = context.WithCancel(context.Background())

	// Pick interface
	var err error
	dllPick, err = pickInterface(dllCfg)
	if err != nil {
		log.Printf("[dll] interface pick fail: %v", err)
		return -3
	}

	// Peer store and DHT
	dllPeers = newPeerStore()
	dllDHT = newSimpleDHT(dllID.NodeID)

	// Load saved peers
	loadPeersOnStart(dllPeers, dllPaths.PeersEnc, dllSecrets.FileKey[:])
	go startAutoSavePeersLoop(dllCtx, dllPeers, dllPaths.PeersEnc, dllSecrets.FileKey[:])

	// Start beacon broadcaster/listener
	if err := startBroadcaster(dllCtx, dllCfg, dllID, dllPick, dllNodeKeys, dllSecrets.BeaconKey[:]); err != nil {
		log.Printf("[dll] broadcaster fail: %v", err)
		return -4
	}
	if err := startListener(dllCtx, dllCfg, dllPeers, dllPick, dllSecrets.BeaconKey[:]); err != nil {
		log.Printf("[dll] listener fail: %v", err)
		return -5
	}

	// Create server
	dllServer = newServer(dllCfg, dllID, dllPeers, dllDHT, dllNodeKeys, dllPaths, dllSecrets)

	// HTTP servers
	bindIP := dllCfg.BindIP
	if bindIP == "" {
		bindIP = dllPick.IPStr
	}
	publicAddr := fmt.Sprintf("%s:%d", bindIP, dllCfg.APIPort)
	controlAddr := fmt.Sprintf("127.0.0.1:%d", dllCfg.ControlPort)

	dllPublicSrv = &http.Server{
		Addr:              publicAddr,
		Handler:           dllServer.PublicHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	dllControlSrv = &http.Server{
		Addr:              controlAddr,
		Handler:           dllServer.ControlHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("[dll] public HTTP on %s", publicAddr)
		if err := dllPublicSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[dll] public HTTP error: %v", err)
		}
	}()

	go func() {
		log.Printf("[dll] control HTTP on %s", controlAddr)
		if err := dllControlSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[dll] control HTTP error: %v", err)
		}
	}()

	dllRunning = true
	log.Printf("[dll] started successfully")
	return 0
}

// P2P_Stop gracefully stops all p2p services.
//
//export P2P_Stop
func P2P_Stop() {
	dllMu.Lock()
	defer dllMu.Unlock()

	if !dllRunning {
		return
	}

	if dllCancel != nil {
		dllCancel()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if dllPublicSrv != nil {
		_ = dllPublicSrv.Shutdown(ctx)
	}
	if dllControlSrv != nil {
		_ = dllControlSrv.Shutdown(ctx)
	}

	dllRunning = false
	log.Printf("[dll] stopped")
}

// P2P_GetStatus returns JSON status. Caller must free the returned string.
//
//export P2P_GetStatus
func P2P_GetStatus() *C.char {
	dllMu.Lock()
	defer dllMu.Unlock()

	status := map[string]any{
		"running":      dllRunning,
		"node_id":      "",
		"hostname":     "",
		"api_port":     0,
		"control_port": 0,
		"peers_count":  0,
	}

	if dllRunning && dllCfg != nil {
		status["node_id"] = dllID.NodeID
		status["hostname"] = dllID.Hostname
		status["api_port"] = dllCfg.APIPort
		status["control_port"] = dllCfg.ControlPort
		if dllPeers != nil {
			status["peers_count"] = len(dllPeers.List())
		}
	}

	b, _ := json.Marshal(status)
	return cString(string(b))
}

// P2P_GetPeers returns JSON array of peers. Caller must free the returned string.
//
//export P2P_GetPeers
func P2P_GetPeers() *C.char {
	dllMu.Lock()
	defer dllMu.Unlock()

	if dllPeers == nil {
		return cString("[]")
	}

	peers := dllPeers.List()
	b, _ := json.Marshal(peers)
	return cString(string(b))
}

// P2P_GetNodeID returns the node ID. Caller must free.
//
//export P2P_GetNodeID
func P2P_GetNodeID() *C.char {
	dllMu.Lock()
	defer dllMu.Unlock()
	return cString(dllID.NodeID)
}

// P2P_GetPublicKey returns base64 encoded public key. Caller must free.
//
//export P2P_GetPublicKey
func P2P_GetPublicKey() *C.char {
	dllMu.Lock()
	defer dllMu.Unlock()
	if dllNodeKeys == nil {
		return cString("")
	}
	return cString(base64.RawURLEncoding.EncodeToString(dllNodeKeys.Pub[:]))
}

// P2P_FreeString frees a string returned by this library.
//
//export P2P_FreeString
func P2P_FreeString(s *C.char) {
	if s != nil {
		C.free(unsafe.Pointer(s))
	}
}

// P2P_IsRunning returns 1 if node is running, 0 otherwise.
//
//export P2P_IsRunning
func P2P_IsRunning() C.int {
	dllMu.Lock()
	defer dllMu.Unlock()
	if dllRunning {
		return 1
	}
	return 0
}

// Required for c-shared build mode
func main() {}
