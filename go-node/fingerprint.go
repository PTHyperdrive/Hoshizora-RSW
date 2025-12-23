package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"io"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/crypto/hkdf"
)

type fpInput struct {
	SN   string   `json:"sn,omitempty"`
	MACs []string `json:"macs,omitempty"`
	Disp string   `json:"disp,omitempty"`
	OS   string   `json:"os"`
	Host string   `json:"host"`
}

func trySerial() string {
	if s := os.Getenv("MIXNETS_DEVICE_SN"); s != "" {
		return s
	}
	if runtime.GOOS == "linux" {
		paths := []string{
			"/sys/class/dmi/id/product_uuid",
			"/sys/class/dmi/id/board_serial",
			"/sys/devices/virtual/dmi/id/product_uuid",
		}
		for _, p := range paths {
			if b, err := os.ReadFile(p); err == nil {
				s := strings.TrimSpace(string(b))
				if s != "" && s != "None" {
					return s
				}
			}
		}
	}
	return ""
}

func allMACs() []string {
	ifs, _ := net.Interfaces()
	var macs []string
	for _, i := range ifs {
		if i.Flags&net.FlagLoopback != 0 {
			continue
		}
		m := i.HardwareAddr.String()
		if m == "" {
			continue
		}
		macs = append(macs, strings.ToLower(m))
	}
	sort.Strings(macs)
	return macs
}

func primaryDisplay() string {
	if s := os.Getenv("MIXNETS_DISP"); s != "" {
		return s
	}
	return ""
}

func deriveNodeKeyPair(orgSalt []byte) (ed25519.PrivateKey, ed25519.PublicKey, string) {
	host, _ := os.Hostname()
	fp := fpInput{
		SN:   trySerial(),
		MACs: allMACs(),
		Disp: primaryDisplay(),
		OS:   runtime.GOOS,
		Host: host,
	}
	j, _ := json.Marshal(fp)
	h := sha256.Sum256(j)

	// NodeID = base32(SHA256(salt || h))[:52]
	nodeHash := sha256.Sum256(append(orgSalt, h[:]...))
	id := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(nodeHash[:]))
	if len(id) > 52 {
		id = id[:52]
	}

	// ed25519 seed via HKDF(salt, h)
	hk := hkdf.New(sha256.New, h[:], orgSalt, []byte("mixnets-device-seed"))
	seed := make([]byte, 32)
	io.ReadFull(hk, seed)
	priv := ed25519.NewKeyFromSeed(seed)
	return priv, priv.Public().(ed25519.PublicKey), id
}
