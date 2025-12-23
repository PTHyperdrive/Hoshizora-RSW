//go:build windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func readOSString(key string) (string, error) {
	switch key {
	case "machine_guid":
		return readReg("HKLM\\SOFTWARE\\Microsoft\\Cryptography", "MachineGuid")
	case "board_name":
		return readReg("HKLM\\HARDWARE\\DESCRIPTION\\System\\BIOS", "BaseBoardProduct")
	case "win_build":
		return readReg("HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", "CurrentBuildNumber")
	case "win_install_date":
		return readReg("HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", "InstallDate")
	default:
		return "", errors.New("unknown key")
	}
}

func readReg(path, value string) (string, error) {
	out, err := runExec("reg", "query", path, "/v", value)
	if err != nil {
		return "", err
	}
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, value) {
			parts := strings.Fields(ln)
			if len(parts) >= 3 {
				return strings.Join(parts[2:], " "), nil
			}
		}
	}
	return "", errors.New("value not found")
}

func runExec(name string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, stderr.String())
	}
	return stdout.String(), nil
}
