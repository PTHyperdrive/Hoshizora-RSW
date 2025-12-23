package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

var envMagic = []byte("MENV1") // file header for env.enc

// kdf derives a 32B key from passphrase and salt using Argon2id.
// m=64 MiB, t=2, p=1 (tune if needed).
func kdf(pass []byte, salt []byte) []byte {
	return argon2.IDKey(pass, salt, 2, 64*1024, 1, 32)
}

// sealEnvSecrets encrypts EnvSecrets JSON into env.enc: MAGIC|salt|nonce|ct.
func sealEnvSecrets(path string, pass []byte, sec *EnvSecrets) error {
	plain, err := json.Marshal(struct {
		BeaconKeyB64 string `json:"beacon_key_b64"`
		FileKeyB64   string `json:"file_key_b64"`
	}{
		BeaconKeyB64: sec.BeaconKeyB64,
		FileKeyB64:   sec.FileKeyB64,
	})
	if err != nil {
		return err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key := kdf(pass, salt)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ct := aead.Seal(nil, nonce, plain, nil)

	out := make([]byte, 0, len(envMagic)+16+len(nonce)+len(ct)+4)
	out = append(out, envMagic...)
	out = append(out, salt...)
	out = append(out, nonce...)
	// optional plaintext length prefix for future; not required, but harmless
	var lbuf [4]byte
	binary.BigEndian.PutUint32(lbuf[:], uint32(len(plain)))
	out = append(out, lbuf[:]...)
	out = append(out, ct...)

	return os.WriteFile(path, out, 0600)
}

// openEnvSecrets decrypts env.enc using passphrase and fills sec.
func openEnvSecrets(path string, pass []byte) (*EnvSecrets, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	min := len(envMagic) + 16 + chacha20poly1305.NonceSizeX + 4
	if len(b) < min {
		return nil, errors.New("env.enc too short")
	}
	if string(b[:len(envMagic)]) != string(envMagic) {
		return nil, errors.New("bad env.enc magic")
	}
	offset := len(envMagic)
	salt := b[offset : offset+16]
	offset += 16
	nonce := b[offset : offset+chacha20poly1305.NonceSizeX]
	offset += chacha20poly1305.NonceSizeX
	// skip len (4 bytes)
	offset += 4
	ct := b[offset:]

	key := kdf(pass, salt)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	plain, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, errors.New("env.enc decrypt failed (wrong pass?)")
	}
	var tmp struct {
		BeaconKeyB64 string `json:"beacon_key_b64"`
		FileKeyB64   string `json:"file_key_b64"`
	}
	if err := json.Unmarshal(plain, &tmp); err != nil {
		return nil, err
	}
	sec := &EnvSecrets{
		BeaconKeyB64: tmp.BeaconKeyB64,
		FileKeyB64:   tmp.FileKeyB64,
	}
	// decode into fixed arrays
	if dec, err := base64.RawURLEncoding.DecodeString(sec.BeaconKeyB64); err == nil && len(dec) == 32 {
		copy(sec.BeaconKey[:], dec)
	} else {
		return nil, fmt.Errorf("invalid beacon key in env.enc")
	}
	if dec, err := base64.RawURLEncoding.DecodeString(sec.FileKeyB64); err == nil && len(dec) == 32 {
		copy(sec.FileKey[:], dec)
	} else {
		return nil, fmt.Errorf("invalid file key in env.enc")
	}
	return sec, nil
}
