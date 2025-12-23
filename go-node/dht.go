package main

import (
	"encoding/hex"
	"math/big"
	"sync"
)

type DHT interface {
	Put(key string, providers []string)
	Get(key string) []string
	SelfID() string
}

type simpleDHT struct {
	selfID string
	mu     sync.RWMutex
	table  map[string]map[string]struct{} // key -> set(nodeID)
}

func newSimpleDHT(selfID string) *simpleDHT {
	return &simpleDHT{selfID: selfID, table: make(map[string]map[string]struct{})}
}

func (d *simpleDHT) Put(key string, providers []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	set := d.table[key]
	if set == nil {
		set = make(map[string]struct{})
		d.table[key] = set
	}
	for _, p := range providers {
		set[p] = struct{}{}
	}
}

func (d *simpleDHT) Get(key string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	set := d.table[key]
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	return out
}

func (d *simpleDHT) SelfID() string { return d.selfID }

// XOR helpers (for future Kademlia)
func xorDistance(a, b string) *big.Int {
	ax, _ := hex.DecodeString(a)
	bx, _ := hex.DecodeString(b)
	if len(ax) != len(bx) {
		n := max(len(ax), len(bx))
		ax = leftPad(ax, n)
		bx = leftPad(bx, n)
	}
	out := make([]byte, len(ax))
	for i := range ax {
		out[i] = ax[i] ^ bx[i]
	}
	return new(big.Int).SetBytes(out)
}

func leftPad(b []byte, n int) []byte {
	if len(b) >= n {
		return b
	}
	p := make([]byte, n-len(b))
	return append(p, b...)
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
