//go:build !dll
// +build !dll

package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	// ---- Flags / config ----
	cfg := defaultConfig()

	flag.IntVar(&cfg.APIPort, "api-port", cfg.APIPort, "HTTP API port")
	flag.StringVar(&cfg.MCGroup, "mc-group", cfg.MCGroup, "multicast group (IPv4)")
	flag.IntVar(&cfg.MCPort, "mc-port", cfg.MCPort, "multicast UDP port")
	flag.DurationVar(&cfg.BroadcastIntv, "beacon-intv", cfg.BroadcastIntv, "beacon interval")
	flag.StringVar(&cfg.BindIP, "bind", cfg.BindIP, "HTTP bind IP (default: chosen iface IP)")
	flag.StringVar(&cfg.MCSubnet, "mc-subnet", cfg.MCSubnet, "CIDR to choose NIC, e.g. 192.168.3.0/24")
	flag.StringVar(&cfg.MCIface, "mc-iface", cfg.MCIface, "Interface name to force (overrides mc-subnet)")
	flag.IntVar(&cfg.ControlPort, "control-port", cfg.ControlPort, "localhost control port")

	var (
		newNet  bool
		envPass string
	)
	flag.BoolVar(&newNet, "new-net", false, "generate a new env.enc with fresh keys")
	flag.StringVar(&envPass, "env-pass", "", "passphrase for env.enc (or set MIXNETS_ENV_PASS)")
	flag.Parse()

	// ---- Environment (cross-platform ~/.mixnets) ----
	envPaths, err := initStorageEnv()
	if err != nil {
		log.Fatalf("env init fail: %v", err)
	}

	// ---- Require passphrase (flag or env var) ----
	if envPass == "" {
		envPass = os.Getenv("MIXNETS_ENV_PASS")
	}
	if envPass == "" {
		log.Fatalf("env.enc passphrase missing. Supply --env-pass or set MIXNETS_ENV_PASS")
	}

	// ---- Load or create encrypted env.enc using passphrase ----
	var secrets *EnvSecrets
	if _, err := os.Stat(envPaths.EnvEnc); err == nil {
		secrets, err = loadEnvSecrets(envPaths, []byte(envPass))
		if err != nil {
			log.Fatalf("env.enc load: %v", err)
		}
	} else {
		if !newNet {
			log.Fatalf("environment not set. Run with --new-net and provide --env-pass (or MIXNETS_ENV_PASS) to create %s", envPaths.EnvEnc)
		}
		secrets, err = createEnvSecrets(envPaths, []byte(envPass))
		if err != nil {
			log.Fatalf("env.enc create: %v", err)
		}
		log.Printf("[env] created %s", envPaths.EnvEnc)
	}

	// ---- Identity & MixNet keypair ----
	id := buildNodeIdentity()
	nodeKeys, err := newNodeKeypair()
	if err != nil {
		log.Fatalf("keypair: %v", err)
	}
	log.Printf("[node] id=%s host=%s", id.NodeID[:8], id.Hostname)
	log.Printf("[mix] pubkey(base64)=%s", base64.RawURLEncoding.EncodeToString(nodeKeys.Pub[:]))

	// ---- Pick interface & IP ----
	pick, err := pickInterface(cfg)
	if err != nil {
		log.Fatalf("interface pick: %v", err)
	}
	log.Printf("[net] using iface=%s ip=%s net=%s (forced=%v byName=%v byCIDR=%v)",
		pick.Iface.Name, pick.IPStr, pick.NetStr, pick.Forced, pick.ByName, pick.ByCIDR)

	// ---- Discovery + DHT ----
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ps := newPeerStore()
	dht := newSimpleDHT(id.NodeID)

	// Restore and auto-persist peers using env.enc FileKey
	loadPeersOnStart(ps, envPaths.PeersEnc, secrets.FileKey[:])
	go startAutoSavePeersLoop(ctx, ps, envPaths.PeersEnc, secrets.FileKey[:])

	// Encrypted beacon broadcaster/listener using env.enc BeaconKey
	if err := startBroadcaster(ctx, cfg, id, pick, nodeKeys, secrets.BeaconKey[:]); err != nil {
		log.Fatalf("broadcaster: %v", err)
	}
	if err := startListener(ctx, cfg, ps, pick, secrets.BeaconKey[:]); err != nil {
		log.Fatalf("listener: %v", err)
	}

	// ---- HTTP servers: public + control ----
	bindIP := cfg.BindIP
	if bindIP == "" {
		bindIP = pick.IPStr
	}
	publicAddr := fmt.Sprintf("%s:%d", bindIP, cfg.APIPort)     // peer-facing on NIC IP
	controlAddr := fmt.Sprintf("127.0.0.1:%d", cfg.ControlPort) // local-only

	// Pass secrets into the server so control endpoints can use them
	srv := newServer(cfg, id, ps, dht, nodeKeys, envPaths, secrets)

	publicSrv := &http.Server{
		Addr:              publicAddr,
		Handler:           srv.PublicHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	controlSrv := &http.Server{
		Addr:              controlAddr,
		Handler:           srv.ControlHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("[public http] listening on %s", publicAddr)
		if err := publicSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("public http: %v", err)
		}
	}()
	go func() {
		log.Printf("[control http] listening on %s (local only)", controlAddr)
		if err := controlSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("control http: %v", err)
		}
	}()

	select {} // block forever
}
