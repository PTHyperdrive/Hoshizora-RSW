package main

import (
	"net"
	"sync"
	"time"
)

// Server hosts HTTP APIs, local KV, and node keypair for MixNet.
type Server struct {
	cfg          *Config
	id           NodeIdentity
	peers        *PeerStore
	dht          DHT
	nodeKeys     *NodeKeypair
	paths        *EnvPaths
	secrets      *EnvSecrets
	mu           sync.RWMutex
	kv           map[string][]byte
	chainMu      sync.Mutex
	chainTip     string
	seenMu       sync.Mutex
	seen         map[string]struct{}
	pendingCmdMu sync.Mutex
	pendingCmd   *SyncCommand
}

type Config struct {
	APIPort       int
	MCGroup       string
	MCPort        int
	BroadcastIntv time.Duration
	MaxDataBytes  int64
	ControlPort   int
	BindIP        string // HTTP bind IP (defaults to detected iface IP)
	MCSubnet      string // e.g., "192.168.3.0/24"
	MCIface       string // optional interface name to force
}

type ifacePick struct {
	Iface  *net.Interface
	IP     net.IP
	IPNet  *net.IPNet
	IPStr  string
	NetStr string
	Forced bool
	ByName bool
	ByCIDR bool
}

type EnvPaths struct {
	BaseDir   string
	ConfigEnc string
	PeersEnc  string
	ChunksDir string
	KeyPath   string // legacy (still used by X25519 node keys if you kept that)
	EnvEnc    string // NEW: env.enc (JSON with BeaconKey/FileKey)
	EnvFile   string // Full path to env.enc file
}

type NodeIdentity struct {
	NodeID   string
	Hostname string
	Attrs    map[string]string
}

type PeerStore struct {
	mu    sync.RWMutex
	peers map[string]PeerInfo
}

// Beacon is the structure each node advertises (encrypted on wire)
type Beacon struct {
	Type     string `json:"type"`
	NodeID   string `json:"node_id"`
	APIPort  int    `json:"api_port"`
	Hostname string `json:"hostname"`
	TS       int64  `json:"ts"`
	PubKey   string `json:"pubkey"` // Mixnet public key (base64)
}

// PeerInfo is each peer record discovered
type PeerInfo struct {
	NodeID   string    `json:"node_id"`
	Addr     string    `json:"addr"` // "ip:apiport"
	APIPort  int       `json:"api_port"`
	Hostname string    `json:"hostname"`
	LastSeen time.Time `json:"last_seen"`
	PubKey   []byte    `json:"-"`
}
type onionLayerPlain struct {
	Next    string `json:"next"`    // next hop address (host:port) or empty if final
	Payload string `json:"payload"` // base64(inner ciphertext)
	Meta    struct {
		Final bool   `json:"final"`
		MsgID string `json:"msgid"`
		TTL   int    `json:"ttl"`
	} `json:"meta"`
}

type onionPacket struct {
	EphemeralPub string `json:"ephemeral_pub"` // base64 32
	Ciphertext   string `json:"ciphertext"`    // base64 nonce+ciphertext
}

// PeerStore holds discovered peers

type PeerSnapshot struct {
	Version int         `json:"version"`
	NodeID  string      `json:"node_id"`
	Created time.Time   `json:"created"`
	Peers   []PeerBrief `json:"peers"`
}

type PeerBrief struct {
	NodeID    string    `json:"node_id"`
	Addr      string    `json:"addr"`
	Hostname  string    `json:"hostname"`
	LastSeen  time.Time `json:"last_seen"`
	PubKeyB64 string    `json:"pubkey_b64"`
}

type Block struct {
	Hash     string `json:"hash"`
	PrevHash string `json:"prev_hash"`
	Name     string `json:"name"`
	Size     int    `json:"size"`
	Created  int64  `json:"created_unix"`
	OriginID string `json:"origin_id"`
}

type EnvSecrets struct {
	BeaconKeyB64 string   `json:"beacon_key_b64"` // base64url(32B)
	FileKeyB64   string   `json:"file_key_b64"`   // base64url(32B)
	BeaconKey    [32]byte `json:"-"`
	FileKey      [32]byte `json:"-"`
}

type FinalEnvelope struct {
	Type       string `json:"type"` // "text" | "file"
	SenderID   string `json:"sender_id"`
	ReceiverID string `json:"receiver_id"`
	Name       string `json:"name,omitempty"` // optional file name
	MsgID      string `json:"msgid"`
	DataB64    string `json:"data_b64"` // Base64URL-encoded payload (ciphertext for text; raw for file)
}

func defaultConfig() *Config {
	return &Config{
		APIPort:       8080,
		MCGroup:       "239.255.255.250",
		MCPort:        35888,
		BroadcastIntv: 3 * time.Second,
		MaxDataBytes:  1 << 30,
		MCSubnet:      "192.168.1.0/24",
		ControlPort:   8081,
	}
}
