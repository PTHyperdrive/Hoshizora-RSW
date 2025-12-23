package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"runtime"
	"strings"
)

func buildNodeIdentity() NodeIdentity {
	attrs := map[string]string{
		"goos":   runtime.GOOS,
		"goarch": runtime.GOARCH,
	}
	hn, _ := os.Hostname()
	attrs["hostname"] = hn

	// MACs (best-effort) — collected in netselect.go helper
	attrs["macs"] = strings.Join(listMACs(), ",")

	// Go runtime version as coarse OS fingerprint
	attrs["osver"] = runtime.Version()

	// Windows extras (best effort) via per-OS files
	if v, err := readOSString("machine_guid"); err == nil && v != "" {
		attrs["machine_guid"] = v
	}
	if v, err := readOSString("board_name"); err == nil && v != "" {
		attrs["board_name"] = v
	}
	if v, err := readOSString("win_build"); err == nil && v != "" {
		attrs["win_build"] = v
	}
	if v, err := readOSString("win_install_date"); err == nil && v != "" {
		attrs["win_install_date"] = v
	}

	// Deterministic hash over ordered keys
	keys := []string{"macs", "osver", "win_build", "win_install_date", "machine_guid", "board_name", "hostname", "goos", "goarch"}
	var buf bytes.Buffer
	for _, k := range keys {
		buf.WriteString(k + "=" + attrs[k] + ";")
	}
	sum := sha256.Sum256(buf.Bytes())
	id := hex.EncodeToString(sum[:])

	return NodeIdentity{NodeID: id, Hostname: hn, Attrs: attrs}
}
