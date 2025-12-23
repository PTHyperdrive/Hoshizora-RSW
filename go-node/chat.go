package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

func (n *Node) signChat(text string) ChatMsg {
	msg := ChatMsg{
		Text:      text,
		PeerID:    n.peerID.String(),
		PubB64:    base64.StdEncoding.EncodeToString(n.pub),
		Timestamp: time.Now().Unix(),
	}
	msg.SigB64 = base64.StdEncoding.EncodeToString(ed25519.Sign(n.priv, msg.body()))
	return msg
}

func (n *Node) verifyChat(msg ChatMsg) bool {
	pubRaw, err := base64.StdEncoding.DecodeString(msg.PubB64)
	if err != nil || len(pubRaw) != ed25519.PublicKeySize {
		return false
	}
	sigRaw, err := base64.StdEncoding.DecodeString(msg.SigB64)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pubRaw), msg.body(), sigRaw) &&
		strings.TrimSpace(msg.Text) != ""
}

func (n *Node) publishChat(text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	msg := n.signChat(text)
	data, _ := json.Marshal(msg)
	for _, pid := range n.h.Network().Peers() {
		// FIX: host.Host has no Context(); use context.Background()
		s, err := n.h.NewStream(context.Background(), pid, protoChat)
		if err != nil {
			continue
		}
		_ = s.SetWriteDeadline(time.Now().Add(3 * time.Second))
		_, _ = s.Write(data)
		_, _ = s.Write([]byte("\n")) // NDJSON
		s.CloseWrite()
		s.Close()
	}
	return nil
}
