package main

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/crypto/chacha20poly1305"
)

// ----------------------------
// Per-file symmetric key (32B)
// ----------------------------

func newFileKey() ([32]byte, error) {
	var k [32]byte
	_, err := rand.Read(k[:])
	return k, err
}

func aeadSealWithKey(k []byte, plain []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(k)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := aead.Seal(nil, nonce, plain, nil)
	return append(nonce, ct...), nil
}

func aeadOpenWithKey(k []byte, blob []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(k)
	if err != nil {
		return nil, err
	}
	if len(blob) < chacha20poly1305.NonceSizeX {
		return nil, errors.New("cipher too short")
	}
	nonce := blob[:chacha20poly1305.NonceSizeX]
	ct := blob[chacha20poly1305.NonceSizeX:]
	return aead.Open(nil, nonce, ct, nil)
}

// ----------------------------
// Local key file persistence
// ----------------------------

func ensureKeysDir(paths *EnvPaths) (string, error) {
	dir := filepath.Join(paths.BaseDir, "keys")
	err := os.MkdirAll(dir, 0o700)
	return dir, err
}

func saveFileKey(paths *EnvPaths, name string, k *[32]byte) (string, error) {
	dir, err := ensureKeysDir(paths)
	if err != nil {
		return "", err
	}
	fp := filepath.Join(dir, name)
	// store raw bytes (local secure dir). If you want, store base64 instead.
	err = os.WriteFile(fp, k[:], 0o600)
	return fp, err
}

func loadFileKey(paths *EnvPaths, name string) ([32]byte, error) {
	var k [32]byte
	dir := filepath.Join(paths.BaseDir, "keys")
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return k, err
	}
	if len(b) != 32 {
		return k, errors.New("invalid key file size")
	}
	copy(k[:], b)
	return k, nil
}
