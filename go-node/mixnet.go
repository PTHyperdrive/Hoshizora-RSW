// mixnet.go
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"sort"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
)

// FinalEnvelope is what the LAST hop receives as plaintext.
// For Type "text", Data is the encrypted text (hard-coded key); for "file", Data is raw file bytes.
// ---------------- Hard-coded text encryption (prototype) ----------------

// WARNING: for demo only. Derive a static 32-byte key from a hard-coded passphrase.
// In production, replace with per-peer/session keys and real KMS.
var hardCodedTextKey = sha256.Sum256([]byte("MIXNET_TEXT_KEY_v1"))

func encryptTextHardcoded(plain []byte) (string, error) {
	aead, err := chacha20poly1305.NewX(hardCodedTextKey[:])
	if err != nil {
		return "", err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := aead.Seal(nil, nonce, plain, nil)
	out := append(nonce, ct...)
	return base64.RawURLEncoding.EncodeToString(out), nil
}

func decryptTextHardcoded(b64 string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	if len(raw) < chacha20poly1305.NonceSizeX {
		return nil, errors.New("ciphertext too short")
	}
	nonce := raw[:chacha20poly1305.NonceSizeX]
	ct := raw[chacha20poly1305.NonceSizeX:]
	aead, err := chacha20poly1305.NewX(hardCodedTextKey[:])
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ct, nil)
}

// ---------------- Build "furthest" path ----------------

// chooseHopsFurthest selects up to maxHops peers that:
//  1. end with dest (if present in the peer list),
//  2. otherwise are the farthest by XOR distance from selfID.
//
// Requires PeerInfo to carry Addr (ip:port) and PubKey (32 bytes).
func chooseHopsFurthest(selfID, destID string, peers []PeerInfo, maxHops int) ([]hopInfo, error) {
	if maxHops < 1 {
		maxHops = 1
	}
	// First, see if dest is known
	var dest *PeerInfo
	candidates := make([]PeerInfo, 0, len(peers))
	for _, p := range peers {
		if p.NodeID == selfID {
			continue
		}
		if len(p.PubKey) != 32 || p.Addr == "" {
			continue
		}
		if p.NodeID == destID {
			cp := p
			dest = &cp
			continue
		}
		candidates = append(candidates, p)
	}
	if dest == nil {
		return nil, fmt.Errorf("destination %s not found among peers", destID)
	}

	// Sort candidates by XOR distance from selfID, descending (furthest first)
	sort.Slice(candidates, func(i, j int) bool {
		di := xorDistance(selfID, candidates[i].NodeID)
		dj := xorDistance(selfID, candidates[j].NodeID)
		return di.Cmp(dj) > 0
	})

	// Build path: pick farthest (maxHops-1) + final dest
	hops := make([]hopInfo, 0, maxHops)
	for _, p := range candidates {
		if len(hops) >= maxHops-1 {
			break
		}
		hops = append(hops, hopInfo{NodeID: p.NodeID, Addr: p.Addr, PubKey: p.PubKey})
	}
	// Ensure final hop is dest
	hops = append(hops, hopInfo{NodeID: dest.NodeID, Addr: dest.Addr, PubKey: dest.PubKey})
	return hops, nil
}

// ------------------- Node keypair -------------------
// Node should hold these (generate at startup and advertise pubkey in beacon)
type NodeKeypair struct {
	Priv [32]byte
	Pub  [32]byte
}

func newNodeKeypair() (*NodeKeypair, error) {
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return nil, err
	}
	// clamp private according to X25519 spec (not strictly necessary when using curve25519.X25519 helper)
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, &priv) // helper: some libs have different signature; below I use X25519
	// If your curve25519 package provides X25519, use that:
	// pub, _ := curve25519.X25519(priv[:], curve25519.Basepoint)
	// But a statically-typed array is easier to store.
	copy(pub[:], pub[:])

	return &NodeKeypair{Priv: priv, Pub: pub}, nil
}

// For compatibility, we will use X25519 via curve25519.X25519 where available:
// ------------------- Onion layer format -------------------
// Each layer is encrypted with AEAD using key = HKDF(shared_secret) (we'll use first 32 bytes).
// Layer plaintext (JSON) structure:
// {
//   "next": "ip:port" or "" if final,
//   "payload": base64(ciphertext of inner layer or final payload),
//   "meta": { "final": bool, "msgid": "...", "ttl": n }
// }

// For simplicity we'll create functions to build onion and to peel one layer.

// ------------------- Helpers -------------------
func randBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

func aeadEncrypt(key32, plaintext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key32)
	if err != nil {
		return nil, err
	}
	nonce, err := randBytes(chacha20poly1305.NonceSizeX)
	if err != nil {
		return nil, err
	}
	ct := aead.Seal(nil, nonce, plaintext, nil)
	return append(nonce, ct...), nil
}

func aeadDecrypt(key32, nonceAndCT []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key32)
	if err != nil {
		return nil, err
	}
	if len(nonceAndCT) < chacha20poly1305.NonceSizeX {
		return nil, errors.New("ciphertext too short")
	}
	nonce := nonceAndCT[:chacha20poly1305.NonceSizeX]
	ct := nonceAndCT[chacha20poly1305.NonceSizeX:]
	pt, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, err
	}
	return pt, nil
}

// naive HKDF: here we just use the shared secret and expand to 32 bytes by hashing (for prototype).
// In production use proper HKDF.
func sharedToKey(shared []byte) []byte {
	// Use simple SHA-256(shared) -> 32
	h := sha256Sum(shared)
	return h[:]
}

func sha256Sum(b []byte) [32]byte {
	var out [32]byte
	h := sha256.New()
	h.Write(b)
	sum := h.Sum(nil)
	copy(out[:], sum)
	return out
}

// ------------------- Build onion -------------------

// hopInfo contains the info needed for each hop: NodeID, addr, and pubkey bytes
type hopInfo struct {
	NodeID string
	Addr   string // ip:port
	PubKey []byte // 32 bytes
}

// buildOnion: hops is ordered [hop0, hop1, ..., finalHop]. payload is final plaintext (file chunk).
// Returns top-level onionPacket as bytes that should be sent to hops[0].Addr
func buildOnion(hops []hopInfo, payload []byte, ttl int) ([]byte, error) {
	// start from final payload (inner-most plaintext)
	inner := payload
	msgidBytes, _ := randBytes(16)
	msgid := base64.RawURLEncoding.EncodeToString(msgidBytes)

	for i := len(hops) - 1; i >= 0; i-- {
		h := hops[i]
		plain := onionLayerPlain{}
		if i == len(hops)-1 { // final
			plain.Next = ""
			plain.Payload = base64.RawURLEncoding.EncodeToString(inner)
			plain.Meta.Final = true
			plain.Meta.MsgID = msgid
			plain.Meta.TTL = ttl
		} else {
			plain.Next = hops[i+1].Addr
			plain.Payload = base64.RawURLEncoding.EncodeToString(inner)
			plain.Meta.Final = false
			plain.Meta.MsgID = msgid
			plain.Meta.TTL = ttl
		}
		plainB, _ := json.Marshal(plain)

		// ephemeral key for this layer
		ephemeralPriv := make([]byte, 32)
		if _, err := rand.Read(ephemeralPriv); err != nil {
			return nil, err
		}
		ephemeralPub, _ := curve25519.X25519(ephemeralPriv, curve25519.Basepoint)

		// shared = X25519(ephemeralPriv, hop.PubKey)
		shared, err := curve25519.X25519(ephemeralPriv, h.PubKey)
		if err != nil {
			return nil, err
		}
		aeadKey := sharedToKey(shared)
		ct, err := aeadEncrypt(aeadKey, plainB)
		if err != nil {
			return nil, err
		}
		op := onionPacket{
			EphemeralPub: base64.RawURLEncoding.EncodeToString(ephemeralPub),
			Ciphertext:   base64.RawURLEncoding.EncodeToString(ct),
		}
		inner, _ = json.Marshal(op) // inner becomes the ciphertext for next outer layer
	}
	return inner, nil // fully wrapped onion (JSON bytes) to send to first hop
}

// ------------------- Relay handler: peel one layer -------------------

// relayHandler should be registered on each node as POST /mix/relay
// Body: JSON onionPacket (outermost). The handler will:
//   - decode JSON, use its own privkey to derive shared key and decrypt one layer
//   - obtain next and payload; if next=="" then this node is final receiver and will process payload
//   - else forward to next address via HTTP POST to /mix/relay
func relayHandler(nodeKeys *NodeKeypair, srv *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse outer onion packet
		var op onionPacket
		if err := json.NewDecoder(r.Body).Decode(&op); err != nil {
			http.Error(w, "bad packet", http.StatusBadRequest)
			return
		}

		epub, err := base64.RawURLEncoding.DecodeString(op.EphemeralPub)
		if err != nil || len(epub) != 32 {
			http.Error(w, "bad ephemeral", http.StatusBadRequest)
			return
		}
		ct, err := base64.RawURLEncoding.DecodeString(op.Ciphertext)
		if err != nil {
			http.Error(w, "bad ct", http.StatusBadRequest)
			return
		}

		// Derive per-hop key: X25519(selfPriv, ephPub) -> AEAD(sha256(shared))
		shared, err := curve25519.X25519(nodeKeys.Priv[:], epub)
		if err != nil {
			http.Error(w, "shared fail", http.StatusInternalServerError)
			return
		}
		aeadKey := sharedToKey(shared)

		plainB, err := aeadDecrypt(aeadKey, ct)
		if err != nil {
			http.Error(w, "decrypt fail", http.StatusForbidden)
			return
		}

		// One hop's plaintext
		var plain onionLayerPlain
		if err := json.Unmarshal(plainB, &plain); err != nil {
			http.Error(w, "bad layer", http.StatusBadRequest)
			return
		}
		if plain.Meta.TTL <= 0 {
			http.Error(w, "ttl expired", http.StatusBadRequest)
			return
		}

		// innerB is the next content (either another onionPacket JSON or FinalEnvelope JSON)
		innerB, err := base64.RawURLEncoding.DecodeString(plain.Payload)
		if err != nil {
			http.Error(w, "bad inner payload", http.StatusBadRequest)
			return
		}

		// FINAL HOP?
		if plain.Next == "" || plain.Meta.Final {
			// Try to parse FinalEnvelope
			var env FinalEnvelope
			if err := json.Unmarshal(innerB, &env); err != nil {
				// Store raw if not an envelope
				key := "mixmsg-" + time.Now().Format("150405.000")
				srv.mu.Lock()
				srv.kv[key] = innerB
				srv.mu.Unlock()
				log.Printf("[mix] final: stored RAW %d bytes (couldn't parse envelope)", len(innerB))
				writeJSON(w, map[string]any{"status": "ok", "final": true, "raw": true})
				return
			}

			switch env.Type {
			case "text":
				plainTxt, err := decryptTextHardcoded(env.DataB64)
				if err != nil {
					log.Printf("[mix] final text decrypt fail: %v", err)
					http.Error(w, "decrypt fail", http.StatusForbidden)
					return
				}
				key := "text-" + env.MsgID
				srv.mu.Lock()
				srv.kv[key] = plainTxt
				srv.mu.Unlock()
				log.Printf("[mix] final TEXT: msgid=%s from=%s to=%s size=%d", env.MsgID, env.SenderID, env.ReceiverID, len(plainTxt))
				writeJSON(w, map[string]any{"status": "ok", "final": true, "type": "text", "msgid": env.MsgID})

			case "file":
				raw, err := base64.RawURLEncoding.DecodeString(env.DataB64)
				if err != nil {
					http.Error(w, "bad file payload", http.StatusBadRequest)
					return
				}
				key := "file-" + env.MsgID + "-" + env.Name
				srv.mu.Lock()
				srv.kv[key] = raw
				srv.mu.Unlock()
				log.Printf("[mix] final FILE: msgid=%s name=%s from=%s to=%s size=%d", env.MsgID, env.Name, env.SenderID, env.ReceiverID, len(raw))
				writeJSON(w, map[string]any{"status": "ok", "final": true, "type": "file", "msgid": env.MsgID, "name": env.Name})

			default:
				key := "mixmsg-" + env.MsgID
				srv.mu.Lock()
				srv.kv[key] = innerB
				srv.mu.Unlock()
				writeJSON(w, map[string]any{"status": "ok", "final": true, "type": "unknown", "msgid": env.MsgID})
			}
			return
		}

		// NOT FINAL: forward inner onion JSON (innerB) to next hop with jitter
		plain.Meta.TTL--

		// Random small delay (100–600ms) as mixing jitter
		jitter, _ := rand.Int(rand.Reader, big.NewInt(500))
		time.Sleep(time.Millisecond * (100 + time.Duration(jitter.Int64())))

		nextURL := fmt.Sprintf("http://%s/mix/relay", plain.Next)
		resp, err := http.Post(nextURL, "application/json", bytes.NewReader(innerB))
		if err != nil {
			log.Printf("[mix] forward err to %s: %v", plain.Next, err)
			http.Error(w, "forward fail", http.StatusBadGateway)
			return
		}
		_ = resp.Body.Close()
		writeJSON(w, map[string]any{"status": "forwarded", "to": plain.Next})
	}
}
