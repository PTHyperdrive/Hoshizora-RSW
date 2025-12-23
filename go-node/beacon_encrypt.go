// beacon_encrypt.go (replace file content)

package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"

	"golang.org/x/crypto/chacha20poly1305"
)

var beaconMagic = []byte("MIXB1")

func encryptBeaconWithKey(v any, key []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	plain, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := aead.Seal(nil, nonce, plain, nil)
	out := append(append(beaconMagic[:0:0], beaconMagic...), nonce...)
	out = append(out, ct...)
	return out, nil
}

func decryptBeaconWithKey(pkt []byte, key []byte, out any) error {
	if len(pkt) <= len(beaconMagic)+chacha20poly1305.NonceSizeX {
		return errors.New("packet too short")
	}
	if string(pkt[:len(beaconMagic)]) != string(beaconMagic) {
		return errors.New("bad magic")
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return err
	}
	nonce := pkt[len(beaconMagic) : len(beaconMagic)+chacha20poly1305.NonceSizeX]
	ct := pkt[len(beaconMagic)+chacha20poly1305.NonceSizeX:]
	plain, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(plain, out)
}
