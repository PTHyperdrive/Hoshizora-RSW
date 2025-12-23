package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	mdns "github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
)

func envPort(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	if p, err := strconv.Atoi(v); err == nil && p > 0 && p < 65536 {
		return p
	}
	return def
}

func buildListenAddrs() []string {
	quicPort := envPort("MIXNET_QUIC_PORT", 4003)
	wrtcPort := envPort("MIXNET_WRTC_PORT", 4004)

	return []string{
		// TCP fallback
		"/ip4/0.0.0.0/tcp/0",
		"/ip6/::/tcp/0",
		// QUIC v1 (UDP)
		fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", quicPort),
		fmt.Sprintf("/ip6/::/udp/%d/quic-v1", quicPort),
		// WebRTC (UDP) — MUST NOT share UDP port with QUIC
		fmt.Sprintf("/ip4/0.0.0.0/udp/%d/webrtc", wrtcPort),
		fmt.Sprintf("/ip6/::/udp/%d/webrtc", wrtcPort),
	}
}

type Node struct {
	h      host.Host
	priv   ed25519.PrivateKey
	pub    ed25519.PublicKey
	peerID peer.ID
	nodeID string
	geo    string

	latMu sync.Mutex
	rtts  map[peer.ID]time.Duration

	chatMu  sync.Mutex
	chatLog []ChatMsg

	fileMu    sync.Mutex
	manifests map[string]FileManifest
	recvMap   map[string]map[int]bool
}

type mdnsNotifeeImpl struct{ h host.Host }

func (m *mdnsNotifeeImpl) HandlePeerFound(info peer.AddrInfo) {
	_ = m.h.Connect(context.Background(), info)
}

func newNode(ctx context.Context, _ []string, orgSalt []byte) (*Node, error) {
	// fingerprint → ed25519 + NodeID
	priv, pub, nodeID := deriveNodeKeyPair(orgSalt)
	libPriv, _, err := crypto.KeyPairFromStdKey(&priv)
	if err != nil {
		return nil, err
	}

	// Use libp2p defaults (include QUIC & WebRTC) + explicit listen addrs so we
	// actually expose those UDP transports on predictable ports for dialers.
	h, err := libp2p.New(
		libp2p.Identity(libPriv),
		libp2p.DefaultSecurity,
		libp2p.DefaultMuxers,
		libp2p.DefaultTransports, // includes TCP + QUIC + WebRTC (and others)
		libp2p.ListenAddrStrings(buildListenAddrs()...),
	)
	if err != nil {
		return nil, err
	}

	// mDNS (new API signature)
	_ = mdns.NewMdnsService(h, mdnsTag, &mdnsNotifeeImpl{h})

	n := &Node{
		h:         h,
		priv:      priv,
		pub:       pub,
		peerID:    h.ID(),
		nodeID:    nodeID,
		rtts:      map[peer.ID]time.Duration{},
		manifests: map[string]FileManifest{},
		recvMap:   map[string]map[int]bool{},
	}

	// stream handlers (unchanged)
	h.SetStreamHandler(protoChat, n.handleChatStream)
	h.SetStreamHandler(protoFile, n.handleFileStream)

	// ping loop (RTT for nearest)
	go n.pingLoop(ctx)
	return n, nil
}

func (n *Node) pingLoop(ctx context.Context) {
	p := ping.NewPingService(n.h)
	for {
		for _, pid := range n.h.Network().Peers() {
			ch := p.Ping(ctx, pid)
			select {
			case res := <-ch:
				if res.Error == nil {
					n.latMu.Lock()
					n.rtts[pid] = res.RTT
					n.latMu.Unlock()
				}
			case <-time.After(2 * time.Second):
			}
		}
		time.Sleep(3 * time.Second)
	}
}

func (n *Node) nearestPeer() (peer.ID, time.Duration) {
	n.latMu.Lock()
	defer n.latMu.Unlock()
	type item struct {
		id  peer.ID
		rtt time.Duration
	}
	var arr []item
	for _, p := range n.h.Network().Peers() {
		arr = append(arr, item{p, n.rtts[p]})
	}
	sort.Slice(arr, func(i, j int) bool { return arr[i].rtt < arr[j].rtt })
	if len(arr) == 0 {
		return "", 0
	}
	return arr[0].id, arr[0].rtt
}

// --------- stream handlers ---------

func (n *Node) handleChatStream(s network.Stream) {
	defer s.Close()
	dec := json.NewDecoder(s)
	for {
		var msg ChatMsg
		if err := dec.Decode(&msg); err != nil {
			return
		}
		if !n.verifyChat(msg) {
			continue
		}
		n.chatMu.Lock()
		n.chatLog = append(n.chatLog, msg)
		n.chatMu.Unlock()
		log.Printf("[chat] %s: %s", msg.PeerID, msg.Text)
	}
}

func (n *Node) handleFileStream(s network.Stream) {
	defer s.Close()
	dec := json.NewDecoder(s)
	for {
		// mixed stream: first value determines type
		var probe map[string]any
		if err := dec.Decode(&probe); err != nil {
			return
		}
		if _, ok := probe["fileName"]; ok {
			// manifest
			b, _ := json.Marshal(probe)
			var man FileManifest
			_ = json.Unmarshal(b, &man)
			if !n.verifyManifest(man) {
				continue
			}
			n.fileMu.Lock()
			n.manifests[man.ID] = man
			if _, ok := n.recvMap[man.ID]; !ok {
				n.recvMap[man.ID] = map[int]bool{}
			}
			n.fileMu.Unlock()
			log.Printf("[man] %s %s (%d bytes, %d chunks)", man.PeerID, man.FileName, man.Size, man.Chunks)
		} else {
			// chunk
			b, _ := json.Marshal(probe)
			var ch FileChunk
			_ = json.Unmarshal(b, &ch)
			n.storeChunk(ch)
		}
	}
}
