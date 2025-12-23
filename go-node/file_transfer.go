package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func (n *Node) peersByRTT() []peer.ID {
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
	out := make([]peer.ID, 0, len(arr))
	for _, it := range arr {
		out = append(out, it.id)
	}
	return out
}

func (n *Node) verifyManifest(man FileManifest) bool {
	pubRaw, _ := base64.StdEncoding.DecodeString(man.PubB64)
	if len(pubRaw) != ed25519.PublicKeySize {
		return false
	}
	sigRaw, _ := base64.StdEncoding.DecodeString(man.SigB64)
	if !ed25519.Verify(ed25519.PublicKey(pubRaw), man.body(), sigRaw) {
		return false
	}
	return man.computeID() == man.ID
}

func (n *Node) broadcastFile(filePath string) (FileManifest, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return FileManifest{}, err
	}
	st, _ := f.Stat()
	defer f.Close()

	// Per-file symmetric key (32 bytes)
	kFile := make([]byte, 32)
	_, _ = rand.Read(kFile)

	plainHash := sha256.New()
	ciphHash := sha256.New()

	chunks := int((st.Size() + int64(maxChunk) - 1) / int64(maxChunk))
	man := FileManifest{
		FileName:  filepath.Base(filePath),
		Size:      st.Size(),
		ChunkSize: maxChunk,
		Chunks:    chunks,
		PeerID:    n.peerID.String(),
		PubB64:    base64.StdEncoding.EncodeToString(n.pub),
		Timestamp: time.Now().Unix(),
	}

	// Wrap K_file for the org (demo)
	wrapped, wnonce, err := wrapKeyWithGroup(kFile)
	if err != nil {
		return FileManifest{}, err
	}
	man.WrappedKeyB64 = base64.StdEncoding.EncodeToString(wrapped)
	man.WrapNonceB64 = base64.StdEncoding.EncodeToString(wnonce)

	// Stage encrypted chunks in memory (simpler demo)
	type staged struct{ nonce, ct []byte }
	stagedChunks := make([]staged, 0, chunks)

	buf := make([]byte, maxChunk)
	for i := 0; i < chunks; i++ {
		nr, _ := io.ReadFull(f, buf)
		plain := buf[:nr]
		plainHash.Write(plain)

		nonce := hkdfBytes(kFile, fmt.Sprintf("chunk-%d", i), 12)
		ct := gcm(kFile).Seal(nil, nonce, plain, nil)
		ciphHash.Write(ct)

		stagedChunks = append(stagedChunks, staged{nonce, ct})
	}

	man.PlainSHA256 = hex.EncodeToString(plainHash.Sum(nil))
	man.CipherSHA256 = hex.EncodeToString(ciphHash.Sum(nil))
	man.SigB64 = base64.StdEncoding.EncodeToString(ed25519.Sign(n.priv, man.body()))
	man.ID = man.computeID()

	// Send to each peer over a /file stream: manifest first, then NDJSON chunks
	for _, pid := range n.peersByRTT() {
		s, err := n.h.NewStream(context.Background(), pid, protoFile)
		if err != nil {
			continue
		}
		enc := json.NewEncoder(s)
		_ = s.SetWriteDeadline(time.Now().Add(10 * time.Second))
		// manifest
		_ = enc.Encode(man)
		// chunks
		for i, st := range stagedChunks {
			ch := FileChunk{
				ManifestID: man.ID,
				Index:      i,
				NonceB64:   base64.StdEncoding.EncodeToString(st.nonce),
				DataB64:    base64.StdEncoding.EncodeToString(st.ct),
				PeerID:     n.peerID.String(),
			}
			// newline-delimited JSON
			b, _ := json.Marshal(ch)
			_, _ = s.Write(b)
			_, _ = s.Write([]byte("\n"))
			time.Sleep(8 * time.Millisecond)
		}
		s.CloseWrite()
		s.Close()
	}

	return man, nil
}

func (n *Node) storeChunk(ch FileChunk) {
	n.fileMu.Lock()
	man, ok := n.manifests[ch.ManifestID]
	n.fileMu.Unlock()
	if !ok {
		return
	}

	nonce := mustDecodeB64(ch.NonceB64)
	ct := mustDecodeB64(ch.DataB64)

	kFile, err := unwrapKeyWithGroup(mustDecodeB64(man.WrappedKeyB64), mustDecodeB64(man.WrapNonceB64))
	if err != nil {
		return
	}
	pt, err := gcm(kFile).Open(nil, nonce, ct, nil)
	if err != nil {
		return
	}

	os.MkdirAll(filepath.Join(storeDir, ch.ManifestID), 0o755)
	fn := filepath.Join(storeDir, ch.ManifestID, fmt.Sprintf("%06d.part", ch.Index))
	_ = os.WriteFile(fn, pt, 0o644)

	n.fileMu.Lock()
	if _, ok := n.recvMap[ch.ManifestID]; !ok {
		n.recvMap[ch.ManifestID] = map[int]bool{}
	}
	n.recvMap[ch.ManifestID][ch.Index] = true
	complete := len(n.recvMap[ch.ManifestID]) == man.Chunks
	n.fileMu.Unlock()

	if complete {
		n.tryAssemble(ch.ManifestID)
	}
}

// tryAssemble assembles plaintext parts, verifies SHA-256, and writes final file.
func (n *Node) tryAssemble(mid string) {
	n.fileMu.Lock()
	man := n.manifests[mid]
	n.fileMu.Unlock()

	out := filepath.Join(storeDir, man.ID+"__"+sanitize(man.FileName))
	if _, err := os.Stat(out); err == nil {
		return
	}
	fout, err := os.Create(out)
	if err != nil {
		return
	}
	defer fout.Close()

	h := sha256.New()
	for i := 0; i < man.Chunks; i++ {
		part := filepath.Join(storeDir, man.ID, fmt.Sprintf("%06d.part", i))
		b, err := os.ReadFile(part)
		if err != nil {
			return
		}
		h.Write(b)
		if _, err := fout.Write(b); err != nil {
			return
		}
	}
	if hex.EncodeToString(h.Sum(nil)) != man.PlainSHA256 {
		log.Printf("[file] integrity FAILED for %s", man.FileName)
		return
	}
	log.Printf("[file] OK: %s", out)
}
