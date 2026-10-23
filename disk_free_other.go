//go:build !windows

package main

func diskFreeBytes(string) (uint64, error) {
	return 0, nil
}
