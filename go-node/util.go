package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
)

func mustDecodeB64(s string) []byte {
	b, _ := base64.StdEncoding.DecodeString(s)
	return b
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' {
			return '-'
		}
		return r
	}, s)
}

func trim(s string) string { return strings.TrimSpace(s) }

func tempUploadPath(name string) string {
	return filepath.Join(os.TempDir(), "up__"+sanitize(name))
}
