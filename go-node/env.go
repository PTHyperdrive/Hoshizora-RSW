// env.go  (replace file or append new pieces)

package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

// Secrets stored inside env.enc

func initStorageEnv() (*EnvPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot find home dir: %v", err)
	}
	base := filepath.Join(home, ".mixnets")
	chunks := filepath.Join(base, "chunks")
	if err := os.MkdirAll(chunks, 0o700); err != nil {
		return nil, fmt.Errorf("cannot create mixnets dirs: %v", err)
	}
	p := &EnvPaths{
		BaseDir:   base,
		ConfigEnc: filepath.Join(base, "Config.enc"),
		PeersEnc:  filepath.Join(base, "peers.enc"),
		ChunksDir: chunks,
		KeyPath:   filepath.Join(base, "key.pem"),
		EnvEnc:    filepath.Join(base, "env.enc"),
	}
	log.Printf("[env] using %s for mixnets storage (%s)", base, runtime.GOOS)
	return p, nil
}

func createEnvSecrets(paths *EnvPaths, pass []byte) (*EnvSecrets, error) {
	var s EnvSecrets
	if _, err := rand.Read(s.BeaconKey[:]); err != nil {
		return nil, err
	}
	if _, err := rand.Read(s.FileKey[:]); err != nil {
		return nil, err
	}
	s.BeaconKeyB64 = base64.RawURLEncoding.EncodeToString(s.BeaconKey[:])
	s.FileKeyB64 = base64.RawURLEncoding.EncodeToString(s.FileKey[:])
	if err := sealEnvSecrets(paths.EnvEnc, pass, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func loadEnvSecrets(paths *EnvPaths, pass []byte) (*EnvSecrets, error) {
	return openEnvSecrets(paths.EnvEnc, pass)
}
