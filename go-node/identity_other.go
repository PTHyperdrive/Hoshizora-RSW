//go:build !windows

package main

func readOSString(key string) (string, error) {
	return "", nil
}
