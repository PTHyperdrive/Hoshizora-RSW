package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand" // <- use this
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/hkdf"
)

func hkdfBytes(key []byte, info string, n int) []byte {
	h := hkdf.New(sha256.New, key, nil, []byte(info))
	out := make([]byte, n)
	io.ReadFull(h, out)
	return out
}

func gcm(key []byte) cipher.AEAD {
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	return aead
}

func groupKey() ([]byte, error) {
	hexStr := strings.TrimSpace(os.Getenv("GROUP_KEY_HEX"))
	if hexStr == "" {
		return nil, errors.New("GROUP_KEY_HEX not set")
	}
	b, err := hex.DecodeString(hexStr)
	if err != nil || len(b) != 32 {
		return nil, errors.New("GROUP_KEY_HEX must be 32 bytes hex (64 hex chars)")
	}
	return b, nil
}

func wrapKeyWithGroup(kFile []byte) (wrapped, nonce []byte, err error) {
	gk, err := groupKey()
	if err != nil {
		return nil, nil, err
	}
	a := gcm(gk)
	nonce = make([]byte, 12)
	_, _ = rand.Read(nonce) // <- changed
	wrapped = a.Seal(nil, nonce, kFile, nil)
	return wrapped, nonce, nil
}

func unwrapKeyWithGroup(wrapped, nonce []byte) ([]byte, error) {
	gk, err := groupKey()
	if err != nil {
		return nil, err
	}
	a := gcm(gk)
	return a.Open(nil, nonce, wrapped, nil)
}
