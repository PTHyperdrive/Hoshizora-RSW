package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type ChatMsg struct {
	Text      string `json:"text"`
	PeerID    string `json:"peerId"`
	PubB64    string `json:"pubKey"`
	SigB64    string `json:"sig"`
	Timestamp int64  `json:"ts"`
}

func (m *ChatMsg) body() []byte {
	type b struct {
		Text, PeerID, PubB64 string
		Timestamp            int64
	}
	j, _ := json.Marshal(b{m.Text, m.PeerID, m.PubB64, m.Timestamp})
	return j
}

type FileManifest struct {
	ID            string `json:"id"`
	FileName      string `json:"fileName"`
	Size          int64  `json:"size"`
	ChunkSize     int    `json:"chunkSize"`
	Chunks        int    `json:"chunks"`
	PlainSHA256   string `json:"plainSha256"`
	CipherSHA256  string `json:"cipherSha256"`
	WrappedKeyB64 string `json:"wrappedKey"`
	WrapNonceB64  string `json:"wrapNonce"`
	PeerID        string `json:"peerId"`
	PubB64        string `json:"pubKey"`
	SigB64        string `json:"sig"`
	Timestamp     int64  `json:"ts"`
}

func (m *FileManifest) body() []byte {
	type b struct {
		FileName                    string
		Size                        int64
		ChunkSize, Chunks           int
		PlainSHA256, CipherSHA256   string
		WrappedKeyB64, WrapNonceB64 string
		PeerID, PubB64              string
		Timestamp                   int64
	}
	j, _ := json.Marshal(b{m.FileName, m.Size, m.ChunkSize, m.Chunks, m.PlainSHA256, m.CipherSHA256, m.WrappedKeyB64, m.WrapNonceB64, m.PeerID, m.PubB64, m.Timestamp})
	return j
}

func (m *FileManifest) computeID() string {
	h := sha256.Sum256(m.body())
	return hex.EncodeToString(h[:])
}

type FileChunk struct {
	ManifestID string `json:"mid"`
	Index      int    `json:"idx"`
	NonceB64   string `json:"nonce"`
	DataB64    string `json:"data"` // ciphertext
	PeerID     string `json:"peerId"`
	SigB64     string `json:"sig"`
}

func (c *FileChunk) body() []byte {
	type b struct {
		ManifestID string
		Index      int
		NonceB64   string
		DataB64    string
		PeerID     string
	}
	j, _ := json.Marshal(b{c.ManifestID, c.Index, c.NonceB64, c.DataB64, c.PeerID})
	return j
}
